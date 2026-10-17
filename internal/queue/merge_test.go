package queue

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func ip(v int) *int { return &v }

func testMerge(unit string) *Merge {
	return NewMerge(engine.New(1), config.Merge{Unit: unit, EmptyLaneRule: "pull_other", PhaseHysteresisGuests: 20, Phases: []config.MergePhase{
		{Name: "normal", WhenLLQueueGuestsBelow: ip(60), LLPerStandby: 2},
		{Name: "elevated", WhenLLQueueGuestsBelow: ip(200), LLPerStandby: 4},
		{Name: "recovery", LLPerStandby: 10},
	}})
}

// drain runs n choices with both lines always non-empty and returns LL guests per standby guest.
func ratio(m *Merge, llSize, stdSize, n int) float64 {
	var ll, std int
	for i := 0; i < n; i++ {
		lp := &guest.Party{Size: llSize, Lane: guest.LightningLane}
		sp := &guest.Party{Size: stdSize, Lane: guest.Standby}
		if m.Choose(lp, sp) {
			m.Took(lp, true)
			ll += llSize
		} else {
			m.Took(sp, true)
			std += stdSize
		}
	}
	return float64(ll) / float64(std)
}

func TestMergeRatioPerPhase(t *testing.T) {
	for _, c := range []struct {
		queue int
		want  float64
		phase string
	}{{10, 2, "normal"}, {100, 4, "elevated"}, {500, 10, "recovery"}} {
		m := testMerge("guests")
		m.Update(c.queue)
		if m.Phase().Name != c.phase {
			t.Fatalf("queue %d: phase %s, want %s", c.queue, m.Phase().Name, c.phase)
		}
		if got := ratio(m, 1, 1, 11000); got < c.want*0.99 || got > c.want*1.01 {
			t.Errorf("%s: %.2f LL per standby guest, want %.0f", c.phase, got, c.want)
		}
	}
}

// Unit guests: a standby party of 4 earns 8 LL guests in normal phase; unit parties: 2 LL parties.
func TestMergeUnits(t *testing.T) {
	if got := ratio(testMerge("guests"), 2, 4, 3000); got < 1.98 || got > 2.02 {
		t.Errorf("guests unit: %.2f LL guests per standby guest, want 2", got)
	}
	if got := ratio(testMerge("parties"), 2, 4, 3000); got < 0.98 || got > 1.02 { // 2 parties of 2 per party of 4
		t.Errorf("parties unit: %.2f LL guests per standby guest, want 1", got)
	}
}

func TestMergePhaseHysteresis(t *testing.T) {
	m := testMerge("guests")
	seq := []struct {
		q    int
		want string
	}{{59, "normal"}, {60, "elevated"}, {45, "elevated"}, {41, "elevated"}, {39, "normal"},
		{250, "recovery"}, {185, "recovery"}, {179, "elevated"}, {10, "normal"}}
	for _, s := range seq {
		m.Update(s.q)
		if m.Phase().Name != s.want {
			t.Fatalf("queue %d: phase %s, want %s", s.q, m.Phase().Name, s.want)
		}
	}
}

func TestMergeEmptyLaneAndNoBanking(t *testing.T) {
	m := testMerge("guests")
	sp := &guest.Party{Size: 1, Lane: guest.Standby}
	lp := &guest.Party{Size: 1, Lane: guest.LightningLane}
	if m.Choose(nil, sp) || !m.Choose(lp, nil) {
		t.Fatal("empty lane must hand the merge to the other line")
	}
	for i := 0; i < 50; i++ { // standby flows while LL is idle: no credit banked
		m.Took(sp, false)
	}
	if m.Choose(lp, sp) {
		t.Fatal("LL got priority from credit banked while its line was empty")
	}
}
