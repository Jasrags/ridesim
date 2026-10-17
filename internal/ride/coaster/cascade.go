package coaster

import (
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// Cascade scopes: which holds count toward cascade_stop.
const (
	// CascadeBrakesAndStations counts next_occupied holds at brake and station blocks (decision
	// 2026-10-17: unplanned holds only; designed waits never count).
	CascadeBrakesAndStations = "brakes_and_stations"
	// CascadeBrakesOnly also excludes stations, where waiting for the next station block is the normal pipeline.
	CascadeBrakesOnly = "brakes_only"
)

// countsForCascade reports whether a hold of reason at b counts toward cascade_stop under scope.
func countsForCascade(b *block, reason, scope string) bool {
	if reason != ReasonNextOccupied || !pausesAt(b) {
		return false
	}
	if scope == CascadeBrakesOnly {
		return b.cfg.Kind == config.BlockBrake
	}
	return true
}

// watchCascade arms the cascade check for a hold that just started.
func (c *Coaster) watchCascade(t *train) {
	cs := c.cfg.Failures.CascadeStop
	if !c.cascadeEnabled || cs == nil || c.Stopped() || !countsForCascade(t.at, t.holdReason, c.opts.CascadeScope) {
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
