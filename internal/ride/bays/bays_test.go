package bays

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func fixture(t *testing.T) *config.BaysFile {
	t.Helper()
	f, err := config.LoadFile("../../config/testdata/test-fx-bays.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status == config.StatusFail {
			t.Fatalf("fixture invalid: %s %s", r.CheckID, r.Detail)
		}
	}
	return f.Bays
}

func run(t *testing.T, cfg *config.BaysFile, opts Options, rate float64, d engine.Duration, setup func(*Ride)) []engine.Event {
	t.Helper()
	sim := engine.New(1)
	r, err := New(sim, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	guest.StartArrivals(sim, guest.Curve{{RatePerHr: rate}}, r, engine.Time(d))
	if setup != nil {
		setup(r)
	}
	sim.RunUntil(engine.Time(d))
	return sim.Log.Events()
}

func riders(evs []engine.Event, from, to engine.Time, bay int) int {
	n := 0
	for _, e := range evs {
		if e.Type == engine.EvDispatch && e.T >= from && e.T < to && (bay == 0 || e.Train == bay) {
			n += e.N
		}
	}
	return n
}

// Full bays with fixed times: each launches every 10 + 20 + 5 + 5 + 60 = 100 s, so 2 x 10 x 36 = 720/hr.
func TestFixedTimesHitTheCycle(t *testing.T) {
	evs := run(t, fixture(t), Options{FixedTimes: true}, 3000, 5*engine.Hour, nil)
	got := float64(riders(evs, engine.Time(engine.Hour), engine.Time(5*engine.Hour), 0)) / 4
	if math.Abs(got-720) > 720*0.01 {
		t.Errorf("%.0f riders/hr, want 720", got)
	}
}

// Bays run their own schedules (Jason): a bay out freezes only that bay; the other keeps flying.
func TestBayDownLeavesTheOtherFlying(t *testing.T) {
	evs := run(t, fixture(t), Options{FixedTimes: true}, 3000, 5*engine.Hour, func(r *Ride) {
		r.sim.At(engine.Time(30*engine.Minute), func() { r.BayDownFor(0, 10*engine.Hour) })
	})
	from, to := engine.Time(engine.Hour), engine.Time(5*engine.Hour)
	if n := riders(evs, from, to, 1); n != 0 {
		t.Errorf("bay 1 carried %d riders while down", n)
	}
	if got := float64(riders(evs, from, to, 2)) / 4; math.Abs(got-360) > 360*0.01 {
		t.Errorf("bay 2 alone: %.0f riders/hr, want 360", got)
	}
}

// A ride stop freezes both bays: no boarding, launch or exit until it resumes.
func TestRideStopFreezesEveryBay(t *testing.T) {
	stop, length := engine.Time(2*engine.Hour), 20*engine.Minute
	evs := run(t, fixture(t), Options{FixedTimes: true}, 3000, 3*engine.Hour, func(r *Ride) {
		r.sim.At(stop, func() { r.StopFor(CauseBriefStop, length) })
	})
	for _, e := range evs {
		if e.T > stop && e.T < stop.Add(length) {
			switch e.Type {
			case engine.EvBoard, engine.EvDispatch, engine.EvUnload, engine.EvTrainEnter, engine.EvLoadDone:
				t.Fatalf("%s at %v during the stop", e.Type, e.T)
			}
		}
	}
	if riders(evs, stop.Add(length), stop.Add(length+10*engine.Minute), 0) == 0 {
		t.Error("nothing launched after the ride resumed")
	}
}

// A second startCheck for the same load does nothing (boarding can re-enter Board through a preshow).
func TestStartCheckRunsOncePerLoad(t *testing.T) {
	r, err := New(engine.New(1), fixture(t), Options{FixedTimes: true})
	if err != nil {
		t.Fatal(err)
	}
	b := r.bays[0]
	r.loading = append(r.loading, b)
	r.startCheck(b)
	gen := b.work.gen
	r.startCheck(b)
	if b.work.gen != gen {
		t.Fatalf("second startCheck restarted the bay's work (gen %d -> %d)", gen, b.work.gen)
	}
}

func TestSameSeedSameLog(t *testing.T) {
	encode := func(evs []engine.Event) []byte {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		for _, e := range evs {
			if err := enc.Encode(e); err != nil {
				t.Fatal(err)
			}
		}
		return buf.Bytes()
	}
	opts := Options{Failures: []string{config.FailBriefStop, config.FailSideDown}}
	if !bytes.Equal(encode(run(t, fixture(t), opts, 600, 6*engine.Hour, nil)), encode(run(t, fixture(t), opts, 600, 6*engine.Hour, nil))) {
		t.Fatal("same seed, different logs")
	}
}
