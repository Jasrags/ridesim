// Package continuous implements the continuous_load archetype (spec §7), an omnimover such as the
// Haunted Mansion: a closed chain of vehicles.count buggies spaced vehicles.spacing_s apart, so one
// buggy reaches the load zone every spacing_s while the chain runs. Parties board the next buggies in
// line, never sharing with strangers: up to seats_max per buggy, using the fewest buggies and
// consecutive ones for a large party (contiguous_vehicles; decision 2026-10-17). A stop of any kind
// freezes the whole chain at once; there is no stacking, the queue absorbs it.
//
// Timing (decision 2026-10-17): spacing drives the loop. The chain takes count x spacing to go round;
// every route est_s is stretched by loop / sum(est_s) so scene timings fit it.
package continuous

import (
	"fmt"
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride"
	"github.com/jasrags/ridesim/internal/ride/lines"
)

// Stop causes recorded in ride_stop / ride_resume events.
const (
	CauseChainStop     = config.FailChainStop
	CauseLongDowntime  = config.FailLongDowntime
	CauseAccessibility = "accessibility_hold"
	CauseWeatherHold   = "weather_hold"
)

// LoadZone is the block name used in board and dispatch events.
const LoadZone = "load_zone"

const throughputWindow = 15 * engine.Minute

// Options are run-level settings that are not part of the ride file.
type Options struct {
	FixedTimes           bool
	Failures             []string // config.FailChainStop, config.FailLongDowntime
	NoAccessibilityHolds bool     // accessibility holds are on unless disabled
}

// Events for one chain out of several (logged so a ride_stop still means the whole ride).
const (
	EvSideDown = "side_down" // Block = chainN
	EvSideUp   = "side_up"
)

// chain is one independent loop with its own load point and conveyor clock.
type chain struct {
	idx       int
	belt      *conveyor
	slot      int64 // vehicles that have reached this chain's load zone
	downDepth int   // side_down outages in progress

	// A party larger than one buggy keeps boarding into the next buggies until seated.
	boarding  *guest.Party
	remaining int
}

// Ride is one continuous_load ride. It implements ride.Ride and lines.Ride.
type Ride struct {
	sim    *engine.Sim
	cfg    *config.ContinuousFile
	opts   Options
	lines  *lines.Lines
	chains []*chain // independent loops (conveyor.chains); Haunted Mansion has one

	spacing  engine.Duration
	loop     engine.Duration // count x spacing
	rideTime engine.Duration // board to unload, stretched route
	scale    float64         // loop / sum(route.est_s)

	stopCount int
	stopDepth map[string]int
}

var _ ride.Ride = (*Ride)(nil)

// New builds the model. The config must have passed validation.
func New(sim *engine.Sim, cfg *config.ContinuousFile, opts Options) (*Ride, error) {
	v := cfg.Vehicles
	if v.Count < 1 || v.SpacingS <= 0 || v.SeatsMax < 1 {
		return nil, fmt.Errorf("%s: vehicles need count, spacing_s and seats_max", cfg.Ride.ID)
	}
	r := &Ride{sim: sim, cfg: cfg, opts: opts, stopDepth: map[string]int{}}
	for i := range cfg.Conveyor.ChainCount() {
		r.chains = append(r.chains, &chain{idx: i, belt: newConveyor(sim)})
	}
	r.spacing = engine.Seconds(v.SpacingS)
	r.loop = engine.Duration(v.Count) * r.spacing
	total, onRide := 0.0, 0.0
	for _, s := range cfg.Route {
		total += s.EstS
		if s.Kind != config.RouteReturn {
			onRide += s.EstS
		}
	}
	if total <= 0 {
		return nil, fmt.Errorf("%s: route has no timings", cfg.Ride.ID)
	}
	r.scale = r.loop.Seconds() / total
	r.rideTime = engine.Seconds(onRide * r.scale)
	r.lines = lines.New(sim, r, &cfg.Queues, cfg.Preshow, cfg.Stages)
	return r, nil
}

// ID implements ride.Ride.
func (r *Ride) ID() string { return r.cfg.Ride.ID }

// Status implements ride.Ride.
func (r *Ride) Status() ride.Status {
	if r.Stopped() {
		return ride.Down
	}
	return ride.Open
}

// Arrive implements ride.Ride.
func (r *Ride) Arrive(p *guest.Party, lane guest.Lane) { r.lines.Arrive(p, lane) }

// RideTime is board-to-unload time on the stretched route.
func (r *Ride) RideTime() engine.Duration { return r.rideTime }

