package continuous

import "github.com/jasrags/ridesim/internal/engine"

// conveyor is a clock that runs only while the chain moves. Every timer on it is in conveyor time, so
// a stop freezes the whole chain at once and resuming shifts everything by the stop length (spec §7:
// a stop halts everything; there is no stacking).
type conveyor struct {
	sim     *engine.Sim
	base    engine.Duration // conveyor time accumulated before the current run
	since   engine.Time     // sim time the current run started
	running bool
	timers  []*ctimer
}

type ctimer struct {
	due  engine.Duration // conveyor time
	fn   func()
	h    engine.Handle
	done bool
}

func newConveyor(sim *engine.Sim) *conveyor {
	return &conveyor{sim: sim, since: sim.Now(), running: true}
}

// now is the current conveyor time.
func (c *conveyor) now() engine.Duration {
	if !c.running {
		return c.base
	}
	return c.base + c.sim.Now().Sub(c.since)
}

// after runs fn when the conveyor has advanced d.
func (c *conveyor) after(d engine.Duration, fn func()) {
	t := &ctimer{due: c.now() + d, fn: fn}
	c.timers = append(c.timers, t)
	if c.running {
		c.arm(t)
	}
}

func (c *conveyor) arm(t *ctimer) {
	t.h = c.sim.After(t.due-c.now(), func() {
		t.done = true
		t.fn()
	})
}

func (c *conveyor) stop() {
	if !c.running {
		return
	}
	c.base, c.running = c.now(), false
	live := c.timers[:0]
	for _, t := range c.timers {
		if !t.done {
			t.h.Cancel()
			live = append(live, t)
		}
	}
	c.timers = live
}

func (c *conveyor) start() {
	if c.running {
		return
	}
	c.since, c.running = c.sim.Now(), true
	for _, t := range c.timers {
		if !t.done {
			c.arm(t)
		}
	}
}

// compact drops fired timers (called occasionally to bound memory).
func (c *conveyor) compact() {
	live := c.timers[:0]
	for _, t := range c.timers {
		if !t.done {
			live = append(live, t)
		}
	}
	c.timers = live
}
