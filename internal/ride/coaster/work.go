package coaster

import (
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// work is a train's single pending timer. It can pause while the ride is stopped and resume with the
// time it had left. gen guards against a stale scheduled callback after the work was replaced.
type work struct {
	fn        func()
	end       engine.Time
	remaining engine.Duration
	h         engine.Handle
	gen       uint64
	active    bool
	paused    bool
}

// pausesAt reports whether a train in b stops while the ride is stopped. Stations, brakes and launch
// tracks (the train is staged, not yet launched) hold trains; lifts and course blocks run the train on
// to the next brake or station.
func pausesAt(b *block) bool {
	return b.role != roleTrack || b.cfg.Kind == config.BlockBrake || b.cfg.Kind == config.BlockLaunch
}

// startWork replaces t's pending work with fn after d. If the ride is stopped and t is somewhere that
// holds trains, the work starts paused.
func (c *Coaster) startWork(t *train, d engine.Duration, fn func()) {
	t.work.h.Cancel()
	t.work = work{fn: fn, active: true, gen: t.work.gen + 1}
	if c.Stopped() && pausesAt(t.at) {
		t.work.paused, t.work.remaining = true, d
		return
	}
	c.schedule(t, d)
}

func (c *Coaster) schedule(t *train, d engine.Duration) {
	gen := t.work.gen
	t.work.end = c.sim.Now().Add(d)
	t.work.h = c.sim.After(d, func() {
		if t.work.gen != gen || !t.work.active || t.work.paused {
			return
		}
		t.work.active = false
		t.work.fn()
	})
}

func (c *Coaster) cancelWork(t *train) {
	t.work.h.Cancel()
	t.work.active = false
}

func (c *Coaster) pauseWork(t *train) {
	if !t.work.active || t.work.paused {
		return
	}
	t.work.h.Cancel()
	t.work.paused, t.work.remaining = true, t.work.end.Sub(c.sim.Now())
}

func (c *Coaster) resumeWork(t *train) {
	if !t.work.active || !t.work.paused {
		return
	}
	t.work.paused = false
	c.schedule(t, t.work.remaining)
}
