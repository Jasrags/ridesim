// Package bays implements the simulator_bays archetype (M13): identical simulator bays that each run
// their own cycle (Jason, 2026-10-18), such as Star Tours. A bay:
//
//  1. lets the previous riders out (exit_s); the rows clear and the next group may board;
//  2. boards for load_s (package seats: rows or boarding groups with callouts), then waits up to
//     max_wait_after_ready_s for more guests unless full or nobody in line fits;
//  3. checks restraints, then runs the safety video and the flight, and starts over.
//
// Bays board from one shared load line, the bay that started boarding first served first. A ride stop
// freezes every bay with its remaining time; side_down freezes one bay.
package bays

import (
	"fmt"
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride"
	"github.com/jasrags/ridesim/internal/ride/lines"
	"github.com/jasrags/ridesim/internal/ride/seats"
)

// Stop causes recorded in ride_stop / ride_resume events.
const (
	CauseBriefStop    = config.FailBriefStop
	CauseLongDowntime = config.FailLongDowntime
	CauseWeatherHold  = "weather_hold"
)

// Events for one bay out (a ride_stop would read as the whole ride stopping).
const (
	EvSideDown = "side_down" // Block = bay name
	EvSideUp   = "side_up"
)

// LoadBlock is the block name used in board and dispatch events; Info carries the bay.
const LoadBlock = "LOAD"

const (
	throughputWindow = 15 * engine.Minute
	postingRefresh   = 300 * engine.Second // matches the park's posted_wait_refresh_s
)

// Options are run-level settings that are not part of the ride file.
type Options struct {
	FixedTimes bool
	Failures   []string // config.FailBriefStop, config.FailLongDowntime, config.FailSideDown
}

type bay struct {
	id        int
	name      string
	seat      seats.Load
	work      work
	downDepth int
}

// Ride is one simulator_bays ride. It implements ride.Ride and lines.Ride.
type Ride struct {
	sim   *engine.Sim
	cfg   *config.BaysFile
	opts  Options
	lines *lines.Lines
	rules seats.Rules

	bays    []*bay
	loading []*bay // bays boarding, in the order they started

	stopCount int
	stopDepth map[string]int
}

var _ ride.Ride = (*Ride)(nil)

// New builds the model. The config must have passed validation.
func New(sim *engine.Sim, cfg *config.BaysFile, opts Options) (*Ride, error) {
	b := cfg.Bays
	if b.Count < 1 || b.SeatsPerBay < 1 {
		return nil, fmt.Errorf("%s: simulator_bays needs bays and seats", cfg.Ride.ID)
	}
	r := &Ride{sim: sim, cfg: cfg, opts: opts, stopDepth: map[string]int{},
		rules: seats.Rules{Seats: b.SeatsPerBay, SeatsPerRow: b.SeatsPerRow, Groups: b.BoardingGroups, CalloutParties: cfg.Cycle.CalloutParties}}
	for i := range b.Count {
		r.bays = append(r.bays, &bay{id: i + 1, name: fmt.Sprintf("BAY_%d", i+1)})
	}
	r.lines = lines.New(sim, r, &cfg.Queues, cfg.Preshow, nil)
	return r, nil
}

// ID implements ride.Ride.
func (r *Ride) ID() string { return r.cfg.Ride.ID }

// Status implements ride.Ride.
func (r *Ride) Status() ride.Status {
	if r.stopCount > 0 {
		return ride.Down
	}
	return ride.Open
}

// Arrive implements ride.Ride.
func (r *Ride) Arrive(p *guest.Party, lane guest.Lane) { r.lines.Arrive(p, lane) }

// WaitingGuests is every guest who joined and has not boarded or reneged.
func (r *Ride) WaitingGuests() int { return r.lines.Waiting() }

// Sample implements lines.Ride (preshow timings).
func (r *Ride) Sample(d config.Dist) engine.Duration { return r.sample(d, engine.StreamStationTimes) }

// PostedWait implements ride.Ride: standby-side guests over standby riders per second in the last 15
// minutes, falling back to the theoretical rate of the bays in service.
func (r *Ride) PostedWait() engine.Duration {
	riders := r.lines.StandbyBoardedSince(r.sim.Now().Add(-throughputWindow))
	perS := float64(riders) / throughputWindow.Seconds()
	if riders == 0 || r.sim.Now() < engine.Time(throughputWindow) {
		perS = float64(r.baysUp()*r.cfg.Bays.SeatsPerBay) / r.cfg.CycleS()
	}
	return engine.Seconds(float64(r.lines.StandbyWaiting()) / perS)
}

