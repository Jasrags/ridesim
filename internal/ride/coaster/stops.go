package coaster

import (
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// Stop causes, recorded in ride_stop / ride_resume events.
const (
	CauseBriefStop    = config.FailBriefStop
	CauseLongDowntime = config.FailLongDowntime
	CauseCascadeStop  = config.FailCascadeStop
	CauseWeatherHold  = "weather_hold"
	CauseBarnSwap     = "barn_swap"
	CauseOpeningDelay = config.FailOpeningDelay
)

// Stopped reports whether any stop cause is active.
func (c *Coaster) Stopped() bool { return c.stopCount > 0 }

// Closed reports a long downtime in progress: the sign says closed (decision 2026-10-17).
func (c *Coaster) Closed() bool {
	return c.stopDepth[CauseLongDowntime] > 0 || c.stopDepth[config.CauseOpsClosure] > 0 || c.stopDepth[config.CauseAreaHours] > 0 || c.stopDepth[CauseOpeningDelay] > 0
}

// Stop adds an active stop cause. Causes nest: the ride runs again only when every cause has resumed.
// On the first cause, trains at stations and brakes pause where they are; trains on lifts and course
// blocks run on to the next brake or station (decision 2026-10-17).
func (c *Coaster) Stop(cause string) {
	c.sim.Emit(engine.Event{Type: engine.EvRideStop, Info: cause})
	c.stopDepth[cause]++
	c.stopCount++
	if c.stopCount > 1 {
		return
	}
	for _, t := range c.trains {
		if pausesAt(t.at) {
			c.pauseWork(t)
		}
	}
	c.kick() // trains waiting to leave a station or brake now log ride_stopped
}

// Resume ends one instance of cause. It panics on an unmatched resume, which is a model bug.
func (c *Coaster) Resume(cause string) {
	if c.stopDepth[cause] == 0 {
		panic("coaster: resume without stop: " + cause)
	}
	c.sim.Emit(engine.Event{Type: engine.EvRideResume, Info: cause})
	c.stopDepth[cause]--
	c.stopCount--
	if c.stopCount > 0 {
		return
	}
	for _, t := range c.trains {
		c.resumeWork(t)
	}
	c.flow()
	c.kick()
}

// StopFor stops the ride for d, then resumes.
func (c *Coaster) StopFor(cause string, d engine.Duration) {
	c.Stop(cause)
	c.sim.After(d, func() { c.Resume(cause) })
}

// startFailures schedules the enabled random failure processes. Occurrences and durations both draw
// from the failures stream; FixedTimes fixes the durations at their means but not the occurrences.
func (c *Coaster) startFailures() {
	f := c.cfg.Failures
	for _, kind := range c.opts.Failures {
		switch kind {
		case config.FailBriefStop:
			if f.BriefStop != nil && f.BriefStop.RatePerHr > 0 {
				c.poisson(CauseBriefStop, 3600/f.BriefStop.RatePerHr, func() engine.Duration {
					return c.sample(f.BriefStop.DurationS, engine.StreamFailures)
				})
			}
		case config.FailLongDowntime:
			if f.LongDowntime != nil && f.LongDowntime.MTBFHr > 0 {
				c.poisson(CauseLongDowntime, f.LongDowntime.MTBFHr*3600, func() engine.Duration {
					m := f.LongDowntime.MTTRMin
					return c.sample(config.Dist{Mean: m.Mean * 60, SD: m.SD * 60}, engine.StreamFailures)
				})
			}
		case config.FailCascadeStop:
			c.cascadeEnabled = f.CascadeStop != nil
		case config.FailSideDown:
			if sd := f.SideDown; sd != nil && sd.RatePerHr > 0 && c.unloadBlock() != nil && c.unloadBlock().alt != nil {
				c.sideDowns(3600 / sd.RatePerHr)
			}
		case config.FailOpeningDelay:
			// Drawn once at opening, before the ride runs ([fan] Rise is down at opening ~25% of days).
			if d := f.OpeningDelay; d != nil && c.sim.Stream(engine.StreamFailures).Float64() < d.Probability {
				c.StopFor(CauseOpeningDelay, c.sample(d.DurationS, engine.StreamFailures))
			}
		}
	}
}

// EvSideDown is logged when one platform of a dual-load station goes out of service. Block = the platform,
// D = how long.
const EvSideDown = "side_down"

// streamSideDown keeps one-platform outages off the failures stream, so turning them on shifts no other failure.
const streamSideDown = engine.StreamFailures + "/side_down"

// sideDowns closes the second load platform at exponential gaps (mean meanGapS seconds) for sampled durations.
// A train already on it finishes loading and leaves.
func (c *Coaster) sideDowns(meanGapS float64) {
	alt := c.unloadBlock().alt
	gap := c.sim.Stream(streamSideDown).ExpFloat64() * meanGapS
	c.sim.After(engine.Duration(math.Max(1, math.Round(gap*1000))), func() {
		d := c.sample(c.cfg.Failures.SideDown.DurationS, streamSideDown)
		if !alt.closed {
			alt.closed = true
			c.sim.Emit(engine.Event{Type: EvSideDown, Block: alt.cfg.ID, D: d})
			c.sim.After(d, func() {
				alt.closed = false
				c.kick()
			})
		}
		c.sideDowns(meanGapS)
	})
}

// poisson schedules stops of cause with exponential gaps of mean meanGapS seconds.
func (c *Coaster) poisson(cause string, meanGapS float64, duration func() engine.Duration) {
	gap := c.sim.Stream(engine.StreamFailures).ExpFloat64() * meanGapS
	c.sim.After(engine.Duration(math.Max(1, math.Round(gap*1000))), func() {
		c.StopFor(cause, duration())
		c.poisson(cause, meanGapS, duration)
	})
}
