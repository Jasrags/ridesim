package elevator

import (
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func engineBoard(c *cab, p *guest.Party) engine.Event {
	return engine.Event{Type: engine.EvBoard, Train: c.id, Block: LoadBlock, Guest: p.ID, N: p.Size, Info: p.Lane.String()}
}

// work is a cab's single pending timer. It pauses while the ride or the cab's side is stopped and
// resumes with the time it had left; gen guards against a stale callback after the work was replaced.
type work struct {
	fn        func()
	end       engine.Time
	remaining engine.Duration
	h         engine.Handle
	gen       uint64
	active    bool
	paused    bool
}

// paused reports whether c is frozen: the whole ride is stopped or its side is down.
func (r *Ride) paused(c *cab) bool { return r.stopCount > 0 || c.side.downDepth > 0 }

func (r *Ride) startWork(c *cab, d engine.Duration, fn func()) {
	c.work.h.Cancel()
	c.work = work{fn: fn, active: true, gen: c.work.gen + 1}
	if r.paused(c) {
		c.work.paused, c.work.remaining = true, d
		return
	}
	r.schedule(c, d)
}

func (r *Ride) schedule(c *cab, d engine.Duration) {
	gen := c.work.gen
	c.work.end = r.sim.Now().Add(d)
	c.work.h = r.sim.After(d, func() {
		if c.work.gen != gen || !c.work.active || c.work.paused {
			return
		}
		c.work.active = false
		c.work.fn()
	})
}

func (r *Ride) cancelWork(c *cab) {
	c.work.h.Cancel()
	c.work.active = false
}

func (r *Ride) pauseWork(c *cab) {
	if !c.work.active || c.work.paused {
		return
	}
	c.work.h.Cancel()
	c.work.paused, c.work.remaining = true, c.work.end.Sub(r.sim.Now())
}

func (r *Ride) resumeWork(c *cab) {
	if !c.work.active || !c.work.paused || r.paused(c) {
		return
	}
	c.work.paused = false
	r.schedule(c, c.work.remaining)
}

// Stopped reports whether the whole ride is stopped.
func (r *Ride) Stopped() bool { return r.stopCount > 0 }

// Closed reports a long downtime or an ops closure in progress: the sign says closed.
func (r *Ride) Closed() bool {
	return r.stopDepth[CauseLongDowntime] > 0 || r.stopDepth[config.CauseOpsClosure] > 0 || r.stopDepth[config.CauseAreaHours] > 0
}

// Stop adds a ride-wide stop cause; every cab pauses where it is. Causes nest.
func (r *Ride) Stop(cause string) {
	r.sim.Emit(engine.Event{Type: engine.EvRideStop, Info: cause})
	r.stopDepth[cause]++
	r.stopCount++
	if r.stopCount == 1 {
		for _, c := range r.cabs {
			r.pauseWork(c)
		}
	}
}

// Resume ends one instance of cause; cabs move again when no cause remains (and their side is up).
func (r *Ride) Resume(cause string) {
	if r.stopDepth[cause] == 0 {
		panic("elevator: resume without stop: " + cause)
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

// SideDownFor takes side i out of service for d: its cabs pause, the other sides keep loading.
func (r *Ride) SideDownFor(i int, d engine.Duration) {
	sd := r.sides[i]
	r.sim.Emit(engine.Event{Type: EvSideDown, Block: sd.name, D: d})
	sd.downDepth++
	if sd.downDepth == 1 {
		for _, c := range r.cabs {
			if c.side == sd {
				r.pauseWork(c)
			}
		}
	}
	r.sim.After(d, func() {
		r.sim.Emit(engine.Event{Type: EvSideUp, Block: sd.name})
		sd.downDepth--
		if sd.downDepth == 0 {
			r.resumeAll()
		}
	})
}

// SidesUp is the number of sides in service (ignoring ride-wide stops).
func (r *Ride) SidesUp() int {
	n := 0
	for _, sd := range r.sides {
		if sd.downDepth == 0 {
			n++
		}
	}
	return n
}

// resumeAll restarts every cab that may move, then seats guests in cabs that were frozen at load.
func (r *Ride) resumeAll() {
	for _, c := range r.cabs {
		r.resumeWork(c)
	}
	r.Board()
}

// startStops schedules the enabled random failures. Occurrences and durations draw from the failures
// stream; FixedTimes fixes the durations at their means but not the occurrences.
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
			if f.SideDown != nil && f.SideDown.RatePerHr > 0 && len(r.sides) > 1 {
				r.poisson(3600/f.SideDown.RatePerHr, func() {
					i := r.sim.Stream(engine.StreamFailures).IntN(len(r.sides))
					r.SideDownFor(i, r.sample(f.SideDown.DurationS, engine.StreamFailures))
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
