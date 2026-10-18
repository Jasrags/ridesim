package park

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/coaster"
	"github.com/jasrags/ridesim/internal/ride/continuous"
)

// Scope is the source name of park-level events.
const Scope = "park"

// snapshotEvery is the in-park population sampling interval.
const snapshotEvery = 5 * engine.Minute

// Options are run-level settings.
type Options struct {
	Attendance      int     // guests for the day
	Scale           float64 // attendance multiplier (studies); 0 = 1
	FixedTimes      bool
	Failures        []string // failure kinds; each ride gets the ones its archetype supports
	WalkingMode     string   // "" = the park file's mode
	NoLightningLane bool
}

// Park is one park day: the graph, activities, the posted-wait board and the agents. Time 0 is
// park.open_hour.
type Park struct {
	root *engine.Sim
	sim  *engine.Sim // park scope
	cfg  *config.ParkFile
	opts Options
	g    *Graph
	tt   TravelTime

	acts      []activity        // by node index; nil for gate, plazas and exit
	board     []engine.Duration // posted waits guests see, by node index
	closed    []bool            // rides the board shows closed (long downtime), by node index
	ll        *lightningLane
	entrance  int
	exit      int
	elsewhere int // node index, -1 if none
	closeAt   engine.Time
	meals     [][2]engine.Time

	agents []*agent
	inPark int
	crowd  float64        // crowd scale; parties drawn at config.MaxCrowdScale x attendance arrive while thin < crowd/max
	states map[string]int // lookup by name only; snapshots iterate agentStates
	starts []func()
}

// New builds the park. ds supplies the ride files.
func New(root *engine.Sim, ds *config.Dataset, cfg *config.ParkFile, opts Options) (*Park, error) {
	if opts.WalkingMode != "" {
		c := *cfg
		c.Park.WalkingMode = opts.WalkingMode
		cfg = &c
	}
	g, err := NewGraph(cfg)
	if err != nil {
		return nil, err
	}
	p := &Park{root: root, sim: root.Scope(Scope), cfg: cfg, opts: opts, g: g, tt: NewTravelTime(cfg),
		acts: make([]activity, len(g.Nodes)), board: make([]engine.Duration, len(g.Nodes)), closed: make([]bool, len(g.Nodes)), elsewhere: -1,
		closeAt: engine.Time(engine.Seconds(float64(cfg.Park.CloseHour-cfg.Park.OpenHour) * 3600)), states: map[string]int{}, crowd: 1}
	openS := float64(cfg.Park.OpenHour * 3600)
	for _, w := range cfg.Decision.MealWindows {
		from, to, err := config.ParseClockWindow(w)
		if err != nil {
			return nil, err
		}
		p.meals = append(p.meals, [2]engine.Time{engine.Time(engine.Seconds(from - openS)), engine.Time(engine.Seconds(to - openS))})
	}
	for _, n := range g.Nodes {
		scope := root.Scope(n.ID)
		switch n.Kind {
		case config.NodeEntrance:
			p.entrance = n.Idx
		case config.NodeExit:
			p.exit = n.Idx
		case config.NodeRide:
			a, err := p.hostRide(ds, scope, n)
			if err != nil {
				return nil, err
			}
			p.acts[n.Idx] = a
		case config.NodeRideSimple, config.NodeShow, config.NodeDining, config.NodeElsewhere, config.NodeShop, config.NodeMeet:
			// Every activity scope opens with run_start, so per-node metrics work on its events.
			exitTo := ""
			if e := g.Edge(n.ExitEdge); e != nil {
				exitTo = g.Nodes[e.Other(n.Idx)].ID
			}
			scope.Emit(engine.Event{Type: engine.EvRunStart, Info: fmt.Sprintf(
				"scenario=park node=%s archetype=%s seats=0 load= brakes= preshow_batch=0 ll_phase=none exit_to=%s", n.ID, n.Kind, exitTo)})
		}
		switch n.Kind {
		case config.NodeRideSimple:
			s := newSimpleRide(scope, n)
			p.acts[n.Idx], p.starts = s, append(p.starts, s.start)
		case config.NodeShow:
			s := newShow(scope, n, openS)
			p.acts[n.Idx], p.starts = s, append(p.starts, s.start)
		case config.NodeDining:
			p.acts[n.Idx] = newDining(scope, n, opts.FixedTimes)
		case config.NodeElsewhere:
			p.acts[n.Idx], p.elsewhere = &elsewhere{n: n, sim: scope, fixed: opts.FixedTimes}, n.Idx
		case config.NodeMeet:
			p.acts[n.Idx] = newMeet(scope, n, opts.FixedTimes)
		case config.NodeShop:
			p.acts[n.Idx] = &elsewhere{n: n, sim: scope, fixed: opts.FixedTimes}
		}
	}
	for _, id := range cfg.Congestion.Spillback.AppliesTo {
		e := g.Edge(id)
		a := p.acts[e.from]
		if a == nil {
			a = p.acts[e.to]
		}
		if a != nil && a.queueCapacity() > 0 {
			e.spill = func() int { return max(0, a.queueGuests()-a.queueCapacity()) }
		}
	}
	if cfg.LightningLane != nil && !opts.NoLightningLane {
		ll, err := newLightningLane(p, cfg.LightningLane)
		if err != nil {
			return nil, err
		}
		p.ll = ll
	}
	return p, nil
}