// RouteScale is loop / sum(route.est_s).
func (r *Ride) RouteScale() float64 { return r.scale }

// WaitingGuests is every guest who joined and has not boarded or reneged.
func (r *Ride) WaitingGuests() int { return r.lines.Waiting() }

// Sample implements lines.Ride (preshow timings).
func (r *Ride) Sample(d config.Dist) engine.Duration { return r.sample(d, engine.StreamStationTimes) }

// Board implements lines.Ride. Boarding happens buggy by buggy as each reaches the load zone, so a line
// change needs no immediate action.
func (r *Ride) Board() {}

// PostedWait implements ride.Ride: standby-side guests over standby riders per second in the last 15
// minutes, falling back to 1/spacing x occupancy_target riders per second.
func (r *Ride) PostedWait() engine.Duration {
	riders := r.lines.StandbyBoardedSince(r.sim.Now().Add(-throughputWindow))
	perS := float64(riders) / throughputWindow.Seconds()
	if riders == 0 || r.sim.Now() < engine.Time(throughputWindow) {
		perS = float64(r.chainsUp()) * r.cfg.Vehicles.OccupancyTarget.Mean / r.cfg.Vehicles.SpacingS
	}
	return engine.Seconds(float64(r.lines.StandbyWaiting()) / perS)
}

// Start begins operating: the chain runs, failures and accessibility holds are scheduled, the preshow opens.
func (r *Ride) Start() {
	r.lines.Start()
	r.startStops()
	r.startPosting()
	// Chains start staggered so their load points take turns: one vehicle somewhere every spacing / chains.
	for _, ch := range r.chains {
		// On the chain's own clock, so a stop or outage before a chain's first vehicle delays it too.
		ch.belt.after(engine.Duration(ch.idx)*r.spacing/engine.Duration(len(r.chains)), func() { r.nextBuggy(ch) })
	}
}

// postingRefresh matches the park's posted_wait_refresh_s.
const postingRefresh = 300 * engine.Second

// startPosting logs the posted wait now and every refresh, so metrics can compare it with true waits.
func (r *Ride) startPosting() {
	var post func()
	post = func() {
		r.sim.Emit(engine.Event{Type: engine.EvPostedWait, N: int(r.PostedWait() / engine.Second), Info: config.PostedQueueLenOverThroughput})
		r.sim.After(postingRefresh, post)
	}
	post()
}

// nextBuggy loads the buggy now at ch's load zone and schedules the next one a spacing later. Buggy ids
// run 1..count on the first chain, count+1..2 x count on the second, and so on.
func (r *Ride) nextBuggy(ch *chain) {
	ch.slot++
	r.loadBuggy(ch, ch.idx*r.cfg.Vehicles.Count+int(ch.slot%int64(r.cfg.Vehicles.Count))+1)
	if ch.slot%1024 == 0 {
		ch.belt.compact()
	}
	ch.belt.after(r.spacing, func() { r.nextBuggy(ch) })
}

