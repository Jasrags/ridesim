package queue

import (
	"fmt"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// Preshow events. N is guests; Info is the room index.
const (
	EvPreshowFill    = "preshow_fill"    // room started FILLING
	EvPreshowShow    = "preshow_show"    // room started SHOWING with N guests
	EvPreshowRelease = "preshow_release" // room started RELEASING with N guests
	EvPreshowBlocked = "preshow_blocked" // RELEASING stalled: load area full
	EvPreshowEmpty   = "preshow_empty"   // room emptied; turnover begins
)

// PreshowHost is what a preshow stage needs from its ride.
type PreshowHost interface {
	// NextParty returns the party at the front of the outdoor queues (earliest joined across lanes).
	NextParty() *guest.Party
	// TakeParty removes NextParty from its queue.
	TakeParty() *guest.Party
	// LoadAreaFree is how many more guests the load area accepts.
	LoadAreaFree() int
	// Release moves a party from a preshow room into the load area.
	Release(p *guest.Party)
	// Sample draws a duration for the stage's timings.
	Sample(d config.Dist) engine.Duration
}

type roomState int

const (
	roomEmpty roomState = iota // ready to fill
	roomFilling
	roomShowing
	roomReleasing
	roomTurnover
)

type room struct {
	idx      int
	state    roomState
	parties  []*guest.Party
	guests   int
	timedOut bool // max_fill_wait_s passed while FILLING; start on the next guest
	gen      uint64
	blocked  bool
}

// Preshow is a batch holding stage (spec §8): rooms cycle FILLING -> SHOWING -> RELEASING -> turnover.
// A room shows when full or max_fill_wait_s after it began filling; parties are never split across
// batches; RELEASING waits while the load area is full (back-pressure into the outdoor queue). One room
// fills at a time.
type Preshow struct {
	sim   *engine.Sim
	cfg   *config.Preshow
	host  PreshowHost
	rooms []*room

	// Drain and Pull may be re-entered through host callbacks (Release -> board -> Drain). Nested calls
	// set a flag and the outer call loops, so a room is never released or emptied twice.
	draining, redrain bool
	pulling, repull   bool
}

// NewPreshow builds the stage. Call Start once the host is ready.
func NewPreshow(sim *engine.Sim, cfg *config.Preshow, host PreshowHost) *Preshow {
	p := &Preshow{sim: sim, cfg: cfg, host: host}
	for i := 0; i < cfg.Rooms; i++ {
		p.rooms = append(p.rooms, &room{idx: i})
	}
	return p
}

// Start opens the first room for filling.
func (p *Preshow) Start() { p.openNextRoom() }

// Guests is how many guests are inside the rooms.
func (p *Preshow) Guests() int {
	n := 0
	for _, r := range p.rooms {
		n += r.guests
	}
	return n
}

func (p *Preshow) emit(typ string, r *room, n int) {
	p.sim.Emit(engine.Event{Type: typ, N: n, Info: fmt.Sprint(r.idx)})
}

func (p *Preshow) filling() *room {
	for _, r := range p.rooms {
		if r.state == roomFilling {
			return r
		}
	}
	return nil
}

func (p *Preshow) openNextRoom() {
	if p.filling() != nil {
		return
	}
	for _, r := range p.rooms {
		if r.state == roomEmpty {
			r.state, r.timedOut = roomFilling, false
			r.gen++
			gen := r.gen
			p.emit(EvPreshowFill, r, 0)
			p.sim.After(engine.Seconds(p.cfg.MaxFillWaitS), func() {
				if r.gen == gen && r.state == roomFilling {
					r.timedOut = true
					p.maybeShow(r)
				}
			})
			p.Pull()
			return
		}
	}
}

// Pull moves waiting parties into the filling room. The host calls it when a party arrives.
func (p *Preshow) Pull() {
	if p.pulling {
		p.repull = true
		return
	}
	p.pulling = true
	for {
		p.repull = false
		p.pullOnce()
		if !p.repull {
			break
		}
	}
	p.pulling = false
}

func (p *Preshow) pullOnce() {
	r := p.filling()
	if r == nil {
		return
	}
	for {
		next := p.host.NextParty()
		if next == nil {
			break
		}
		if r.guests+next.Size > p.cfg.BatchSize {
			if r.guests > 0 {
				p.show(r) // the next party would not fit: the batch is full
			}
			return
		}
		p.host.TakeParty()
		next.Committed = true
		r.parties = append(r.parties, next)
		r.guests += next.Size
	}
	if r.guests == p.cfg.BatchSize {
		p.show(r)
		return
	}
	p.maybeShow(r)
}

func (p *Preshow) maybeShow(r *room) {
	if r.state == roomFilling && r.timedOut && r.guests > 0 {
		p.show(r)
	}
}

func (p *Preshow) show(r *room) {
	r.state = roomShowing
	p.emit(EvPreshowShow, r, r.guests)
	p.sim.After(p.host.Sample(p.cfg.DurationS), func() {
		r.state = roomReleasing
		p.emit(EvPreshowRelease, r, r.guests)
		p.Drain()
	})
	p.openNextRoom()
}

// Drain releases parties from RELEASING rooms while the load area has room. The host calls it when
// load-area space frees up.
func (p *Preshow) Drain() {
	if p.draining {
		p.redrain = true
		return
	}
	p.draining = true
	for {
		p.redrain = false
		p.drainOnce()
		if !p.redrain {
			break
		}
	}
	p.draining = false
}

func (p *Preshow) drainOnce() {
	for _, r := range p.rooms {
		if r.state != roomReleasing {
			continue
		}
		for len(r.parties) > 0 && r.parties[0].Size <= p.host.LoadAreaFree() {
			party := r.parties[0]
			r.parties = r.parties[1:]
			r.guests -= party.Size
			p.host.Release(party)
		}
		if len(r.parties) > 0 {
			if !r.blocked {
				r.blocked = true
				p.emit(EvPreshowBlocked, r, r.guests)
			}
			continue
		}
		r.blocked = false
		r.state = roomTurnover
		p.emit(EvPreshowEmpty, r, 0)
		p.sim.After(p.host.Sample(p.cfg.TurnoverS), func() {
			r.state = roomEmpty
			p.openNextRoom()
		})
	}
}
