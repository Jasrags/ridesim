package park

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// lightningLane issues return windows (decision 2026-10-17, [user]): purchasing parties book up to
// advance_bookings before the day in random order, each the earliest window with inventory starting at
// or after their arrival; afterwards one more each time a pass is used or lapses. A party books each
// modeled ride at most once a day. elsewhere_share of bookings are for attractions outside the modeled
// rides: unlimited inventory, redeemed as an elsewhere visit.
//
// With tiers (M9, Jason): rides declare tier1, tier2 or single_pass. Multi Pass buyers book
// advance_by_tier passes per tier before the day, then rebook any Multi Pass ride after each use or
// lapse. single_pass_share of parties also buy each Single Pass ride: one booking, never rebooked.
type lightningLane struct {
	p      *Park
	cfg    *config.ParkLightningLane
	tiers  bool
	window engine.Duration
	grace  engine.Duration
	rides  []*llRide // node order
}

type llRide struct {
	node     int
	tier     string // "" in an untiered park
	appeal   float64
	capacity float64 // guests/hr the inventory share applies to
	per      int     // guests sellable per window at the current share
	inv      []int   // guests still sellable per window
}

type booking struct {
	node       int
	start, end engine.Time
	single     bool // a Single Pass: does not use up a Multi Pass and is not rebooked
}

func newLightningLane(p *Park, cfg *config.ParkLightningLane) (*lightningLane, error) {
	ll := &lightningLane{p: p, cfg: cfg, tiers: p.cfg.HasLLTiers(), window: engine.Seconds(cfg.WindowMin * 60), grace: engine.Seconds(cfg.GraceMin * 60)}
	windows := int(math.Ceil(float64(p.closeAt) / float64(ll.window)))
	for _, a := range p.acts {
		var n *Node
		var capacity float64
		switch act := a.(type) {
		case *rideAct:
			if ll.tiers && act.n.LightningLane == "" || !ll.tiers && !act.ll {
				continue
			}
			if !act.ll {
				return nil, fmt.Errorf("%s: lightning_lane %s but its ride has no Lightning Lane line", act.n.ID, act.n.LightningLane)
			}
			n, capacity = act.n, act.info.ListedCapacityPerHr
		case *simpleRide:
			if !ll.tiers || act.n.LightningLane == "" {
				continue
			}
			n, capacity = act.n, act.n.CapacityPerHr
		default:
			continue
		}
		per := int(math.Round(cfg.InventoryShare * capacity * ll.window.Seconds() / 3600))
		r := &llRide{node: n.Idx, tier: n.LightningLane, appeal: n.Appeal, capacity: capacity, per: per, inv: make([]int, windows)}
		for i := range r.inv {
			r.inv[i] = per
		}
		ll.rides = append(ll.rides, r)
	}
	return ll, nil
}

// ride is the inventory of node, or nil when it sells no Lightning Lane.
func (ll *lightningLane) ride(node int) *llRide {
	for _, r := range ll.rides {
		if r.node == node {
			return r
		}
	}
	return nil
}

// setShare is the ll_inventory lever: windows starting at or after now sell share of capacity. Passes
// already sold stay valid, so a window can be left with nothing to sell.
func (ll *lightningLane) setShare(r *llRide, share float64, now engine.Time) {
	per := int(math.Round(share * r.capacity * ll.window.Seconds() / 3600))
	for w := ll.windowAt(now); w < len(r.inv); w++ {
		sold := r.per - r.inv[w]
		r.inv[w] = max(0, per-sold)
	}
	r.per = per
}

// advance runs the before-the-day bookings in a random party order.
func (ll *lightningLane) advance(agents []*agent) {
	var buyers []*agent
	for _, a := range agents {
		if a.passes > 0 || len(a.singles) > 0 {
			buyers = append(buyers, a)
		}
	}
	rng := ll.p.sim.Stream(engine.StreamLLChoice)
	rng.Shuffle(len(buyers), func(i, j int) { buyers[i], buyers[j] = buyers[j], buyers[i] })
	for _, a := range buyers {
		ll.advanceFor(a, a.arrive)
	}
}