// Start opens the preshow (if any), schedules failures and opens every bay for boarding.
func (r *Ride) Start() {
	r.lines.Start()
	r.startStops()
	var post func()
	post = func() {
		r.sim.Emit(engine.Event{Type: engine.EvPostedWait, N: int(r.PostedWait() / engine.Second), Info: config.PostedQueueLenOverThroughput})
		r.sim.After(postingRefresh, post)
	}
	post()
	for _, b := range r.bays {
		r.startBoarding(b)
	}
}

// ---------------------------------------------------------------------------------------------------
// The bay cycle

func (r *Ride) startBoarding(b *bay) {
	r.sim.Emit(engine.Event{Type: engine.EvTrainEnter, Train: b.id, Block: LoadBlock, Info: b.name})
	b.seat = r.rules.New()
	r.loading = append(r.loading, b)
	r.boardBay(b)
	r.startWork(b, r.sample(r.cfg.Cycle.LoadS, engine.StreamStationTimes), func() { r.loadDone(b) })
}

func (r *Ride) loadDone(b *bay) {
	b.seat.FillWait = true
	r.sim.Emit(engine.Event{Type: engine.EvLoadDone, Train: b.id, Block: LoadBlock, N: b.seat.Riders})
	r.boardBay(b)
	if !r.atLoad(b) { // boarding re-entered Board, which already started the check
		return
	}
	if r.rules.FillWaitOver(&b.seat, r.lines) {
		r.startCheck(b)
		return
	}
	r.startWork(b, engine.Seconds(r.cfg.Cycle.MaxWaitAfterReadyS), func() { r.startCheck(b) })
}

// startCheck ends boarding. Boarding can re-enter Board (lines.Boarded drains a preshow, which calls
// Board), so a bay whose check has already started is left alone.
func (r *Ride) startCheck(b *bay) {
	if !r.leaveLoading(b) {
		return
	}
	r.startWork(b, r.sample(r.cfg.Cycle.RestraintCheckS, engine.StreamStationTimes), func() { r.launch(b) })
}

func (r *Ride) launch(b *bay) {
	r.sim.Emit(engine.Event{Type: engine.EvDispatch, Train: b.id, Block: LoadBlock, N: b.seat.Riders, Info: b.name})
	y := r.cfg.Cycle
	show := r.sample(y.SafetyVideoS, engine.StreamTrackTimes) + r.sample(y.FlightS, engine.StreamTrackTimes)
	r.startWork(b, show, func() {
		r.startWork(b, r.sample(y.ExitS, engine.StreamStationTimes), func() {
			s := b.seat
			r.sim.Emit(engine.Event{Type: engine.EvUnload, Train: b.id, Block: b.name, N: s.Riders})
			for _, p := range s.Parties {
				p.Exited(guest.Rode)
			}
			r.startBoarding(b)
		})
	})
}

// Board implements lines.Ride: the load line changed, so seat whoever fits, oldest boarding bay first.
func (r *Ride) Board() {
	for _, b := range append([]*bay(nil), r.loading...) {
		if r.paused(b) || !r.atLoad(b) { // a nested Board may have sent it on already
			continue
		}
		r.boardBay(b)
		if !r.atLoad(b) {
			continue
		}
		if b.seat.FillWait && r.rules.FillWaitOver(&b.seat, r.lines) {
			r.cancelWork(b)
			r.startCheck(b)
		}
	}
}