// loadBuggy seats guests in buggy id on chain ch: the rest of a large party, the parties that share it
// (party_vehicle_policy shared), or the party at the front of the line. A boarding miss (board_fail_rate)
// sends the buggy on empty and the party takes the next one.
func (r *Ride) loadBuggy(ch *chain, id int) {
	seats := r.cfg.Vehicles.SeatsMax
	if ch.boarding == nil {
		p, line := r.lines.Head()
		if p == nil {
			if r.shared() {
				r.loadShared(ch, id) // single riders may still be waiting
				return
			}
			r.dispatch(ch, id, 0, nil)
			return
		}
		if f := r.cfg.LoadZone.BoardFailRate; f > 0 && r.sim.Stream(engine.StreamStationTimes).Float64() < f {
			r.dispatch(ch, id, 0, nil)
			return
		}
		if r.shared() && p.Size <= seats {
			r.loadShared(ch, id)
			return
		}
		r.lines.Pop(line)
		r.lines.Seated(p)
		r.sim.Emit(engine.Event{Type: engine.EvBoard, Train: id, Block: LoadZone, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
		ch.boarding, ch.remaining = p, p.Size
		r.lines.Boarded()
	}
	// Spread the party evenly over the fewest buggies: a party of 4 rides 2 + 2, a party of 5 rides 3 + 2.
	buggiesLeft := (ch.remaining + seats - 1) / seats
	n := (ch.remaining + buggiesLeft - 1) / buggiesLeft
	ch.remaining -= n
	var last []*guest.Party // the party is off the ride when its last buggy unloads
	if ch.remaining == 0 {
		last, ch.boarding = []*guest.Party{ch.boarding}, nil
	}
	r.dispatch(ch, id, n, last)
}

func (r *Ride) shared() bool { return r.cfg.LoadingShared.PartyVehiclePolicy == config.SharedVehicles }

// loadShared fills one vehicle ([user] Smugglers Run): parties from the line take the first party_seats
// seats (a party larger than that boards only an empty vehicle), single riders fill the rest, then
// parties fill what is left. When the next party does not fit, cast call one that does from up to
// callout_parties back. A party larger than a vehicle waits at the head for an empty one.
func (r *Ride) loadShared(ch *chain, id int) {
	seats, ls := r.cfg.Vehicles.SeatsMax, r.cfg.LoadingShared
	front := ls.PartySeats
	if front == 0 {
		front = seats
	}
	n := 0
	var aboard []*guest.Party
	seat := func(p *guest.Party) {
		r.lines.Seated(p)
		r.sim.Emit(engine.Event{Type: engine.EvBoard, Train: id, Block: LoadZone, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
		aboard = append(aboard, p)
		n += p.Size
	}
	parties := func(limit int) {
		for n < limit {
			p, line := r.lines.HeadParty()
			if p == nil {
				return
			}
			if n+p.Size <= limit || (n == 0 && p.Size <= seats) {
				r.lines.Pop(line)
				seat(p)
				continue
			}
			if ls.CalloutParties == 0 {
				return
			}
			c := r.lines.CallOut(limit-n, ls.CalloutParties)
			if c == nil {
				return
			}
			seat(c)
		}
	}
	parties(front)
	for n < seats {
		p := r.lines.PeekSingle()
		if p == nil || n+p.Size > seats {
			break
		}
		r.lines.PopSingle()
		seat(p)
	}
	parties(seats)
	if n > 0 {
		r.lines.Boarded()
	}
	r.dispatch(ch, id, n, aboard)
}

// dispatch logs the buggy leaving the load zone with n riders and schedules its unload on its chain's
// clock; the parties in done are told they rode then.
func (r *Ride) dispatch(ch *chain, id, n int, done []*guest.Party) {
	r.sim.Emit(engine.Event{Type: engine.EvDispatch, Train: id, Block: LoadZone, N: n})
	if n > 0 {
		ch.belt.after(r.rideTime, func() {
			r.sim.Emit(engine.Event{Type: engine.EvUnload, Train: id, Block: "unload_zone", N: n})
			for _, p := range done {
				p.Exited(guest.Rode)
			}
		})
	}
}

// ---------------------------------------------------------------------------------------------------
// Stops

// Stopped reports whether the chain is halted.
func (r *Ride) Stopped() bool { return r.stopCount > 0 }

// Closed reports a long downtime in progress: the sign says closed (decision 2026-10-17).
func (r *Ride) Closed() bool {
	return r.stopDepth[CauseLongDowntime] > 0 || r.stopDepth[config.CauseOpsClosure] > 0 || r.stopDepth[config.CauseAreaHours] > 0
}

// Stop halts the whole chain for cause. Causes nest.
func (r *Ride) Stop(cause string) {
	r.sim.Emit(engine.Event{Type: engine.EvRideStop, Info: cause})
	r.stopDepth[cause]++
	r.stopCount++
	for _, ch := range r.chains {
		ch.belt.stop()
	}
}

// Resume ends one instance of cause; the chain moves again when no cause remains.
func (r *Ride) Resume(cause string) {
	if r.stopDepth[cause] == 0 {
		panic("continuous: resume without stop: " + cause)
	}
	r.sim.Emit(engine.Event{Type: engine.EvRideResume, Info: cause})
	r.stopDepth[cause]--
	r.stopCount--
	if r.stopCount == 0 {
		r.startChains()
	}
}

// startChains restarts every chain that is not down on its own.
func (r *Ride) startChains() {
	for _, ch := range r.chains {
		if ch.downDepth == 0 {
			ch.belt.start()
		}
	}
}

func (r *Ride) chainsUp() int {
	n := 0
	for _, ch := range r.chains {
		if ch.downDepth == 0 {
			n++
		}
	}
	return max(1, n)
}

// ChainDownFor takes chain i out for d (one turntable of several): its vehicles freeze where they are,
// riders aboard included, and its load point stops; the other chains keep running.
func (r *Ride) ChainDownFor(i int, d engine.Duration) {
	ch := r.chains[i]
	r.sim.Emit(engine.Event{Type: EvSideDown, Block: fmt.Sprintf("chain%d", i+1), D: d})
	ch.downDepth++
	ch.belt.stop()
	r.sim.After(d, func() {
		r.sim.Emit(engine.Event{Type: EvSideUp, Block: fmt.Sprintf("chain%d", i+1)})
		ch.downDepth--
		if ch.downDepth == 0 && r.stopCount == 0 {
			ch.belt.start()
		}
	})
}

// StopFor stops the chain for d, then resumes.
func (r *Ride) StopFor(cause string, d engine.Duration) {
	r.Stop(cause)
	r.sim.After(d, func() { r.Resume(cause) })
}

func (r *Ride) startStops() {
	f := r.cfg.Failures
	for _, kind := range r.opts.Failures {
		switch kind {
		case config.FailChainStop:
			if f.ChainStop != nil && f.ChainStop.RatePerHr > 0 {
				r.poisson(CauseChainStop, 3600/f.ChainStop.RatePerHr, engine.StreamFailures, func() engine.Duration {
					return r.sample(f.ChainStop.DurationS, engine.StreamFailures)
				})
			}
		case config.FailSideDown:
			if f.SideDown != nil && f.SideDown.RatePerHr > 0 && len(r.chains) > 1 {
				r.chainDowns(3600 / f.SideDown.RatePerHr)
			}
		case config.FailLongDowntime:
			if f.LongDowntime != nil && f.LongDowntime.MTBFHr > 0 {
				r.poisson(CauseLongDowntime, f.LongDowntime.MTBFHr*3600, engine.StreamFailures, func() engine.Duration {
					m := f.LongDowntime.MTTRMin
					return r.sample(config.Dist{Mean: m.Mean * 60, SD: m.SD * 60}, engine.StreamFailures)
				})
			}
		}
	}
	if h := r.cfg.LoadingShared.AccessibilityHold; !r.opts.NoAccessibilityHolds && h.RatePerHr > 0 {
		// Wheelchair guests boarding or leaving freeze the carpet and buggies ([wiki]); an ops process, not a failure.
		r.poisson(CauseAccessibility, 3600/h.RatePerHr, engine.StreamOps, func() engine.Duration {
			return r.sample(h.DurationS, engine.StreamOps)
		})
	}
}

// chainDowns takes a random chain out at exponential gaps of mean meanGapS seconds.
func (r *Ride) chainDowns(meanGapS float64) {
	gap := r.sim.Stream(engine.StreamFailures).ExpFloat64() * meanGapS
	r.sim.After(engine.Duration(math.Max(1, math.Round(gap*1000))), func() {
		i := r.sim.Stream(engine.StreamFailures).IntN(len(r.chains))
		r.ChainDownFor(i, r.sample(r.cfg.Failures.SideDown.DurationS, engine.StreamFailures))
		r.chainDowns(meanGapS)
	})
}

func (r *Ride) poisson(cause string, meanGapS float64, stream string, duration func() engine.Duration) {
	gap := r.sim.Stream(stream).ExpFloat64() * meanGapS
	r.sim.After(engine.Duration(math.Max(1, math.Round(gap*1000))), func() {
		r.StopFor(cause, duration())
		r.poisson(cause, meanGapS, stream, duration)
	})
}

// sample draws from a normal truncated at zero; FixedTimes uses the mean and consumes no randomness.
func (r *Ride) sample(d config.Dist, stream string) engine.Duration {
	if r.opts.FixedTimes || d.SD == 0 {
		return engine.Seconds(d.Mean)
	}
	return engine.Seconds(max(0, d.Mean+d.SD*r.sim.Stream(stream).NormFloat64()))
}

// SingleRiderOpen reports whether the single-rider line takes new guests.
func (r *Ride) SingleRiderOpen() bool { return r.lines.SingleRiderOpen() }

// SetSingleRiderOpen opens or closes the single-rider line (an operations lever).
func (r *Ride) SetSingleRiderOpen(open bool) { r.lines.SetSingleRiderOpen(open) }

// SetMergeRatio implements the park's ll_merge lever.
func (r *Ride) SetMergeRatio(llPerStandby int) bool { return r.lines.SetMergeRatio(llPerStandby) }

// HasMerge reports whether the ll_merge lever applies.
func (r *Ride) HasMerge() bool { return r.lines.HasMerge() }
