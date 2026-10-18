package continuous

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// A party larger than one buggy is told it rode once, when its last buggy unloads.
func TestLargePartyIsToldItRodeOnceAfterLastBuggy(t *testing.T) {
	f, err := config.LoadFile("../../config/testdata/test-fx-omnimover.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.Continuous
	cfg.Preshow = nil
	sim := engine.New(1)
	r, err := New(sim, cfg, Options{FixedTimes: true, NoAccessibilityHolds: true})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	var exits []engine.Time
	p := &guest.Party{ID: 1, Size: cfg.Vehicles.SeatsMax + 1}
	p.OnExit = func(e guest.Exit) {
		if e == guest.Rode {
			exits = append(exits, sim.Now())
		}
	}
	sim.At(engine.Time(engine.Second), func() { r.Arrive(p, guest.Standby) })
	sim.RunUntil(engine.Time(engine.Hour))
	var unloads []engine.Time
	for _, e := range sim.Log.Events() {
		if e.Type == engine.EvUnload {
			unloads = append(unloads, e.T)
		}
	}
	if len(exits) != 1 || len(unloads) != 2 || exits[0] != unloads[1] {
		t.Fatalf("exits %v unloads %v, want one exit at the second unload", exits, unloads)
	}
}

// One chain out of several freezes its vehicles and load point; the others keep running, so a fixture
// with two chains carries half while one is down.
func TestChainDownHalvesTwoChains(t *testing.T) {
	f, err := config.LoadFile("../../config/testdata/test-fx-omnimover.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := *f.Continuous
	cfg.Preshow = nil
	cfg.Conveyor.Chains = 2
	riders := func(down bool) int {
		sim := engine.New(1)
		r, err := New(sim, &cfg, Options{FixedTimes: true, NoAccessibilityHolds: true})
		if err != nil {
			t.Fatal(err)
		}
		r.Start()
		guest.StartArrivals(sim, guest.Curve{{RatePerHr: 6000}}, r, engine.Time(3*engine.Hour))
		if down {
			sim.At(engine.Time(30*engine.Minute), func() { r.ChainDownFor(0, 10*engine.Hour) })
		}
		sim.RunUntil(engine.Time(3 * engine.Hour))
		n := 0
		for _, e := range sim.Log.Events() {
			if e.Type == engine.EvDispatch && e.T > engine.Time(engine.Hour) {
				if down && e.Train <= cfg.Vehicles.Count {
					t.Fatalf("buggy %d on the down chain dispatched at %v", e.Train, e.T)
				}
				n += e.N
			}
		}
		return n
	}
	up, down := riders(false), riders(true)
	if d := float64(down) / float64(up); d < 0.48 || d > 0.52 {
		t.Errorf("one of two chains down carried %d vs %d (%.2f), want half", down, up, d)
	}
}