func (r *Ride) boardBay(b *bay) {
	if r.paused(b) {
		return
	}
	r.rules.Board(&b.seat, r.lines, func(p *guest.Party) {
		r.sim.Emit(engine.Event{Type: engine.EvBoard, Train: b.id, Block: LoadBlock, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
	})
}

func (r *Ride) atLoad(b *bay) bool {
	for _, x := range r.loading {
		if x == b {
			return true
		}
	}
	return false
}

func (r *Ride) leaveLoading(b *bay) bool {
	for i, x := range r.loading {
		if x == b {
			r.loading = append(r.loading[:i], r.loading[i+1:]...)
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------------------------------
// Timers and stops

// work is a bay's single pending timer; it pauses while the ride or the bay is stopped.
type work struct {
	fn        func()
	end       engine.Time
	remaining engine.Duration
	h         engine.Handle
	gen       uint64
	active    bool
	paused    bool
}

func (r *Ride) paused(b *bay) bool { return r.stopCount > 0 || b.downDepth > 0 }

func (r *Ride) startWork(b *bay, d engine.Duration, fn func()) {
	b.work.h.Cancel()
	b.work = work{fn: fn, active: true, gen: b.work.gen + 1}
	if r.paused(b) {
		b.work.paused, b.work.remaining = true, d
		return
	}
	r.schedule(b, d)
}

func (r *Ride) schedule(b *bay, d engine.Duration) {
	gen := b.work.gen
	b.work.end = r.sim.Now().Add(d)
	b.work.h = r.sim.After(d, func() {
		if b.work.gen != gen || !b.work.active || b.work.paused {
			return
		}
		b.work.active = false
		b.work.fn()
	})
}

func (r *Ride) cancelWork(b *bay) {
	b.work.h.Cancel()
	b.work.active = false
}

func (r *Ride) pauseWork(b *bay) {
	if !b.work.active || b.work.paused {
		return
	}
	b.work.h.Cancel()
	b.work.paused, b.work.remaining = true, b.work.end.Sub(r.sim.Now())
}

func (r *Ride) resumeAll() {
	for _, b := range r.bays {
		if b.work.active && b.work.paused && !r.paused(b) {
			b.work.paused = false
			r.schedule(b, b.work.remaining)
		}
	}
	r.Board()
}

func (r *Ride) baysUp() int {
	n := 0
	for _, b := range r.bays {
		if b.downDepth == 0 {
			n++
		}
	}
	return max(1, n)
}

// Stopped reports whether the whole ride is stopped.
func (r *Ride) Stopped() bool { return r.stopCount > 0 }

// Closed reports a long downtime or an ops closure in progress: the sign says closed.
func (r *Ride) Closed() bool {
	return r.stopDepth[CauseLongDowntime] > 0 || r.stopDepth[config.CauseOpsClosure] > 0
}

// Stop adds a ride-wide stop cause; every bay pauses where it is. Causes nest.
func (r *Ride) Stop(cause string) {
	r.sim.Emit(engine.Event{Type: engine.EvRideStop, Info: cause})
	r.stopDepth[cause]++
	r.stopCount++
	if r.stopCount == 1 {
		for _, b := range r.bays {
			r.pauseWork(b)
		}
	}
}

// Resume ends one instance of cause; bays run again when no cause remains (and they are not down).
func (r *Ride) Resume(cause string) {
	if r.stopDepth[cause] == 0 {
		panic("bays: resume without stop: " + cause)
	}
	r.sim.Emit(engine.Event{Type: engine.EvRideResume, Info: cause})
	r.stopDepth[cause]--
	r.stopCount--
	if r.stopCount == 0 {
		r.resumeAll()
	}
}

// StopFor stops the ride for d, then resumes.
func (r *Ride) StopFor(cause string, d engine.Duration) {
	r.Stop(cause)
	r.sim.After(d, func() { r.Resume(cause) })
}

// BayDownFor takes bay i out for d: it pauses where it is (riders aboard included); the others run.
func (r *Ride) BayDownFor(i int, d engine.Duration) {
	b := r.bays[i]
	r.sim.Emit(engine.Event{Type: EvSideDown, Block: b.name, D: d})
	b.downDepth++
	r.pauseWork(b)
	r.sim.After(d, func() {
		r.sim.Emit(engine.Event{Type: EvSideUp, Block: b.name})
		b.downDepth--
		if b.downDepth == 0 {
			r.resumeAll()
		}
	})
}

func (r *Ride) startStops() {
	f := r.cfg.Failures
	for _, kind := range r.opts.Failures {
		switch kind {
		case config.FailBriefStop:
			if f.BriefStop != nil && f.BriefStop.RatePerHr > 0 {
				r.poisson(3600/f.BriefStop.RatePerHr, func() {
					r.StopFor(CauseBriefStop, r.sample(f.BriefStop.DurationS, engine.StreamFailures))
				})
			}
		case config.FailLongDowntime:
			if f.LongDowntime != nil && f.LongDowntime.MTBFHr > 0 {
				r.poisson(f.LongDowntime.MTBFHr*3600, func() {
					m := f.LongDowntime.MTTRMin
					r.StopFor(CauseLongDowntime, r.sample(config.Dist{Mean: m.Mean * 60, SD: m.SD * 60}, engine.StreamFailures))
				})
			}
		case config.FailSideDown:
			if f.SideDown != nil && f.SideDown.RatePerHr > 0 && len(r.bays) > 1 {
				r.poisson(3600/f.SideDown.RatePerHr, func() {
					i := r.sim.Stream(engine.StreamFailures).IntN(len(r.bays))
					r.BayDownFor(i, r.sample(f.SideDown.DurationS, engine.StreamFailures))
				})
			}
		}
	}
}

func (r *Ride) poisson(meanGapS float64, fire func()) {
	gap := r.sim.Stream(engine.StreamFailures).ExpFloat64() * meanGapS
	r.sim.After(engine.Duration(math.Max(1, math.Round(gap*1000))), func() {
		fire()
		r.poisson(meanGapS, fire)
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
