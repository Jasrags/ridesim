// Package coaster implements the multi_train_coaster archetype (spec §6): a closed loop of blocks,
// each holding at most one train; brakes that stop only when the way ahead is blocked; clearance
// rules and switch realign gating; the shadow-room hold_until_clear; and a pipelined UNLOAD/LOAD
// station with hold_for_full_or_max_wait dispatch.
//
// Movement model: a train entering a block does that block's work (transit, unload, or load), then
// asks to exit. If the exit is allowed it moves to the next block instantly (trains are points).
// Otherwise it is held where it is (logged as hold_start/hold_end); a held train on a brake block
// leaves release_delay_s after the way clears. Failures, barn swaps, single rider and Lightning Lane
// come in later milestones.
package coaster

import (
	"fmt"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/queue"
	"github.com/jasrags/ridesim/internal/ride"
)

// holdUntilClearMinTrains: the shadow-room hold is ignored with fewer trains (spec §6).
const holdUntilClearMinTrains = 5

// Options are run-level settings that are not part of the ride file.
type Options struct {
	Trains     int      // trains in service; 0 means trains.default_in_service
	FixedTimes bool     // use every distribution's mean (sd = 0)
	Failures   []string // enabled failure kinds (config.Fail*); nil = failures off

	CascadeScope      string  // CascadeBrakesAndStations (default) or CascadeBrakesOnly
	CascadeToleranceS float64 // overrides failures.cascade_stop.blocked_tolerance_s when > 0 (analysis only)
}

// Hold reasons recorded in hold_start events.
const (
	ReasonNextOccupied   = "next_occupied"
	ReasonClearance      = "clearance"
	ReasonSwitchRealign  = "switch_realign"
	ReasonHoldUntilClear = "hold_until_clear"
	ReasonMinInterval    = "min_dispatch_interval"
	ReasonRideStopped    = "ride_stopped"
)

type role int

const (
	roleTrack  role = iota
	roleUnload      // station block followed by another station block
	roleLoad        // station block followed by track
	roleBoth        // lone station block: unload then load
)

type switchState struct {
	id        string
	alignedAt engine.Time
}

type block struct {
	cfg      *config.Block
	role     role
	next     *block
	occupant *train
	switches []*switchState
	rule     *exitRule
}

type exitRule struct {
	id       string
	requires []*block
	switches []*switchState
}

type stationPhase int

const (
	phaseNone stationPhase = iota
	phaseUnloading
	phaseBoarding // load_s window, guests board
	phaseFillWait // load_s done, train not full, waiting up to max_wait_after_ready_s
	phaseChecking // restraint check + dispatch clearance
)

type train struct {
	id    int
	at    *block
	seats int

	riders   int
	workDone bool // finished this block's work and wants to exit

	held       bool
	holdReason string
	holdSeq    uint64 // increments on every hold segment; stale cascade checks compare it
	releasing  bool   // brake release delay running

	phase stationPhase
	work  work // the one pending timer for this train's current block (transit, station phase, release)

	// hold_until_clear bookkeeping, set on entry to such a block.
	shadowSnapshot int
	shadowFree     bool
}

// Coaster is one multi_train_coaster ride. It implements ride.Ride.
type Coaster struct {
	sim  *engine.Sim
	cfg  *config.CoasterFile
	opts Options

	blocks   []*block // file order, which is loop order starting at the first station block
	byID     map[string]*block
	switches map[string]*switchState
	trains   []*train
	load     *block

	standby *queue.FIFO

	lastDispatch   engine.Time
	dispatched     bool
	unloadAdvances int
	kicking        bool
	rekick         bool

	stopCount int            // active stop causes; the ride runs only at 0
	stopDepth map[string]int // per cause, lookups only

	cascadeEnabled bool
}

var _ ride.Ride = (*Coaster)(nil)