// hostRide builds the full ride model for a ride node in its own scope and logs its run_start, so
// per-ride metrics work on the scope's events.
func (p *Park) hostRide(ds *config.Dataset, sim *engine.Sim, n *Node) (*rideAct, error) {
	rf := ds.Ride(n.RideID)
	if rf == nil {
		return nil, fmt.Errorf("park node %s: ride %s not found", n.ID, n.RideID)
	}
	a := &rideAct{n: n, sim: sim, info: rf.RideInfo()}
	failures := func(allowed ...string) []string {
		var out []string
		for _, f := range p.opts.Failures {
			for _, ok := range allowed {
				if f == ok {
					out = append(out, f)
				}
			}
		}
		return out
	}
	var info string
	switch rf.Kind {
	case config.KindCoaster:
		cfg := rf.Coaster
		c, err := coaster.New(sim, cfg, coaster.Options{FixedTimes: p.opts.FixedTimes,
			Failures: failures(config.FailBriefStop, config.FailLongDowntime, config.FailCascadeStop)})
		if err != nil {
			return nil, err
		}
		var brakes []string
		for _, b := range cfg.Blocks {
			if b.Kind == config.BlockBrake {
				brakes = append(brakes, b.ID)
			}
		}
		info = fmt.Sprintf("archetype=multi_train_coaster seats=%d load=%s brakes=%s", cfg.Trains.SeatsPerTrain, c.LoadBlock(), strings.Join(brakes, ","))
		a.r, a.capacity = c, cfg.Queues.Standby.PhysicalCapacity
		a.ll, a.single = cfg.Queues.SkipLine.Enabled, cfg.Queues.SingleRider.Enabled
		info += runInfoTail(cfg.Preshow, cfg.Queues)
	case config.KindContinuous:
		cfg := rf.Continuous
		c, err := continuous.New(sim, cfg, continuous.Options{FixedTimes: p.opts.FixedTimes,
			Failures: failures(config.FailChainStop, config.FailLongDowntime)})
		if err != nil {
			return nil, err
		}
		info = fmt.Sprintf("archetype=continuous_load seats=%d load=%s brakes=", cfg.Vehicles.SeatsMax, continuous.LoadZone)
		a.r, a.capacity = c, cfg.Queues.Standby.PhysicalCapacity
		a.ll, a.single = cfg.Queues.SkipLine.Enabled, cfg.Queues.SingleRider.Enabled
		info += runInfoTail(cfg.Preshow, cfg.Queues)
	default:
		return nil, fmt.Errorf("park node %s: %s is not a ride", n.ID, n.RideID)
	}
	sim.Emit(engine.Event{Type: engine.EvRunStart, Info: fmt.Sprintf("scenario=park ride=%s node=%s %s", n.RideID, n.ID, info)})
	return a, nil
}

func runInfoTail(ps *config.Preshow, q config.Queues) string {
	batch := 0
	if ps != nil && ps.Enabled {
		batch = ps.BatchSize
	}
	phase := "none"
	if q.SkipLine.Enabled && len(q.SkipLine.Merge.Phases) > 0 {
		phase = q.SkipLine.Merge.Phases[0].Name
	}
	return fmt.Sprintf(" preshow_batch=%d ll_phase=%s", batch, phase)
}

// CloseAt is closing time as an offset from opening.
func (p *Park) CloseAt() engine.Time { return p.closeAt }

// Graph exposes the layout (metrics and tests).
func (p *Park) Graph() *Graph { return p.g }

