package queue

import (
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// EvMergePhase is logged when the merge changes phase. Info = phase name, N = LL queue guests.
const EvMergePhase = "merge_phase"

// Merge decides, at a ride's merge point, whether the next party comes from Lightning Lane or standby
// (spec §8). Each phase admits up to ll_per_standby LL units per standby unit while both lines have
// someone; an empty line hands everything to the other (empty_lane_rule pull_other). The unit is guests
// or parties. The phase follows the LL queue length: it moves up when the queue reaches a phase's
// threshold and back down only once the queue is phase_hysteresis_guests below it, so it cannot flap.
type Merge struct {
	sim        *engine.Sim
	phases     []config.MergePhase
	hysteresis int
	byParties  bool

	phase  int
	credit int // LL units the merge may still admit before the next standby unit
}

// NewMerge builds a merge from a ride's skip_line.merge section. Phases must be validated
// (increasing thresholds, last unbounded).
func NewMerge(sim *engine.Sim, cfg config.Merge) *Merge {
	return &Merge{sim: sim, phases: cfg.Phases, hysteresis: cfg.PhaseHysteresisGuests, byParties: cfg.Unit == "parties"}
}

// Phase is the current phase.
func (m *Merge) Phase() config.MergePhase { return m.phases[m.phase] }

func (m *Merge) units(p *guest.Party) int {
	if m.byParties {
		return 1
	}
	return p.Size
}

// Update recomputes the phase for an LL queue of llGuests guests.
func (m *Merge) Update(llGuests int) {
	next := m.phase
	for next+1 < len(m.phases) && llGuests >= *m.phases[next].WhenLLQueueGuestsBelow {
		next++ // up: queue reached this phase's ceiling
	}
	for next > 0 && llGuests < *m.phases[next-1].WhenLLQueueGuestsBelow-m.hysteresis {
		next-- // down: queue fell clearly below the previous phase's ceiling
	}
	if next != m.phase {
		m.phase = next
		m.sim.Emit(engine.Event{Type: EvMergePhase, N: llGuests, Info: m.phases[next].Name})
	}
}

// Choose reports whether the next party should be the LL head (true) or the standby head (false).
// Either head may be nil; with both nil the answer is irrelevant.
func (m *Merge) Choose(ll, standby *guest.Party) bool {
	switch {
	case ll == nil:
		return false
	case standby == nil:
		return true
	}
	return m.credit > 0
}

// Took records that p went through the merge; llWaiting says whether the LL line still had someone
// before p left it (credit only accrues while LL guests are waiting, so an idle LL line banks nothing).
func (m *Merge) Took(p *guest.Party, llWaiting bool) {
	if p.Lane == guest.LightningLane {
		m.credit -= m.units(p)
		if m.credit < 0 {
			m.credit = 0
		}
		return
	}
	if llWaiting {
		m.credit += m.Phase().LLPerStandby * m.units(p)
	}
}
