package coaster

import (
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// Cascade scopes: which next_occupied holds count toward cascade_stop. Designed waits (hold_until_clear,
// switch_realign, min interval, clearance) never count.
const (
	CascadeBrakesOnly        = config.CascadeScopeBrakesOnly // default (decision 2026-10-17)
	CascadeBrakesAndStations = config.CascadeScopeBrakesAndStations
)

// countsForCascade reports whether a hold of reason at b counts toward cascade_stop under scope
// ("" means the default, brakes only).
func countsForCascade(b *block, reason, scope string) bool {
	if reason != ReasonNextOccupied || !pausesAt(b) {
		return false
	}
	if scope == CascadeBrakesAndStations {
		return true
	}
	return b.cfg.Kind == config.BlockBrake
}

func (c *Coaster) cascadeScope() string {
	if c.opts.CascadeScope != "" {
		return c.opts.CascadeScope
	}
	return c.cfg.Failures.CascadeStop.Scope
}

// watchCascade arms the cascade check for a hold that just started.
func (c *Coaster) watchCascade(t *train) {
	cs := c.cfg.Failures.CascadeStop
	if !c.cascadeEnabled || cs == nil || c.Stopped() || !countsForCascade(t.at, t.holdReason, c.cascadeScope()) {
		return
	}
	tol := cs.BlockedToleranceS
	if c.opts.CascadeToleranceS > 0 {
		tol = c.opts.CascadeToleranceS
	}
	seq, at := t.holdSeq, t.at
	c.sim.After(engine.Seconds(tol), func() {
		if !t.held || t.holdSeq != seq || t.at != at || c.Stopped() {
			return
		}
		c.cascadeStop(t, cs)
	})
}

// cascadeStop halts the ride because t was blocked too long. Without auto_resume an operator reset
// (operator_reset_s) is needed; with auto_resume the stop is logged and the ride resumes at once.
func (c *Coaster) cascadeStop(t *train, cs *config.CascadeStop) {
	c.sim.Emit(engine.Event{Type: engine.EvCascade, Train: t.id, Block: t.at.cfg.ID, Info: t.holdReason})
	reset := engine.Duration(0)
	if !cs.AutoResume {
		reset = c.sample(cs.OperatorResetS, engine.StreamFailures)
	}
	c.StopFor(CauseCascadeStop, reset)
}
