package park

import (
	"math"
	"sort"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/queue"
	"github.com/jasrags/ridesim/internal/ride"
)

// Park event types (park scope unless noted). Guest is the agent id; ride-level events use visit ids
// (agent id << visitBits | visit number) so a party riding twice never collides.
const (
	EvParkArrive   = "park_arrive"   // Guest, N=size
	EvParkLeave    = "park_leave"    // Guest, N
	EvChoose       = "choose"        // Guest, Block=node, Info=lane
	EvWalk         = "walk"          // Guest, Block=edge, N, D=crossing time
	EvEdgeBlocked  = "edge_blocked"  // Guest, Block=edge: edge at jam density, waiting to enter
	EvVisitEnd     = "visit_end"     // Guest, Block=node, Info=rode|balked|reneged
	EvLLBook       = "ll_book"       // Guest, Block=node, N=window start (s after open)
	EvLLRedeem     = "ll_redeem"     // Guest, Block=node
	EvLLLapse      = "ll_lapse"      // Guest, Block=node
	EvShowStart    = "show_start"    // show scope: N=audience
	EvShowEnd      = "show_end"      // show scope: N=audience let out
	EvParkSnapshot = "park_snapshot" // N=guests in park, Info=per-state counts
	EvParkEdge     = "park_edge"     // Block=edge, N=comfortable capacity in guests, Info=from-to (at start)
)

const visitBits = 8

// activity is something a party can do at a node.
type activity interface {
	node() *Node
	// posted is what the sign says now (the board snapshots it every refresh).
	posted() engine.Duration
	// visit admits p (OnExit set) in lane; the activity calls p.Exited when the party is done.
	visit(p *guest.Party, lane guest.Lane)
	// queueGuests is guests waiting (for spillback); 0 for activities without an outdoor queue.
	queueGuests() int
	queueCapacity() int
}

// hostedRide is a full ride model running in its own scope.
type hostedRide interface {
	ride.Ride
	Start()
	WaitingGuests() int
	Closed() bool // long downtime: the sign says closed
}

type rideAct struct {
	n        *Node
	sim      *engine.Sim // the ride's scope
	r        hostedRide
	info     *config.RideInfo
	capacity int  // physical standby capacity
	ll       bool // skip line enabled
	single   bool // single-rider line enabled
}

func (a *rideAct) node() *Node             { return a.n }
func (a *rideAct) posted() engine.Duration { return a.r.PostedWait() }
func (a *rideAct) closed() bool            { return a.r.Closed() }

// visit admits p; a party reaching a closed ride (it was open on the board they read) turns away.
func (a *rideAct) visit(p *guest.Party, lane guest.Lane) {
	if a.r.Closed() {
		a.sim.Emit(engine.Event{Type: engine.EvBalk, Guest: p.ID, N: p.Size, Info: "closed"})
		p.Exited(guest.Balked)
		return
	}
	a.r.Arrive(p, lane)
}
func (a *rideAct) queueGuests() int   { return a.r.WaitingGuests() }
func (a *rideAct) queueCapacity() int { return a.capacity }

// simpleRide is a ride_simple node: a vehicle every simpleInterval takes capacity x interval guests in
// line order (parties never split, nobody skips), rides duration_s and unloads. Lightning Lane guests
// board first, up to simpleLLShare of the seats, so standby always moves (M9). Standby guests balk on the
// posted wait; there is no reneging.
type simpleRide struct {
	n      *Node
	sim    *engine.Sim
	line   queue.FIFO // standby
	llLine queue.FIFO // Lightning Lane
	seats  int
	trip   int
	llHist [simpleLLWindow]int // Lightning Lane riders on each of the last simpleLLWindow vehicles
}

// simpleInterval is the ride_simple dispatch interval; validation checks a vehicle fits the largest party.
const simpleInterval = config.RideSimpleIntervalS * engine.Second

const (
	simpleLLNum, simpleLLDen = 2, 3 // Lightning Lane takes at most 2/3 of a vehicle (spec normal 2:1 merge) [fan, assumed]
	simpleLLWindow           = 15   // vehicles (15 min) of Lightning Lane use behind the posted wait
)

func newSimpleRide(sim *engine.Sim, n *Node) *simpleRide {
	s := &simpleRide{n: n, sim: sim, seats: int(math.Max(1, math.Round(n.CapacityPerHr*simpleInterval.Seconds()/3600)))}
	return s
}

func (s *simpleRide) start() {
	var tick func()
	tick = func() {
		s.dispatch()
		s.sim.After(simpleInterval, tick)
	}
	s.sim.After(simpleInterval, tick)
}

func (s *simpleRide) node() *Node { return s.n }

