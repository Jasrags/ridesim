package park

import (
	"math"
	"sort"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// streamSingleRider is the park-scope stream for single-rider lane rolls.
const streamSingleRider = "single_rider"

// agent is one party in the park.
type agent struct {
	id       int64
	size     int
	arrive   engine.Time
	leaveAt  engine.Time
	patience engine.Duration
	thrill   float64

	at     int // current node
	visits int
	count  []int // visits (any outcome) per node
	meals  []bool
	state  string
	gone   bool

	passes   int // bookings still to make today
	held     []*booking
	llBooked []bool // per node: booked today
}

// Agent states for park snapshots, in report order.
var agentStates = []string{"walking", "edge_wait", config.NodeRide, config.NodeRideSimple, config.NodeShow, config.NodeDining, config.NodeElsewhere}

func (a *agent) drop(b *booking) bool {
	for i, h := range a.held {
		if h == b {
			a.held = append(a.held[:i], a.held[i+1:]...)
			return true
		}
	}
	return false
}

// heldFor is a booking for node n redeemable at t, or nil.
func (p *Park) heldFor(a *agent, n int, t engine.Time) *booking {
	if p.ll == nil {
		return nil
	}
	for _, b := range a.held {
		if b.node == n && p.ll.valid(b, t) {
			return b
		}
	}
	return nil
}

func (p *Park) setState(a *agent, s string) {
	if a.state != "" {
		p.states[a.state] -= a.size
	}
	a.state = s
	if s != "" {
		p.states[s] += a.size
	}
}

// enter admits an arriving party at the gate.
func (p *Park) enter(a *agent) {
	p.inPark += a.size
	a.at = p.entrance
	p.sim.Emit(engine.Event{Type: EvParkArrive, Guest: a.id, N: a.size})
	p.decide(a)
}

// decide picks the next activity, or heads for the exit.
func (p *Park) decide(a *agent) {
	now := p.sim.Now()
	if now >= a.leaveAt {
		p.walkTo(a, p.exit, func() { p.leave(a) })
		return
	}
	c := p.choose(a)
	if c < 0 {
		p.walkTo(a, p.exit, func() { p.leave(a) })
		return
	}
	p.sim.Emit(engine.Event{Type: EvChoose, Guest: a.id, Block: p.g.Nodes[c].ID})
	p.walkTo(a, c, func() { p.arriveAt(a, c) })
}

func (p *Park) leave(a *agent) {
	p.setState(a, "")
	a.gone = true
	a.held = nil // unused passes go home with the party: not a lapse, and the inventory stays sold
	p.inPark -= a.size
	p.sim.Emit(engine.Event{Type: EvParkLeave, Guest: a.id, N: a.size})
}

// arriveAt starts the visit at node c, redeeming a Lightning Lane booking when one is valid now.
func (p *Park) arriveAt(a *agent, c int) {
	act := p.acts[c]
	lane := guest.Standby
	// Drawn on every visit from its own stream, so toggling LL or single-rider lines never shifts choices.
	single := p.sim.Stream(streamSingleRider).Float64() < config.DefaultSingleRiderShare
	if b := p.heldFor(a, c, p.sim.Now()); b != nil {
		lane = guest.LightningLane
		p.ll.redeem(a, b)
	} else if ra, ok := act.(*rideAct); ok && a.size == 1 && ra.single && single {
		lane = guest.SingleRider
	}
	a.visits++
	party := &guest.Party{ID: a.id<<visitBits | int64(a.visits%(1<<visitBits)), Size: a.size, Patience: a.patience}
	party.OnExit = func(e guest.Exit) {
		p.sim.After(0, func() { p.visitDone(a, c, e) }) // never re-enter the activity synchronously
	}
	p.setState(a, p.g.Nodes[c].Kind)
	act.visit(party, lane)
}

func (p *Park) visitDone(a *agent, c int, e guest.Exit) {
	p.sim.Emit(engine.Event{Type: EvVisitEnd, Guest: a.id, Block: p.g.Nodes[c].ID, Info: e.String()})
	a.count[c]++
	n := p.g.Nodes[c]
	if n.Kind == config.NodeDining && e == guest.Rode {
		if w := p.mealWindow(p.sim.Now()); w >= 0 {
			a.meals[w] = true
		}
	}
	if n.Kind == config.NodeShow && e == guest.Rode && n.ExitEdge != "" {
		// The audience lets out by the exit edge (theater -> Haunted Mansion) and decides at its far end.
		ex := p.g.Edge(n.ExitEdge)
		p.walkEdges(a, []*Edge{ex}, func() { p.decide(a) })
		return
	}
	p.decide(a)
}

// walkTo walks the free-flow route to node b, then calls then.
func (p *Park) walkTo(a *agent, b int, then func()) { p.walkEdges(a, p.g.Route(a.at, b), then) }

func (p *Park) walkEdges(a *agent, route []*Edge, then func()) {
	if len(route) == 0 {
		then()
		return
	}
	e := route[0]
	if len(e.wait) > 0 || !p.cross(a, route, then) {
		// At jam density (or others already waiting): wait in line at the edge entrance, logged once.
		p.setState(a, "edge_wait")
		p.sim.Emit(engine.Event{Type: EvEdgeBlocked, Guest: a.id, Block: e.ID, N: a.size})
		e.wait = append(e.wait, waiter{a, route, then})
	}
}

// waiter is a party waiting to enter an edge at jam density.
type waiter struct {
	a     *agent
	route []*Edge
	then  func()
}

// cross enters route[0] if it has room and schedules the far end; false when the edge is jammed.
func (p *Park) cross(a *agent, route []*Edge, then func()) bool {
	e := route[0]
	d, ok := p.tt.Enter(e, a.size)
	if !ok {
		return false
	}
	p.setState(a, "walking")
	p.sim.Emit(engine.Event{Type: EvWalk, Guest: a.id, Block: e.ID, N: a.size, D: d})
	p.sim.After(d, func() {
		p.tt.Leave(e, a.size)
		a.at = e.Other(a.at)
		p.wake(e)
		p.walkEdges(a, route[1:], then)
	})
	return true
}

// wake admits parties waiting at e in arrival order, stopping at the first that still does not fit.
func (p *Park) wake(e *Edge) {
	for len(e.wait) > 0 {
		w := e.wait[0]
		if !p.cross(w.a, w.route, w.then) {
			return
		}
		e.wait = e.wait[1:]
	}
}

// ---------------------------------------------------------------------------------------------------
// Choice (spec §9): softmax over the top k by utility = appeal + thrill match - wait penalty (posted
// wait from the board, refreshed every posted_wait_refresh_s) - walk penalty (raised by fatigue) -
// repeat penalty + Lightning Lane return-window bonus + meal-window bonus for dining.

type option struct {
	node int
	u    float64
}

// choose returns the chosen node, or -1 when nothing is available.
func (p *Park) choose(a *agent) int {
	d := p.cfg.Decision
	w := d.Utility
	now := p.sim.Now()
	fatigue := d.FatiguePerHour * now.Sub(a.arrive).Seconds() / 3600
	var opts []option
	for _, act := range p.acts {
		if act == nil {
			continue
		}
		n := act.node()
		if p.closed[n.Idx] {
			continue
		}
		walkS := p.g.Distance(a.at, n.Idx) / p.cfg.Congestion.FreeSpeedFps
		eta := now.Add(engine.Seconds(walkS))
		waitMin := p.board[n.Idx].Seconds() / 60
		u := w.Appeal*n.Appeal + w.ThrillMatch*(1-math.Abs(n.Thrill-a.thrill))
		switch n.Kind {
		case config.NodeShow:
			sh := act.(*show)
			at := sh.nextStart()
			if at < 0 || at < eta {
				continue
			}
			waitMin = at.Sub(eta).Seconds() / 60
		case config.NodeDining:
			if mw := p.mealWindow(now); mw >= 0 && !a.meals[mw] {
				u += d.MealWindowBonus
			}
		}
		if b := p.heldFor(a, n.Idx, eta); b != nil {
			waitMin = 0 // guests expect a short Lightning Lane wait
			u += w.LLReturnWindowBonus
		}
		if n.Kind != config.NodeElsewhere {
			u -= w.RepeatPenalty * float64(a.count[n.Idx])
		}
		u -= w.WaitPenaltyPerMin*waitMin + w.WalkPenaltyPerMin*walkS/60*(1+fatigue)
		opts = append(opts, option{n.Idx, u})
	}
	if len(opts) == 0 {
		return -1
	}
	sort.SliceStable(opts, func(i, j int) bool { return opts[i].u > opts[j].u })
	if len(opts) > d.K {
		opts = opts[:d.K]
	}
	total := 0.0
	weights := make([]float64, len(opts))
	for i, o := range opts {
		weights[i] = math.Exp((o.u - opts[0].u) / d.Temperature)
		total += weights[i]
	}
	x := p.sim.Stream(engine.StreamGuestChoice).Float64() * total
	for i, wt := range weights {
		if x -= wt; x < 0 {
			return opts[i].node
		}
	}
	return opts[len(opts)-1].node
}

// mealWindow is the index of the meal window containing t, or -1.
func (p *Park) mealWindow(t engine.Time) int {
	for i, w := range p.meals {
		if t >= w[0] && t < w[1] {
			return i
		}
	}
	return -1
}
