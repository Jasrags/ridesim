package park

import (
	"fmt"
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// lightningLane issues return windows (decision 2026-10-17, [user]): purchasing parties book up to
// advance_bookings before the day in random order, each the earliest window with inventory starting at
// or after their arrival; afterwards one more each time a pass is used or lapses. A party books each
// modeled ride at most once a day. elsewhere_share of bookings are for attractions outside the modeled
// rides: unlimited inventory, redeemed as an elsewhere visit.
type lightningLane struct {
	p      *Park
	cfg    *config.ParkLightningLane
	window engine.Duration
	grace  engine.Duration
	rides  []*llRide // node order
}

type llRide struct {
	node   int
	appeal float64
	inv    []int // guests still sellable per window
}

type booking struct {
	node       int
	start, end engine.Time
}

func newLightningLane(p *Park, cfg *config.ParkLightningLane) *lightningLane {
	ll := &lightningLane{p: p, cfg: cfg, window: engine.Seconds(cfg.WindowMin * 60), grace: engine.Seconds(cfg.GraceMin * 60)}
	windows := int(math.Ceil(float64(p.closeAt) / float64(ll.window)))
	for _, a := range p.acts {
		ra, ok := a.(*rideAct)
		if !ok || !ra.ll {
			continue
		}
		per := int(math.Round(cfg.InventoryShare * ra.info.ListedCapacityPerHr * ll.window.Seconds() / 3600))
		r := &llRide{node: ra.n.Idx, appeal: ra.n.Appeal, inv: make([]int, windows)}
		for i := range r.inv {
			r.inv[i] = per
		}
		ll.rides = append(ll.rides, r)
	}
	return ll
}

// advance runs the before-the-day bookings in a random party order.
func (ll *lightningLane) advance(agents []*agent) {
	var buyers []*agent
	for _, a := range agents {
		if a.passes > 0 {
			buyers = append(buyers, a)
		}
	}
	rng := ll.p.sim.Stream(engine.StreamLLChoice)
	rng.Shuffle(len(buyers), func(i, j int) { buyers[i], buyers[j] = buyers[j], buyers[i] })
	for _, a := range buyers {
		a.advanced = true
		for i := 0; i < ll.cfg.AdvanceBookings && a.passes > 0; i++ {
			if !ll.book(a, a.arrive) {
				break
			}
		}
	}
}

// book makes one booking for a starting at or after from; false when nothing is available.
func (ll *lightningLane) book(a *agent, from engine.Time) bool {
	if a.passes <= 0 {
		return false
	}
	// Each party draws from its own stream, two values per booking, so what one party books never
	// shifts another party's draws (common random numbers across variants).
	rng := ll.p.sim.Stream(fmt.Sprintf("%s/%d", engine.StreamLLChoice, a.id))
	toElsewhere, x0 := rng.Float64() < ll.cfg.ElsewhereShare, rng.Float64()
	if ll.p.elsewhere >= 0 && toElsewhere {
		w := ll.windowAt(from)
		if engine.Time(w)*engine.Time(ll.window) >= ll.p.closeAt {
			return false
		}
		ll.take(a, &booking{node: ll.p.elsewhere, start: engine.Time(w) * engine.Time(ll.window)})
		return true
	}
	// Rides not yet booked today, weighted by appeal; try them in a drawn order until one has a window.
	var cands []*llRide
	weight := 0.0
	for _, r := range ll.rides {
		if !a.llBooked[r.node] {
			cands = append(cands, r)
			weight += r.appeal
		}
	}
	for len(cands) > 0 {
		x, pick := x0*weight, len(cands)-1
		for i, r := range cands {
			if x -= r.appeal; x < 0 {
				pick = i
				break
			}
		}
		r := cands[pick]
		for w := ll.windowAt(from); w < len(r.inv); w++ {
			if r.inv[w] >= a.size {
				r.inv[w] -= a.size
				a.llBooked[r.node] = true
				ll.take(a, &booking{node: r.node, start: engine.Time(w) * engine.Time(ll.window)})
				return true
			}
		}
		weight -= r.appeal
		cands = append(cands[:pick:pick], cands[pick+1:]...)
	}
	return false
}

// windowAt is the first window starting at or after t.
func (ll *lightningLane) windowAt(t engine.Time) int {
	return int((int64(t) + int64(ll.window) - 1) / int64(ll.window))
}

func (ll *lightningLane) take(a *agent, b *booking) {
	b.end = b.start.Add(ll.window)
	a.passes--
	a.held = append(a.held, b)
	ll.p.sim.Emit(engine.Event{Type: EvLLBook, Guest: a.id, N: int(b.start / engine.Time(engine.Second)), Block: ll.p.g.Nodes[b.node].ID})
	lapse := b.end.Add(ll.grace)
	if lapse < ll.p.sim.Now() {
		lapse = ll.p.sim.Now()
	}
	ll.p.sim.At(lapse, func() {
		if a.drop(b) {
			ll.p.sim.Emit(engine.Event{Type: EvLLLapse, Guest: a.id, Block: ll.p.g.Nodes[b.node].ID})
			if !a.gone {
				ll.book(a, ll.p.sim.Now())
			}
		}
	})
}

// valid reports whether b can be redeemed at t (window open or within grace).
func (ll *lightningLane) valid(b *booking, t engine.Time) bool {
	return b.start <= t && t <= b.end.Add(ll.grace)
}

// redeem uses b now and books the next pass.
func (ll *lightningLane) redeem(a *agent, b *booking) {
	a.drop(b)
	ll.p.sim.Emit(engine.Event{Type: EvLLRedeem, Guest: a.id, Block: ll.p.g.Nodes[b.node].ID})
	ll.book(a, ll.p.sim.Now())
}
