package coaster

import (
	"fmt"
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/queue"
)

// Guest flow: arrive -> balk check -> outdoor standby / single-rider line (reneging allowed) ->
// preshow rooms (if any; single riders go through it too) -> load-area lines -> board.
// Seats are rows of trains.seats_per_row: a standby party takes whole rows and is never split. The
// spare seats parties leave (gaps) go to a solo party at the head of the standby line (the grouper
// pairs odd parties; no skipping) or to single riders; from load_done on, single riders may also take
// whole empty rows (decision 2026-10-17).
//
// After a preshow the load-area line is one combined line ([user] observed on Rock 'n' Roller): parties
// and single riders keep their release order, a single rider who reaches the front boards like a solo
// party, and the grouper pulls the earliest single rider from anywhere in the line to fill a gap. It is
// kept as two queues ordered by LineSeq so the grouper's pull stays O(1). Rides without a preshow keep
// a separate outdoor single-rider line.

func (c *Coaster) initQueues() {
	c.loadStandby, c.loadSingle = c.standby, c.single
	if p := c.cfg.Preshow; p != nil && p.Enabled && p.Rooms > 0 {
		c.loadStandby, c.loadSingle = &queue.FIFO{}, &queue.FIFO{}
		c.preshow = queue.NewPreshow(c.sim, p, preshowHost{c})
	}
}

func (c *Coaster) newTrain() *train {
	c.nextTrainID++
	spr := max(1, c.cfg.Trains.SeatsPerRow)
	return &train{id: c.nextTrainID, seats: c.cfg.Trains.SeatsPerTrain, rows: c.cfg.Trains.SeatsPerTrain / spr}
}

func (t *train) full() bool { return t.rowsFree == 0 && t.gaps == 0 }

func (c *Coaster) rowsFor(size int) int {
	spr := max(1, c.cfg.Trains.SeatsPerRow)
	return (size + spr - 1) / spr
}

// Arrive implements ride.Ride.
func (c *Coaster) Arrive(p *guest.Party, lane guest.Lane) {
	switch lane {
	case guest.LightningLane:
		panic(fmt.Sprintf("coaster %s: Lightning Lane is M4", c.ID()))
	case guest.SingleRider:
		if !c.cfg.Queues.SingleRider.Enabled {
			lane = guest.Standby // no single-rider line: they queue standby as a party of 1
		}
	}
	p.Lane = lane
	if p.Patience > 0 && c.PostedWait() > p.Patience {
		c.sim.Emit(engine.Event{Type: engine.EvBalk, Guest: p.ID, N: p.Size, Info: lane.String()})
		return
	}
	p.JoinedAt = c.sim.Now()
	c.sim.Emit(engine.Event{Type: engine.EvQueueJoin, Guest: p.ID, N: p.Size, Info: lane.String()})
	line := c.standby
	if lane == guest.SingleRider {
		line = c.single
	}
	line.Push(p)
	if p.Patience > 0 {
		c.sim.At(p.JoinedAt.Add(p.Patience), func() {
			if p.Committed || p.Gone {
				return
			}
			line.Remove(p)
			c.sim.Emit(engine.Event{Type: engine.EvRenege, Guest: p.ID, N: p.Size, Info: lane.String()})
		})
	}
	c.flow()
}

// flow moves guests forward after any queue change.
func (c *Coaster) flow() {
	if c.preshow != nil {
		c.preshow.Pull()
	}
	t := c.load.occupant
	if t == nil || c.Stopped() || (t.phase != phaseBoarding && t.phase != phaseFillWait) {
		return
	}
	c.board(t, t.phase == phaseFillWait)
	if t.phase == phaseFillWait {
		c.checkFillWait(t)
	}
}

// board seats standby parties in whole rows (stopping at the first that does not fit), then single
// riders in the gaps, and in whole empty rows when fullRows is set (after load_done).
// lineHead returns the next party in line to board and the queue it is in. With a combined line it is
// whichever lane's head was released first; otherwise only standby boards in order.
func (c *Coaster) lineHead() (*guest.Party, *queue.FIFO) {
	s := c.loadStandby.Peek()
	if c.preshow == nil {
		return s, c.loadStandby
	}
	if sr := c.loadSingle.Peek(); sr != nil && (s == nil || sr.LineSeq < s.LineSeq) {
		return sr, c.loadSingle
	}
	return s, c.loadStandby
}

