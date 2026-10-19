package coaster

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// dualFixture is the test coaster with a second load platform (LOAD_B) and a long load, so one platform cannot
// keep up with the launch interval.
func dualFixture(t *testing.T, dual bool) *config.CoasterFile {
	t.Helper()
	f := fixture(t)
	f.Station.LoadS = config.Dist{Mean: 80, SD: 0}
	f.Trains.FleetSize, f.Trains.MaxInService = 4, 4
	if !dual {
		return f
	}
	blocks := []config.Block{}
	for _, b := range f.Blocks {
		if b.ID == "UNLOAD" {
			b.AltNext = config.Refs{"LOAD_B"}
		}
		blocks = append(blocks, b)
		if b.ID == "LOAD" {
			blocks = append(blocks, config.Block{ID: "LOAD_B", Kind: config.BlockStation, Next: "LIFT"})
		}
	}
	f.Blocks = blocks
	f.Clearance.Rules = append(f.Clearance.Rules, config.ClearanceRule{ID: "dispatch_from_load_b", From: "LOAD_B", RequiresEmpty: []string{"LIFT"},
		Conditions: []config.Condition{{Kind: config.CondSwitchAligned, Switch: "SW1"}}})
	return f
}

func runCoaster(t *testing.T, f *config.CoasterFile, opts Options, d engine.Duration) []engine.Event {
	t.Helper()
	sim := engine.New(1)
	c, err := New(sim, f, opts)
	if err != nil {
		t.Fatal(err)
	}
	c.Start()
	guest.StartArrivals(sim, guest.Curve{{RatePerHr: 20000}}, fill{c}, engine.Time(d))
	sim.RunUntil(engine.Time(d))
	return sim.Log.Events()
}

// Two platforms load in parallel: trains dispatch from both, and far more often than from one platform.
func TestDualLoadDispatchesFromBothPlatforms(t *testing.T) {
	opts := Options{Trains: 4, FixedTimes: true}
	single := dispatchTimes(runCoaster(t, dualFixture(t, false), opts, 2*engine.Hour))
	evs := runCoaster(t, dualFixture(t, true), opts, 2*engine.Hour)
	dual := dispatchTimes(evs)
	from := map[string]int{}
	for _, e := range evs {
		if e.Type == engine.EvDispatch {
			from[e.Block]++
		}
	}
	if from["LOAD"] == 0 || from["LOAD_B"] == 0 {
		t.Fatalf("dispatches by platform %v, want both", from)
	}
	if float64(len(dual)) < 1.4*float64(len(single)) {
		t.Errorf("%d dispatches with two platforms vs %d with one, want at least 1.4x", len(dual), len(single))
	}
	for i := 1; i < len(dual); i++ {
		if gap := dual[i].Sub(dual[i-1]); gap < engine.Seconds(40) {
			t.Fatalf("dispatches %v apart, under the shared 40 s minimum interval", gap)
		}
	}
}

// side_down closes the second platform: no train enters it while it is out, and it is used again after.
func TestDualLoadSideDown(t *testing.T) {
	f := dualFixture(t, true)
	f.Failures.SideDown = &config.RateStop{RatePerHr: 2, DurationS: config.Dist{Mean: 600, SD: 0}}
	evs := runCoaster(t, f, Options{Trains: 4, FixedTimes: true, Failures: []string{config.FailSideDown}}, 4*engine.Hour)
	var outs [][2]engine.Time
	for _, e := range evs {
		if e.Type == EvSideDown {
			outs = append(outs, [2]engine.Time{e.T, e.T.Add(e.D)})
		}
	}
	if len(outs) == 0 {
		t.Fatal("no side_down in 4 h at 2/hr")
	}
	usedAfter := false
	for _, e := range evs {
		if e.Type != engine.EvTrainEnter || e.Block != "LOAD_B" {
			continue
		}
		for _, o := range outs {
			if e.T >= o[0] && e.T < o[1] {
				t.Fatalf("train entered LOAD_B at %v during the outage %v-%v", e.T, o[0], o[1])
			}
			if e.T >= o[1] {
				usedAfter = true
			}
		}
	}
	if !usedAfter {
		t.Error("LOAD_B never used after an outage ended")
	}
}

// alt_next as a list: three platforms all dispatch, and side_down picks among the further ones.
func TestMultiLoadDispatchesFromEveryPlatform(t *testing.T) {
	f := dualFixture(t, true)
	f.Trains.FleetSize, f.Trains.MaxInService = 5, 5
	blocks := []config.Block{}
	for _, b := range f.Blocks {
		if b.ID == "UNLOAD" {
			b.AltNext = config.Refs{"LOAD_B", "LOAD_C"}
		}
		blocks = append(blocks, b)
		if b.ID == "LOAD_B" {
			blocks = append(blocks, config.Block{ID: "LOAD_C", Kind: config.BlockStation, Next: "LIFT"})
		}
	}
	f.Blocks = blocks
	f.Failures.SideDown = &config.RateStop{RatePerHr: 2, DurationS: config.Dist{Mean: 600, SD: 0}}
	evs := runCoaster(t, f, Options{Trains: 5, FixedTimes: true, Failures: []string{config.FailSideDown}}, 4*engine.Hour)
	from, down := map[string]int{}, map[string]int{}
	for _, e := range evs {
		switch e.Type {
		case engine.EvDispatch:
			from[e.Block]++
		case EvSideDown:
			down[e.Block]++
		}
	}
	if from["LOAD"] == 0 || from["LOAD_B"] == 0 || from["LOAD_C"] == 0 {
		t.Fatalf("dispatches by platform %v, want all three", from)
	}
	if down["LOAD"] > 0 || down["LOAD_B"]+down["LOAD_C"] == 0 {
		t.Errorf("side_down by platform %v: want only further platforms", down)
	}
}