// New builds the model. The config must have passed validation.
func New(sim *engine.Sim, cfg *config.CoasterFile, opts Options) (*Coaster, error) {
	if opts.Trains == 0 {
		opts.Trains = cfg.Trains.DefaultInService
	}
	if opts.Trains < 1 || opts.Trains > cfg.Trains.MaxInService || opts.Trains > len(cfg.Blocks)-1 {
		return nil, fmt.Errorf("coaster %s: %d trains in service, allowed 1..%d", cfg.Ride.ID, opts.Trains, min(cfg.Trains.MaxInService, len(cfg.Blocks)-1))
	}
	c := &Coaster{sim: sim, cfg: cfg, opts: opts, byID: map[string]*block{}, switches: map[string]*switchState{}, standby: &queue.FIFO{}, stopDepth: map[string]int{}}
	for i := range cfg.Blocks {
		b := &block{cfg: &cfg.Blocks[i]}
		for _, sid := range b.cfg.Contains {
			s := &switchState{id: sid}
			c.switches[sid] = s
			b.switches = append(b.switches, s)
		}
		c.blocks = append(c.blocks, b)
		c.byID[b.cfg.ID] = b
	}
	for _, b := range c.blocks {
		b.next = c.byID[b.cfg.Next]
		if b.next == nil {
			return nil, fmt.Errorf("coaster %s: block %s.next %q unknown", cfg.Ride.ID, b.cfg.ID, b.cfg.Next)
		}
	}
	prev := map[*block]*block{}
	for _, b := range c.blocks {
		prev[b.next] = b
	}
	for _, b := range c.blocks {
		b.role = stationRole(b, prev[b])
		if b.role == roleLoad || b.role == roleBoth {
			c.load = b
		}
	}
	if c.load == nil {
		return nil, fmt.Errorf("coaster %s: no load station", cfg.Ride.ID)
	}
	for _, r := range cfg.Clearance.Rules {
		from := c.byID[r.From]
		er := &exitRule{id: r.ID}
		for _, id := range r.RequiresEmpty {
			er.requires = append(er.requires, c.byID[id])
		}
		for _, cond := range r.Conditions {
			if cond.Kind == config.CondSwitchAligned {
				er.switches = append(er.switches, c.switches[cond.Switch])
			}
		}
		from.rule = er
	}
	return c, nil
}

func stationRole(b, prev *block) role {
	switch {
	case b.cfg.Kind != config.BlockStation:
		return roleTrack
	case b.next.cfg.Kind == config.BlockStation:
		return roleUnload
	case prev != nil && prev.cfg.Kind == config.BlockStation:
		return roleLoad
	default:
		return roleBoth
	}
}

// ID implements ride.Ride.
func (c *Coaster) ID() string { return c.cfg.Ride.ID }

// Status implements ride.Ride. Failures arrive in M2, so the ride is always open.
func (c *Coaster) Status() ride.Status { return ride.Open }

// PostedWait implements ride.Ride with queue_len_over_throughput against the target dispatch
// interval. Posted-wait algorithms are M2 work; this is the default.
func (c *Coaster) PostedWait() engine.Duration {
	perHr := float64(c.cfg.Trains.SeatsPerTrain) * 3600 / c.cfg.Station.TargetDispatchIntervalS
	return engine.Seconds(float64(c.standby.Guests()) / perHr * 3600)
}

// Start places the trains and begins operating. Trains are placed one per block walking backwards
// from the load station (LOAD, UNLOAD, the brake before unload, ...) and begin that block's work at t=0.
func (c *Coaster) Start() {
	prev := map[*block]*block{}
	for _, b := range c.blocks {
		prev[b.next] = b
	}
	b := c.load
	placed := make([]*block, 0, c.opts.Trains)
	for range c.opts.Trains {
		placed = append(placed, b)
		b = prev[b]
	}
	for i, pb := range placed {
		t := &train{id: i + 1, seats: c.cfg.Trains.SeatsPerTrain}
		c.trains = append(c.trains, t)
		c.enter(t, pb)
	}
	c.startFailures()
	c.kick()
}

// Arrive implements ride.Ride. Only standby exists in M1.
func (c *Coaster) Arrive(p *guest.Party, lane guest.Lane) {
	if lane != guest.Standby {
		panic(fmt.Sprintf("coaster %s: lane %v not modeled until M3/M4", c.ID(), lane))
	}
	p.JoinedAt = c.sim.Now()
	c.sim.Emit(engine.Event{Type: engine.EvQueueJoin, Guest: p.ID, N: p.Size, Info: lane.String()})
	c.standby.Push(p)
	if t := c.load.occupant; t != nil && !c.Stopped() && (t.phase == phaseBoarding || t.phase == phaseFillWait) {
		c.board(t)
		if t.phase == phaseFillWait && t.riders == t.seats {
			c.cancelWork(t)
			c.startCheck(t)
		}
	}
}

// LoadBlock is the id of the block trains are dispatched from.
func (c *Coaster) LoadBlock() string { return c.load.cfg.ID }

// QueueGuests is the number of guests in standby.
func (c *Coaster) QueueGuests() int { return c.standby.Guests() }

// ---------------------------------------------------------------------------------------------------
// Block entry and work

