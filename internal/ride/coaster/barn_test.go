package coaster

import (
	"testing"

	"github.com/jasrags/ridesim/internal/engine"
)

func lastN(evs []engine.Event, typ string) (engine.Event, bool) {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == typ {
			return evs[i], true
		}
	}
	return engine.Event{}, false
}

func TestRemoveTrainSwapsOutAfterUnload(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 3, FixedTimes: true}, 20000, time30())
	h.sim.At(sec(300), h.c.RemoveTrain)
	h.sim.RunUntil(engine.Time(time30()))
	evs := h.sim.Log.Events()
	swap := firstAt(evs, engine.EvBarnSwap, "", sec(300))
	gone, ok := lastN(evs, engine.EvTrainToBarn)
	if swap < 0 || !ok {
		t.Fatal("no barn swap / train_to_barn")
	}
	if gone.T.Sub(swap) != 100*engine.Second || gone.Block != "UNLOAD" || gone.N != 2 {
		t.Fatalf("train_to_barn %+v, swap at %v: want 100 s swap at UNLOAD leaving 2 trains", gone, swap)
	}
	if h.c.InService() != 2 {
		t.Fatalf("in service %d, want 2", h.c.InService())
	}
	if firstAt(evs, engine.EvDispatch, "", gone.T) < 0 {
		t.Fatal("no dispatch after the swap")
	}
}

func TestAddTrainInsertsAtUnload(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 2, FixedTimes: true}, 20000, time30())
	h.sim.At(sec(300), h.c.AddTrain)
	h.sim.RunUntil(engine.Time(time30()))
	evs := h.sim.Log.Events()
	in, ok := lastN(evs, engine.EvTrainFromBarn)
	if !ok || in.Train != 3 || in.N != 3 || in.Block != "UNLOAD" {
		t.Fatalf("train_from_barn %+v, want train 3 at UNLOAD making 3", in)
	}
	if firstAt(evs, engine.EvRideResume, "", in.T) != in.T {
		t.Fatal("ride did not resume when the swap finished")
	}
	n := 0
	for _, e := range evs {
		if e.Type == engine.EvDispatch && e.Train == 3 {
			n++
		}
	}
	if n == 0 {
		t.Fatal("inserted train never dispatched")
	}
}

func TestOpsRejectedOutsideFleetLimits(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 3, FixedTimes: true}, 0, time30())
	h.c.AddTrain() // max_in_service is 3
	if _, ok := lastN(h.sim.Log.Events(), engine.EvOpsRejected); !ok {
		t.Fatal("adding a 4th train was not rejected")
	}
}

func time30() engine.Duration { return 30 * engine.Minute }