// advanceFor makes a party's before-the-day bookings, starting at or after from.
func (ll *lightningLane) advanceFor(a *agent, from engine.Time) {
	a.advanced = true
	if !ll.tiers {
		for i := 0; i < ll.cfg.AdvanceBookings && a.passes > 0; i++ {
			if !ll.book(a, from, "") {
				break
			}
		}
		return
	}
	for _, node := range a.singles {
		ll.bookSingle(a, from, node)
	}
	for _, tier := range []string{config.LLTier1, config.LLTier2} {
		for i := 0; i < ll.cfg.AdvanceByTier[tier] && a.passes > 0; i++ {
			if !ll.book(a, from, tier) {
				break
			}
		}
	}
}

// bookSingle books the Single Pass ride node in its earliest window with room at or after from.
func (ll *lightningLane) bookSingle(a *agent, from engine.Time, node int) {
	for _, r := range ll.rides {
		if r.node != node || a.llBooked[node] {
			continue
		}
		for w := ll.windowAt(from); w < len(r.inv); w++ {
			if r.inv[w] >= a.size {
				r.inv[w] -= a.size
				a.llBooked[node] = true
				ll.take(a, &booking{node: node, start: engine.Time(w) * engine.Time(ll.window), single: true})
				return
			}
		}
	}
}

// book makes one Multi Pass booking for a starting at or after from, from rides of the given tier ("" =
// any Multi Pass ride); false when nothing is available.
func (ll *lightningLane) book(a *agent, from engine.Time, tier string) bool {
	if a.passes <= 0 {
		return false
	}
	// Each party draws from its own stream, two values per booking, so what one party books never
	// shifts another party's draws (common random numbers across variants).
	rng := ll.p.sim.Stream(fmt.Sprintf("%s/%d", engine.StreamLLChoice, a.id))
	toElsewhere, x0 := rng.Float64() < ll.cfg.ElsewhereShare, rng.Float64()
	// In a tiered park the elsewhere share stands for Tier 2 shows: Tier 1 advance bookings skip it.
	if ll.p.elsewhere >= 0 && toElsewhere && tier != config.LLTier1 {
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
		if !a.llBooked[r.node] && r.tier != config.LLSinglePass && (tier == "" || r.tier == tier) {
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
	if !b.single {
		a.passes--
	}
	a.held = append(a.held, b)
	ll.p.sim.Emit(engine.Event{Type: EvLLBook, Guest: a.id, N: int(b.start / engine.Time(engine.Second)), Block: ll.p.g.Nodes[b.node].ID})
	lapse := b.end.Add(ll.grace)
	if lapse < ll.p.sim.Now() {
		lapse = ll.p.sim.Now()
	}
	ll.p.sim.At(lapse, func() {
		if a.drop(b) {
			ll.p.sim.Emit(engine.Event{Type: EvLLLapse, Guest: a.id, Block: ll.p.g.Nodes[b.node].ID})
			if !a.gone && !b.single {
				ll.book(a, ll.p.sim.Now(), "")
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
	if !b.single {
		ll.book(a, ll.p.sim.Now(), "")
	}
}

// streamSinglePass is the park-scope stream for Single Pass purchases: one draw per party per Single
// Pass ride, only in tiered parks, so it never shifts other draws.
const streamSinglePass = "ll_single_pass"

// drawSinglePasses decides which Single Pass rides a party buys.
func (p *Park) drawSinglePasses(a *agent, rng *rand.Rand) {
	if p.cfg.LightningLane == nil || !p.cfg.HasLLTiers() {
		return
	}
	for _, n := range p.g.Nodes {
		if n.LightningLane != config.LLSinglePass {
			continue
		}
		if rng.Float64() < p.cfg.LightningLane.SinglePassShare && p.ll != nil {
			a.singles = append(a.singles, n.Idx)
		}
	}
}