func (c *Coaster) enter(t *train, b *block) {
	b.occupant = t
	t.at = b
	t.workDone = false
	c.sim.Emit(engine.Event{Type: engine.EvTrainEnter, Train: t.id, Block: b.cfg.ID})

	if b.cfg.HoldUntilClear {
		t.shadowSnapshot = c.unloadAdvances
		t.shadowFree = c.unloadOccupant() == nil
	}

	switch b.role {
	case roleTrack:
		c.startWork(t, c.sampleTrack(*b.cfg.TransitS), func() { c.workComplete(t) })
	case roleUnload:
		c.startUnload(t, func() { c.workComplete(t) })
	case roleLoad:
		c.startLoad(t)
	case roleBoth:
		c.startUnload(t, func() { c.startLoad(t) })
	}
}

func (c *Coaster) startUnload(t *train, then func()) {
	t.phase = phaseUnloading
	if t.riders > 0 {
		c.sim.Emit(engine.Event{Type: engine.EvUnload, Train: t.id, Block: t.at.cfg.ID, N: t.riders})
		t.riders = 0
	}
	c.startWork(t, c.sampleStation(c.cfg.Station.UnloadS), func() {
		t.phase = phaseNone
		then()
	})
}

func (c *Coaster) startLoad(t *train) {
	t.phase = phaseBoarding
	if !c.Stopped() {
		c.board(t)
	}
	c.startWork(t, c.sampleStation(c.cfg.Station.LoadS), func() {
		c.sim.Emit(engine.Event{Type: engine.EvLoadDone, Train: t.id, Block: t.at.cfg.ID, N: t.riders})
		if t.riders == t.seats || c.cfg.Station.DispatchRule != config.DispatchHoldForFullOrMaxWait || c.cfg.Station.MaxWaitAfterReadyS <= 0 {
			c.startCheck(t)
			return
		}
		t.phase = phaseFillWait
		c.startWork(t, engine.Seconds(c.cfg.Station.MaxWaitAfterReadyS), func() { c.startCheck(t) })
	})
}

func (c *Coaster) startCheck(t *train) {
	t.phase = phaseChecking
	d := c.sampleStation(c.cfg.Station.RestraintCheckS) + c.sampleStation(c.cfg.Station.DispatchClearanceS)
	c.startWork(t, d, func() {
		t.phase = phaseNone
		c.workComplete(t)
	})
}

// board fills t from the front of standby. Parties are never split; boarding stops at the first
// party that does not fit (no seat-filling around it until single rider exists, M3).
func (c *Coaster) board(t *train) {
	for {
		p := c.standby.Peek()
		if p == nil || p.Size > t.seats-t.riders {
			return
		}
		c.standby.Pop()
		t.riders += p.Size
		c.sim.Emit(engine.Event{Type: engine.EvBoard, Train: t.id, Block: t.at.cfg.ID, Guest: p.ID, N: p.Size})
	}
}

func (c *Coaster) workComplete(t *train) {
	t.workDone = true
	c.kick()
}

// ---------------------------------------------------------------------------------------------------
// Exit gating

// kick retries every train that wants to exit until nothing changes. Re-entrant calls (a move that
// triggers another kick) set a flag and are absorbed by the outer loop, so ordering stays by train id.
func (c *Coaster) kick() {
	if c.kicking {
		c.rekick = true
		return
	}
	c.kicking = true
	for {
		c.rekick = false
		for _, t := range c.trains {
			c.tryExit(t)
		}
		if !c.rekick {
			break
		}
	}
	c.kicking = false
}

func (c *Coaster) tryExit(t *train) {
	if !t.workDone || t.releasing {
		return
	}
	ok, reason, retryAt := c.canExit(t)
	if !ok {
		if !t.held || t.holdReason != reason {
			if t.held {
				c.sim.Emit(engine.Event{Type: engine.EvHoldEnd, Train: t.id, Block: t.at.cfg.ID})
			}
			t.held, t.holdReason = true, reason
			t.holdSeq++
			c.sim.Emit(engine.Event{Type: engine.EvHoldStart, Train: t.id, Block: t.at.cfg.ID, Info: reason})
			c.watchCascade(t)
			c.rekick = true // a newly held train can let a hold_until_clear yield (blockingUpstream)
		}
		if retryAt > c.sim.Now() {
			c.sim.At(retryAt, c.kick)
		}
		return
	}
	if t.held && t.at.cfg.Kind == config.BlockBrake && t.at.cfg.Brake != nil && t.at.cfg.Brake.ReleaseDelayS != nil {
		t.releasing = true
		c.startWork(t, c.sampleTrack(*t.at.cfg.Brake.ReleaseDelayS), func() {
			t.releasing = false
			if ok, _, _ := c.canExit(t); ok {
				c.depart(t)
			}
			c.kick()
		})
		return
	}
	c.depart(t)
	c.rekick = true
}

