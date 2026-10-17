package coaster

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

func TestCountsForCascade(t *testing.T) {
	brake := &block{cfg: &config.Block{Kind: config.BlockBrake}, role: roleTrack}
	station := &block{cfg: &config.Block{Kind: config.BlockStation}, role: roleUnload}
	course := &block{cfg: &config.Block{Kind: config.BlockCourse}, role: roleTrack}
	cases := []struct {
		b      *block
		reason string
		scope  string
		want   bool
	}{
		{brake, ReasonNextOccupied, "", true},
		{station, ReasonNextOccupied, "", false}, // default scope is brakes only
		{station, ReasonNextOccupied, CascadeBrakesAndStations, true},
		{station, ReasonNextOccupied, CascadeBrakesOnly, false},
		{course, ReasonNextOccupied, "", false},
		{brake, ReasonHoldUntilClear, "", false},
		{brake, ReasonClearance, "", false},
		{station, ReasonSwitchRealign, "", false},
		{station, ReasonMinInterval, "", false},
		{brake, ReasonRideStopped, "", false},
	}
	for _, c := range cases {
		if got := countsForCascade(c.b, c.reason, c.scope); got != c.want {
			t.Errorf("%s at %s (scope %q) = %v, want %v", c.reason, c.b.cfg.Kind, c.scope, got, c.want)
		}
	}
}

// With 3 trains on the fixture, trains wait at BRAKE for UNLOAD. A 0.5 s tolerance turns those holds
// into cascade stops, each lasting the 30 s operator reset (fixed times).
func TestCascadeStopTriggersAndResets(t *testing.T) {
	opts := Options{Trains: 3, FixedTimes: true, Failures: []string{config.FailCascadeStop}, CascadeToleranceS: 0.5}
	h := newHarness(t, fixture(t), opts, 20000, 30*engine.Minute)
	h.sim.RunUntil(engine.Time(30 * engine.Minute))
	evs := h.sim.Log.Events()
	var cascade, stop, resume engine.Time = -1, -1, -1
	for _, e := range evs {
		switch {
		case cascade < 0 && e.Type == engine.EvCascade:
			cascade = e.T
			if e.Train == 0 || e.Block == "" {
				t.Errorf("cascade event missing train/block: %+v", e)
			}
		case cascade >= 0 && stop < 0 && e.Type == engine.EvRideStop && e.Info == CauseCascadeStop:
			stop = e.T
		case stop >= 0 && resume < 0 && e.Type == engine.EvRideResume && e.Info == CauseCascadeStop:
			resume = e.T
		}
	}
	if cascade < 0 || stop != cascade {
		t.Fatalf("cascade at %v, stop at %v: want a cascade stop", cascade, stop)
	}
	if resume.Sub(stop) != 30*engine.Second {
		t.Fatalf("cascade stop lasted %v ms, want the 30 s operator reset", resume.Sub(stop))
	}
}

func TestCascadeOffOrTolerantMeansNoStops(t *testing.T) {
	for name, opts := range map[string]Options{
		"disabled":  {Trains: 3, FixedTimes: true, CascadeToleranceS: 0.5},
		"tolerant":  {Trains: 3, FixedTimes: true, Failures: []string{config.FailCascadeStop}, CascadeToleranceS: 3600},
		"brakeonly": {Trains: 1, FixedTimes: true, Failures: []string{config.FailCascadeStop}, CascadeToleranceS: 0.5, CascadeScope: CascadeBrakesOnly},
	} {
		h := newHarness(t, fixture(t), opts, 20000, 30*engine.Minute)
		h.sim.RunUntil(engine.Time(30 * engine.Minute))
		if n := countEvents(h.sim.Log.Events(), engine.EvRideStop, CauseCascadeStop); n != 0 {
			t.Errorf("%s: %d cascade stops, want 0", name, n)
		}
	}
}
