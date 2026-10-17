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