// posted is standby guests over the standby share of capacity: capacity less the Lightning Lane riders
// of the last 15 vehicles, never below the share Lightning Lane must leave to standby.
func (s *simpleRide) posted() engine.Duration {
	ll := 0
	for _, n := range s.llHist {
		ll += n
	}
	standby := math.Max(1-float64(ll)/simpleLLWindow/float64(s.seats), float64(simpleLLDen-simpleLLNum)/simpleLLDen)
	return engine.Seconds(float64(s.line.Guests()) / (s.n.CapacityPerHr / 3600 * standby))
}
func (s *simpleRide) queueGuests() int   { return s.line.Guests() + s.llLine.Guests() }
func (s *simpleRide) queueCapacity() int { return s.n.QueueCapacity }

func (s *simpleRide) visit(p *guest.Party, lane guest.Lane) {
	p.Lane = guest.Standby
	if lane == guest.LightningLane {
		p.Lane = guest.LightningLane // Lightning Lane guests never balk
	} else if p.Patience > 0 && s.posted() > p.Patience {
		s.sim.Emit(engine.Event{Type: engine.EvBalk, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
		p.Exited(guest.Balked)
		return
	}
	p.JoinedAt = s.sim.Now()
	s.sim.Emit(engine.Event{Type: engine.EvQueueJoin, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
	if p.Lane == guest.LightningLane {
		s.llLine.Push(p)
	} else {
		s.line.Push(p)
	}
}

func (s *simpleRide) dispatch() {
	s.trip++
	free, riders, llRiders := s.seats, 0, 0
	var aboard []*guest.Party
	board := func(q *queue.FIFO, limit int) {
		for p := q.Peek(); p != nil && p.Size <= free && p.Size <= limit; p = q.Peek() {
			q.Pop()
			free -= p.Size
			limit -= p.Size
			riders += p.Size
			if p.Lane == guest.LightningLane {
				llRiders += p.Size
			}
			aboard = append(aboard, p)
			s.sim.Emit(engine.Event{Type: engine.EvBoard, Train: s.trip, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
		}
	}
	board(&s.llLine, s.seats*simpleLLNum/simpleLLDen)
	board(&s.line, s.seats)
	board(&s.llLine, s.seats) // seats standby could not use
	s.llHist[s.trip%simpleLLWindow] = llRiders
	if riders == 0 {
		return // empty vehicles are not logged
	}
	s.sim.Emit(engine.Event{Type: engine.EvDispatch, Train: s.trip, N: riders})
	trip := s.trip
	s.sim.After(engine.Seconds(s.n.DurationS), func() {
		s.sim.Emit(engine.Event{Type: engine.EvUnload, Train: trip, N: riders})
		for _, p := range aboard {
			p.Exited(guest.Rode)
		}
	})
}

// show is a scheduled show: parties take seats for the next performance until it starts or fills;
// when it ends everyone leaves at once (by exit_edge, handled by the agent).
type show struct {
	n      *Node
	sim    *engine.Sim
	starts []engine.Time // show starts, offsets from open, ascending
	next   int           // index of the next show not yet started
	seated []*guest.Party
	guests int
}

func newShow(sim *engine.Sim, n *Node, openS float64) *show {
	s := &show{n: n, sim: sim}
	for _, c := range n.Schedule {
		if sec, err := config.ParseClock(c); err == nil && sec >= openS {
			s.starts = append(s.starts, engine.Time(engine.Seconds(sec-openS)))
		}
	}
	sort.Slice(s.starts, func(i, j int) bool { return s.starts[i] < s.starts[j] })
	return s
}

func (s *show) start() {
	for i, at := range s.starts {
		i := i
		s.sim.At(at, func() { s.begin(i) })
	}
}

// nextStart is the next performance not yet started, or -1.
func (s *show) nextStart() engine.Time {
	if s.next >= len(s.starts) {
		return -1
	}
	return s.starts[s.next]
}

func (s *show) node() *Node { return s.n }

// posted is the time until the next performance (the wait), or a day when none is left.
func (s *show) posted() engine.Duration {
	if at := s.nextStart(); at >= 0 {
		return at.Sub(s.sim.Now())
	}
	return 24 * engine.Hour
}
func (s *show) queueGuests() int   { return 0 }
func (s *show) queueCapacity() int { return 0 }

func (s *show) visit(p *guest.Party, lane guest.Lane) {
	if s.nextStart() < 0 || s.guests+p.Size > s.n.CapacityGuests {
		s.sim.Emit(engine.Event{Type: engine.EvBalk, Guest: p.ID, N: p.Size, Info: "full"})
		p.Exited(guest.Balked)
		return
	}
	p.JoinedAt = s.sim.Now()
	s.sim.Emit(engine.Event{Type: engine.EvQueueJoin, Guest: p.ID, N: p.Size, Info: guest.Standby.String()})
	s.seated = append(s.seated, p)
	s.guests += p.Size
}

func (s *show) begin(i int) {
	s.next = i + 1
	audience, n := s.seated, s.guests
	s.seated, s.guests = nil, 0
	for _, p := range audience {
		s.sim.Emit(engine.Event{Type: engine.EvBoard, Train: i + 1, Guest: p.ID, N: p.Size, Info: guest.Standby.String()})
	}
	s.sim.Emit(engine.Event{Type: EvShowStart, Train: i + 1, N: n})
	s.sim.Emit(engine.Event{Type: engine.EvDispatch, Train: i + 1, N: n}) // riders, for per-node metrics
	s.sim.After(engine.Seconds(s.n.DurationMin*60), func() {
		s.sim.Emit(engine.Event{Type: EvShowEnd, Train: i + 1, N: n})
		for _, p := range audience {
			p.Exited(guest.Rode)
		}
	})
}

// dining seats parties in line order (no skipping) while seats are free, for a sampled service time.
type dining struct {
	n     *Node
	sim   *engine.Sim
	line  queue.FIFO
	free  int
	fixed bool
}

func newDining(sim *engine.Sim, n *Node, fixed bool) *dining {
	return &dining{n: n, sim: sim, free: n.Seats, fixed: fixed}
}

func (d *dining) node() *Node { return d.n }

// posted is queued guests over the seat turnover rate (seats / mean service time).
func (d *dining) posted() engine.Duration {
	perS := float64(d.n.Seats) / (d.n.ServiceTimeMin.Mean * 60)
	return engine.Seconds(float64(d.line.Guests()) / perS)
}
func (d *dining) queueGuests() int   { return 0 }
func (d *dining) queueCapacity() int { return 0 }

func (d *dining) visit(p *guest.Party, lane guest.Lane) {
	p.Lane = guest.Standby
	if p.Patience > 0 && d.posted() > p.Patience {
		d.sim.Emit(engine.Event{Type: engine.EvBalk, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
		p.Exited(guest.Balked)
		return
	}
	p.JoinedAt = d.sim.Now()
	d.sim.Emit(engine.Event{Type: engine.EvQueueJoin, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
	d.line.Push(p)
	d.seat()
}

func (d *dining) seat() {
	for p := d.line.Peek(); p != nil && p.Size <= d.free; p = d.line.Peek() {
		d.line.Pop()
		d.free -= p.Size
		d.sim.Emit(engine.Event{Type: engine.EvBoard, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
		d.sim.Emit(engine.Event{Type: engine.EvDispatch, N: p.Size}) // seated guests, for per-node metrics
		p := p
		d.sim.After(sampleDist(d.sim, *d.n.ServiceTimeMin, 60, d.fixed), func() {
			d.free += p.Size
			d.sim.Emit(engine.Event{Type: engine.EvUnload, Guest: p.ID, N: p.Size})
			p.Exited(guest.Rode)
			d.seat()
		})
	}
}

// elsewhere is the rest of the park, or a land's shops: no queue, a sampled visit length.
type elsewhere struct {
	n     *Node
	sim   *engine.Sim
	fixed bool
}

func (e *elsewhere) node() *Node             { return e.n }
func (e *elsewhere) posted() engine.Duration { return 0 }
func (e *elsewhere) queueGuests() int        { return 0 }
func (e *elsewhere) queueCapacity() int      { return 0 }

func (e *elsewhere) visit(p *guest.Party, lane guest.Lane) {
	e.sim.Emit(engine.Event{Type: engine.EvBoard, Guest: p.ID, N: p.Size, Info: lane.String()})
	e.sim.Emit(engine.Event{Type: engine.EvDispatch, N: p.Size})
	e.sim.After(sampleDist(e.sim, *e.n.VisitMin, 60, e.fixed), func() {
		e.sim.Emit(engine.Event{Type: engine.EvUnload, Guest: p.ID, N: p.Size})
		p.Exited(guest.Rode)
	})
}

// sampleDist draws from a normal truncated at 1 s, scaled by unit seconds (60 for minutes), on the
// scope's station_times stream; fixed uses the mean.
func sampleDist(sim *engine.Sim, d config.Dist, unit float64, fixed bool) engine.Duration {
	v := d.Mean
	if !fixed && d.SD > 0 {
		v += d.SD * sim.Stream(engine.StreamStationTimes).NormFloat64()
	}
	return engine.Seconds(math.Max(1, v*unit))
}
