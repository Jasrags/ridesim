package coaster

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func fixture(t *testing.T) *config.CoasterFile {
	t.Helper()
	f, err := config.LoadFile("../../config/testdata/test-fx-coaster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.Coaster.Preshow = nil // model tests exercise track and station timing; preshow has its own tests
	return f.Coaster
}

type fill struct{ c *Coaster }

func (f fill) Arrive(p *guest.Party, l guest.Lane) { f.c.Arrive(p, l) }

func runFixture(t *testing.T, trains int, d engine.Duration) []engine.Event {
	t.Helper()
	sim := engine.New(1)
	c, err := New(sim, fixture(t), Options{Trains: trains, FixedTimes: true})
	if err != nil {
		t.Fatal(err)
	}
	c.Start()
	guest.StartArrivals(sim, guest.Curve{{RatePerHr: 20000}}, fill{c}, engine.Time(d))
	sim.RunUntil(engine.Time(d))
	return sim.Log.Events()
}

func dispatchTimes(evs []engine.Event) []engine.Time {
	var out []engine.Time
	for _, e := range evs {
		if e.Type == engine.EvDispatch {
			out = append(out, e.T)
		}
	}
	return out
}

// One train never waits: interval = lap = track 100 s + unload 20 + load 25 + check 5 + clearance 2.
func TestSingleTrainLapIsHandMath(t *testing.T) {
	evs := runFixture(t, 1, 20*engine.Minute)
	ds := dispatchTimes(evs)
	if len(ds) < 3 {
		t.Fatalf("only %d dispatches", len(ds))
	}
	for i := 1; i < len(ds); i++ {
		if got := ds[i].Sub(ds[i-1]); got != 152*engine.Second {
			t.Fatalf("interval %d = %v ms, want 152 s", i, got)
		}
	}
	for _, e := range evs {
		if e.Type == engine.EvHoldStart && e.Block == "BRAKE" {
			t.Fatalf("train held at an empty brake: %+v", e)
		}
	}
}

// With 3 trains the station (32 s load cycle) outpaces the loop, so trains stack at BRAKE behind
// UNLOAD and each held train pays the 2 s release delay after UNLOAD clears.
func TestHeldBrakeReleasesAfterDelay(t *testing.T) {
	evs := runFixture(t, 3, 30*engine.Minute)
	var holdEnd, nextEnter engine.Time = -1, -1
	for _, e := range evs {
		if holdEnd < 0 && e.Type == engine.EvHoldEnd && e.Block == "BRAKE" {
			holdEnd = e.T
		}
		if holdEnd >= 0 && e.Type == engine.EvTrainEnter && e.Block == "UNLOAD" && e.T >= holdEnd {
			nextEnter = e.T
			break
		}
	}
	if holdEnd < 0 {
		t.Fatal("no hold at BRAKE with 3 trains")
	}
	if nextEnter != holdEnd {
		t.Fatalf("held train entered UNLOAD at %v, hold ended %v; departure should end the hold", nextEnter, holdEnd)
	}
	var lastClear engine.Time = -1
	for _, e := range evs { // UNLOAD clears when its train enters LOAD
		if e.Type == engine.EvTrainEnter && e.Block == "LOAD" && e.T <= holdEnd {
			lastClear = e.T
		}
	}
	if holdEnd.Sub(lastClear) != 2*engine.Second {
		t.Fatalf("release %v after UNLOAD cleared, want the 2 s release delay", holdEnd.Sub(lastClear))
	}
}

func TestRejectsTooManyTrains(t *testing.T) {
	if _, err := New(engine.New(1), fixture(t), Options{Trains: 4}); err == nil {
		t.Fatal("expected error for 4 trains with max_in_service 3")
	}
}