// Start opens the park: rides run, shows are scheduled, the posted-wait board refreshes, the day's
// parties are drawn, Lightning Lane advance bookings are made and arrivals are scheduled.
func (p *Park) Start() {
	for _, a := range p.acts {
		if ra, ok := a.(*rideAct); ok {
			ra.r.Start()
		}
	}
	for _, s := range p.starts {
		s()
	}
	for _, e := range p.g.Edges {
		// N = comfortable capacity C in guests (length x width x comfortable density), for congestion metrics.
		c := int(math.Round(e.LengthFt * e.WidthFt * p.cfg.Congestion.ComfortableDensityPerSqft))
		p.sim.Emit(engine.Event{Type: EvParkEdge, Block: e.ID, N: c, Info: p.g.Nodes[e.from].ID + "-" + p.g.Nodes[e.to].ID})
	}
	var refresh func()
	refresh = func() {
		for i, a := range p.acts {
			if a != nil {
				p.board[i] = a.posted()
			}
			if ra, ok := a.(*rideAct); ok {
				p.closed[i] = ra.closed()
			}
		}
		p.sim.After(engine.Seconds(p.cfg.Decision.PostedWaitRefreshS), refresh)
	}
	refresh()
	p.drawParties()
	if p.ll != nil {
		var expected []*agent // advance bookings only for parties expected at the opening crowd scale
		for _, a := range p.agents {
			if p.admits(a) {
				expected = append(expected, a)
			}
		}
		p.ll.advance(expected)
	}
	for _, a := range p.agents {
		a := a
		p.sim.At(a.arrive, func() { p.enter(a) })
	}
	var snap func()
	snap = func() {
		parts := make([]string, len(agentStates))
		for i, s := range agentStates {
			parts[i] = fmt.Sprintf("%s=%d", s, p.states[s])
		}
		p.sim.Emit(engine.Event{Type: EvParkSnapshot, N: p.inPark, Info: strings.Join(parts, " ")})
		p.sim.After(snapshotEvery, snap)
	}
	snap()
}

// InPark is guests in the park now.
func (p *Park) InPark() int { return p.inPark }

// drawParties draws the whole day's parties up front (Lightning Lane advance booking needs arrival
// times): a Poisson process per opening hour at attendance x share guests, party sizes and guest
// traits from the park file. Times use the arrivals stream, traits guest_choice.
func (p *Park) drawParties() {
	gc := p.cfg.Guests
	sizes := guest.PartySizesFrom(gc.PartySize)
	mean, total := 0.0, 0.0
	for _, s := range sizes {
		mean += float64(s.Size) * s.Weight
		total += s.Weight
	}
	mean /= total
	scale := p.opts.Scale
	if scale == 0 {
		scale = 1
	}
	// The day is drawn at MaxCrowdScale x attendance; each party keeps a uniform thin draw and arrives only
	// while thin < crowd / MaxCrowdScale, so the live crowd lever never shifts any other draw.
	attendance := float64(p.opts.Attendance) * scale * config.MaxCrowdScale
	singles := p.sim.Stream(streamSinglePass)
	times := p.sim.Stream(engine.StreamArrivals)
	traits := p.sim.Stream(engine.StreamGuestChoice)
	norm := func(d config.Dist, lo float64) float64 {
		v := d.Mean
		if !p.opts.FixedTimes && d.SD > 0 {
			v += d.SD * traits.NormFloat64()
		}
		return math.Max(lo, v)
	}
	var id int64
	for h := p.cfg.Park.OpenHour; h < p.cfg.Park.CloseHour; h++ {
		share := gc.ArrivalCurveShareByHour[strconv.Itoa(h)]
		rate := attendance * share / mean // parties per hour
		if rate <= 0 {
			continue
		}
		start := engine.Time(engine.Seconds(float64(h-p.cfg.Park.OpenHour) * 3600))
		end := start.Add(engine.Hour)
		for t := start; ; {
			t = t.Add(engine.Duration(math.Max(1, math.Round(times.ExpFloat64()*3600/rate*1000))))
			if t >= end {
				break
			}
			id++
			x, size := traits.Float64()*total, sizes[len(sizes)-1].Size
			for _, s := range sizes {
				if x -= s.Weight; x < 0 {
					size = s.Size
					break
				}
			}
			a := &agent{id: id, size: size, arrive: t, count: make([]int, len(p.g.Nodes)),
				meals: make([]bool, len(p.meals)), llBooked: make([]bool, len(p.g.Nodes))}
			a.thin = traits.Float64()
			a.patience = engine.Seconds(norm(gc.PatienceMin, 5) * 60)
			a.thrill = math.Min(1, norm(gc.ThrillTolerance, 0))
			a.leaveAt = t.Add(engine.Seconds(norm(gc.StayHours, 1) * 3600))
			if a.leaveAt > p.closeAt {
				a.leaveAt = p.closeAt
			}
			// Always draw the Lightning Lane traits, so turning LL off leaves every other draw unchanged.
			buy, passes := traits.Float64(), int(math.Max(1, math.Round(norm(gc.LLPassesPerGuest, 1))))
			if p.ll != nil && buy < p.cfg.LightningLane.PurchaseShare {
				a.passes = passes
			}
			p.drawSinglePasses(a, singles)
			p.agents = append(p.agents, a)
		}
	}
}

// admits reports whether a party arrives at the current crowd scale.
func (p *Park) admits(a *agent) bool { return a.thin < p.crowd/config.MaxCrowdScale }