// canExit reports whether t may leave its block now. When the blocker is a timer (switch realign,
// minimum dispatch interval) retryAt says when to look again.
func (c *Coaster) canExit(t *train) (ok bool, reason string, retryAt engine.Time) {
	b := t.at
	now := c.sim.Now()
	if c.Stopped() && pausesAt(b) {
		return false, ReasonRideStopped, 0
	}
	if b.next.occupant != nil {
		return false, ReasonNextOccupied, 0
	}
	if r := b.rule; r != nil {
		for _, rb := range r.requires {
			if rb.occupant != nil {
				return false, ReasonClearance, 0
			}
		}
		for _, s := range r.switches {
			if s.alignedAt > now {
				return false, ReasonSwitchRealign, s.alignedAt
			}
		}
	}
	for _, s := range b.next.switches { // a train may not enter a block whose switch is still realigning
		if s.alignedAt > now {
			return false, ReasonSwitchRealign, s.alignedAt
		}
	}
	if c.shadowHoldPending(t) && !c.blockingUpstream(b) {
		return false, ReasonHoldUntilClear, 0
	}
	if b == c.load && c.dispatched {
		if earliest := c.lastDispatch.Add(engine.Seconds(c.cfg.Station.MinDispatchIntervalS)); earliest > now {
			return false, ReasonMinInterval, earliest
		}
	}
	return true, "", 0
}

// shadowHoldPending reports whether hold_until_clear would still hold t: no UNLOAD->LOAD advance
// since t arrived, and UNLOAD was occupied when it did.
func (c *Coaster) shadowHoldPending(t *train) bool {
	return t.at.cfg.HoldUntilClear && len(c.trains) >= holdUntilClearMinTrains && !t.shadowFree && c.unloadAdvances <= t.shadowSnapshot
}

// blockingUpstream reports whether some other train has finished its work and is held because it
// needs b empty. With 5 trains the shadow-room hold and the [fan] clearance rules form a circular
// wait (shadow hold -> LOAD needs PEAK_BRAKE empty -> PEAK needs SHADOW_BRAKE empty), which the real
// ride does not exhibit, so the hold yields in that case (decision 2026-10-17).
func (c *Coaster) blockingUpstream(b *block) bool {
	for _, u := range c.trains {
		if u.at == b || !u.workDone || !u.held {
			continue
		}
		if u.at.next == b {
			return true
		}
		if u.at.rule != nil {
			for _, rb := range u.at.rule.requires {
				if rb == b {
					return true
				}
			}
		}
	}
	return false
}

func (c *Coaster) depart(t *train) {
	from := t.at
	if c.shadowHoldPending(t) {
		c.sim.Emit(engine.Event{Type: engine.EvHoldYield, Train: t.id, Block: from.cfg.ID, Info: ReasonHoldUntilClear})
	}
	if t.held {
		c.sim.Emit(engine.Event{Type: engine.EvHoldEnd, Train: t.id, Block: from.cfg.ID})
		t.held, t.holdReason = false, ""
	}
	if from == c.load {
		c.sim.Emit(engine.Event{Type: engine.EvDispatch, Train: t.id, Block: from.cfg.ID, N: t.riders})
		c.lastDispatch, c.dispatched = c.sim.Now(), true
	}
	if from.role == roleUnload {
		c.unloadAdvances++
	}
	from.occupant = nil
	if len(from.switches) > 0 && c.cfg.Clearance.SwitchRealignS != nil {
		d := c.sampleTrack(*c.cfg.Clearance.SwitchRealignS)
		for _, s := range from.switches {
			s.alignedAt = c.sim.Now().Add(d)
		}
	}
	c.enter(t, from.next)
}

func (c *Coaster) unloadOccupant() *train {
	for _, b := range c.blocks {
		if b.role == roleUnload {
			return b.occupant
		}
	}
	return nil
}

// ---------------------------------------------------------------------------------------------------
// Timing

func (c *Coaster) sampleStation(d config.Dist) engine.Duration {
	return c.sample(d, engine.StreamStationTimes)
}

func (c *Coaster) sampleTrack(d config.Dist) engine.Duration {
	return c.sample(d, engine.StreamTrackTimes)
}

// sample draws from a normal truncated at zero; FixedTimes uses the mean and consumes no randomness.
func (c *Coaster) sample(d config.Dist, stream string) engine.Duration {
	if c.opts.FixedTimes || d.SD == 0 {
		return engine.Seconds(d.Mean)
	}
	v := d.Mean + d.SD*c.sim.Stream(stream).NormFloat64()
	return engine.Seconds(max(0, v))
}