func (c *Coaster) board(t *train, fullRows bool) {
	spr := max(1, c.cfg.Trains.SeatsPerRow)
	boarded := false
	for {
		p, line := c.lineHead()
		if p == nil {
			break
		}
		if p.Size == 1 && t.gaps > 0 {
			line.Pop()
			t.gaps--
			c.seat(t, p)
			boarded = true
			continue
		}
		need := c.rowsFor(p.Size)
		if need > t.rowsFree {
			break
		}
		line.Pop()
		t.rowsFree -= need
		t.gaps += need*spr - p.Size
		c.seat(t, p)
		boarded = true
	}
	for t.gaps > 0 || (fullRows && t.rowsFree > 0) {
		p := c.loadSingle.Pop()
		if p == nil {
			break
		}
		if t.gaps > 0 {
			t.gaps--
		} else {
			t.rowsFree--
			t.gaps += spr - 1
		}
		c.seat(t, p)
		boarded = true
	}
	if boarded && c.preshow != nil {
		c.preshow.Drain() // load-area space freed
	}
}

func (c *Coaster) seat(t *train, p *guest.Party) {
	p.Committed = true
	c.recordWait(c.sim.Now().Sub(p.JoinedAt))
	t.riders += p.Size
	c.sim.Emit(engine.Event{Type: engine.EvBoard, Train: t.id, Block: t.at.cfg.ID, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
}

// checkFillWait ends the fill wait once the train is full or once nobody in line can fill a free seat:
// the next party needs more rows than remain and no single rider is waiting. Both hold rules stop
// waiting then (decision 2026-10-17: cast do not hold a train for guests who cannot board it). An empty
// line keeps waiting: a newcomer could still fill the train.
func (c *Coaster) checkFillWait(t *train) {
	if t.phase != phaseFillWait {
		return
	}
	head, _ := c.lineHead()
	fits := head != nil && (c.rowsFor(head.Size) <= t.rowsFree || (head.Size == 1 && t.gaps > 0))
	stuck := head != nil && !fits && c.loadSingle.Peek() == nil
	if t.full() || stuck {
		c.cancelWork(t)
		c.startCheck(t)
	}
}

// WaitingGuests is every guest who has joined and not yet boarded or reneged.
func (c *Coaster) WaitingGuests() int {
	n := c.standby.Guests() + c.single.Guests()
	if c.preshow != nil {
		n += c.preshow.Guests() + c.loadStandby.Guests() + c.loadSingle.Guests()
	}
	return n
}

// QueueGuests is WaitingGuests (kept for callers of the M1 API).
func (c *Coaster) QueueGuests() int { return c.WaitingGuests() }

// preshowHost adapts the coaster to queue.PreshowHost without widening the Coaster API.
type preshowHost struct{ c *Coaster }

func (h preshowHost) NextParty() *guest.Party {
	a, b := h.c.standby.Peek(), h.c.single.Peek()
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.JoinedAt < a.JoinedAt || (b.JoinedAt == a.JoinedAt && b.ID < a.ID):
		return b
	}
	return a
}

func (h preshowHost) TakeParty() *guest.Party {
	next := h.NextParty()
	if next == nil {
		return nil
	}
	if next.Lane == guest.SingleRider {
		return h.c.single.Pop()
	}
	return h.c.standby.Pop()
}

func (h preshowHost) LoadAreaFree() int {
	limit := h.c.cfg.Preshow.LoadAreaCapacity
	if limit <= 0 {
		return math.MaxInt
	}
	return limit - h.c.loadStandby.Guests() - h.c.loadSingle.Guests()
}

func (h preshowHost) Release(p *guest.Party) {
	h.c.releaseSeq++
	p.LineSeq = h.c.releaseSeq
	if p.Lane == guest.SingleRider {
		h.c.loadSingle.Push(p)
	} else {
		h.c.loadStandby.Push(p)
	}
	h.c.flow()
}

func (h preshowHost) Sample(d config.Dist) engine.Duration {
	return h.c.sample(d, engine.StreamStationTimes)
}
