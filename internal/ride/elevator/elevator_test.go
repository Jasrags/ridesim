package elevator

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/seats"
)

const fixturePath = "../../config/testdata/test-fx-elevator.yaml"

func fixture(t *testing.T) *config.ElevatorFile {
	t.Helper()
	f, err := config.LoadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status == config.StatusFail {
			t.Fatalf("fixture invalid: %s %s", r.CheckID, r.Detail)
		}
	}
	return f.Elevator
}

// run builds the fixture ride with solo guests arriving at rate per hour, lets setup act on it, and
// runs for d.
func run(t *testing.T, cfg *config.ElevatorFile, opts Options, rate float64, d engine.Duration, setup func(*Ride)) []engine.Event {
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

// ridersBetween sums riders dispatched in [from, to), optionally only from cabs in ids.
func ridersBetween(evs []engine.Event, from, to engine.Time, ids map[int]bool) int {
	n := 0
	for _, e := range evs {
		if e.Type == engine.EvDispatch && e.T >= from && e.T < to && (ids == nil || ids[e.Train]) {
			n += e.N
		}
	}
	return n
}

const saturated = 4000.0

// Per side the cab fleet is the bottleneck: lap 20 + 85 + 40 + 10 = 155 s over 3 cabs = 51.7 s, so
// 2 sides x 10 seats x 3600 / 51.7 = 1,393/hr with fixed times and full cabs.
func TestFixedTimesHitTheSideInterval(t *testing.T) {
	cfg := fixture(t)
	evs := run(t, cfg, Options{FixedTimes: true}, saturated, 5*engine.Hour, nil)
	got := float64(ridersBetween(evs, engine.Time(engine.Hour), engine.Time(5*engine.Hour), nil)) / 4
	want := 2 * 10 * 3600 / cfg.SideInterval()
	if math.Abs(got-want) > want*0.01 {
		t.Errorf("%.0f riders/hr, want %.0f (side interval %.1f s)", got, want, cfg.SideInterval())
	}
}

// One side out for the whole window: only the other side's cabs dispatch, at half the capacity.
func TestSideDownHalvesCapacity(t *testing.T) {
	cfg := fixture(t)
	evs := run(t, cfg, Options{FixedTimes: true}, saturated, 5*engine.Hour, func(r *Ride) {
		r.sim.At(engine.Time(30*engine.Minute), func() { r.SideDownFor(0, 10*engine.Hour) })
	})
	from, to := engine.Time(engine.Hour), engine.Time(5*engine.Hour)
	if n := ridersBetween(evs, from, to, map[int]bool{1: true, 2: true, 3: true}); n != 0 {
		t.Errorf("side %q cabs carried %d riders while down", "left", n)
	}
	got := float64(ridersBetween(evs, from, to, nil)) / 4
	want := 10 * 3600 / cfg.SideInterval()
	if math.Abs(got-want) > want*0.01 {
		t.Errorf("%.0f riders/hr with one side down, want %.0f", got, want)
	}
}

// A ride stop freezes every cab: nothing enters a place, loads, dispatches or unloads until it resumes,
// and each cab then finishes the step it was in with the time it had left.
func TestRideStopFreezesEveryCab(t *testing.T) {
	stop, length := engine.Time(2*engine.Hour), 20*engine.Minute
	evs := run(t, fixture(t), Options{FixedTimes: true}, saturated, 3*engine.Hour, func(r *Ride) {
		r.sim.At(stop, func() { r.StopFor(CauseBriefStop, length) })
	})
	for _, e := range evs {
		if e.T > stop && e.T < stop.Add(length) && e.Type != engine.EvQueueJoin && e.Type != engine.EvPostedWait && e.Type != engine.EvBalk && e.Type != engine.EvRenege {
			t.Fatalf("%s at %v during the stop", e.Type, e.T)
		}
	}
	if ridersBetween(evs, stop.Add(length), stop.Add(length+10*engine.Minute), nil) == 0 {
		t.Error("nothing dispatched after the ride resumed")
	}
}

// Every hold ends, every boarded guest unloads, and no place ever holds two cabs.
func TestPlacesHoldOneCabAndGuestsGetOff(t *testing.T) {
	cfg := fixture(t)
	evs := run(t, cfg, Options{Failures: []string{config.FailBriefStop, config.FailSideDown}}, saturated, 8*engine.Hour, nil)
	at := map[int]string{}
	occupant := map[string]int{}
	boarded, unloaded, open := 0, 0, 0
	for _, e := range evs {
		switch e.Type {
		case engine.EvTrainEnter:
			if e.Block == LoadBlock {
				continue
			}
			if o, ok := occupant[e.Block]; ok && o != e.Train {
				t.Fatalf("cab %d entered %s held by cab %d at %v", e.Train, e.Block, o, e.T)
			}
			delete(occupant, at[e.Train])
			at[e.Train], occupant[e.Block] = e.Block, e.Train
		case engine.EvBoard:
			boarded += e.N
		case engine.EvUnload:
			unloaded += e.N
		case engine.EvHoldStart:
			open++
		case engine.EvHoldEnd:
			open--
		}
	}
	// At the end some guests are still aboard (at most every cab full) and some cabs still wait.
	if aboard := boarded - unloaded; aboard < 0 || aboard > cfg.Vehicles.FleetSize*cfg.Vehicles.SeatsPerVehicle {
		t.Errorf("boarded %d, unloaded %d", boarded, unloaded)
	}
	if open < 0 || open > cfg.Vehicles.FleetSize {
		t.Errorf("%d holds still open", open)
	}
	if unloaded == 0 {
		t.Fatal("nobody rode")
	}
}

func TestSameSeedSameLog(t *testing.T) {
	encode := func(evs []engine.Event) []byte {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		for _, e := range evs {
			if err := enc.Encode(e); err != nil {
				t.Fatal(err)
			}
		}
		return b.Bytes()
	}
	opts := Options{Failures: []string{config.FailBriefStop, config.FailSideDown}}
	a := encode(run(t, fixture(t), opts, 1500, 6*engine.Hour, nil))
	b := encode(run(t, fixture(t), opts, 1500, 6*engine.Hour, nil))
	if !bytes.Equal(a, b) {
		t.Fatal("same seed, different logs")
	}
}

func TestNewRejectsGridlockingFleet(t *testing.T) {
	cfg := *fixture(t)
	cfg.Vehicles.PerSide = 5
	if _, err := New(engine.New(1), &cfg, Options{}); err == nil {
		t.Fatal("5 cabs for 5 places accepted")
	}
}

// Boarding the last party can re-enter Board (lines.Boarded drains the preshow, which calls Board), so the
// cab's check may already have started when loadDone or the outer Board resumes. A second startCheck
// must do nothing: no fresh restraint and clearance draw, no new dispatch timer.
func TestStartCheckRunsOncePerLoad(t *testing.T) {
	sim := engine.New(1)
	r, err := New(sim, fixture(t), Options{FixedTimes: true})
	if err != nil {
		t.Fatal(err)
	}
	c := r.cabs[0]
	r.loading = append(r.loading, c)
	r.startCheck(c)
	gen := c.work.gen
	r.startCheck(c)
	if c.work.gen != gen {
		t.Fatalf("second startCheck restarted the cab's work (gen %d -> %d)", gen, c.work.gen)
	}
}

// Cabs waiting for a door at startup hold a named place, not "". With 1 show shaft and 1 door, the
// third cab on a side starts with nowhere to go (2 drop shafts keep the fleet legal).
func TestStartupHoldsNameThePlace(t *testing.T) {
	cfg := *fixture(t)
	cfg.Sides.ShowShafts, cfg.Sides.DropShafts, cfg.Sides.UnloadDoors = 1, 2, 1
	holds := 0
	for _, e := range run(t, &cfg, Options{FixedTimes: true}, 0, engine.Minute, nil) {
		if e.Type == engine.EvHoldStart {
			holds++
		}
		if (e.Type == engine.EvHoldStart || e.Type == engine.EvHoldEnd) && e.Block == "" {
			t.Fatalf("%s for cab %d at %v has no block", e.Type, e.Train, e.T)
		}
	}
	if holds == 0 {
		t.Fatal("no startup holds: the test did not reach the case")
	}
}

// Liveness with a preshow, where boarding can re-enter Board through the preshow drain: every cab keeps
// dispatching through a long day. (The narrow re-entry case itself is guarded in Board and loadDone;
// TestStartCheckRunsOncePerLoad covers the idempotent check.)
func TestEveryCabKeepsCyclingWithPreshow(t *testing.T) {
	cfg := *fixture(t)
	cfg.Preshow = &config.Preshow{Enabled: true, Rooms: 2, BatchSize: 25, DurationS: config.Dist{Mean: 60, SD: 5},
		TurnoverS: config.Dist{Mean: 10, SD: 2}, FillRule: "full_or_max_wait", MaxFillWaitS: 20,
		ReleaseRule: "when_load_area_has_room", LoadAreaCapacity: 40, PartyIntegrity: true}
	evs := run(t, &cfg, Options{}, 1300, 8*engine.Hour, nil)
	last := map[int]engine.Time{}
	for _, e := range evs {
		if e.Type == engine.EvDispatch {
			last[e.Train] = e.T
		}
	}
	for id := 1; id <= cfg.Vehicles.FleetSize; id++ {
		if last[id] < engine.Time(7*engine.Hour) {
			t.Errorf("cab %d last dispatched at %v: stuck", id, last[id])
		}
	}
}

// [user] Tower of Terror: five boarding groups of 3, 4, 3, 4, 7. A party sits in the smallest group with
// room for it; one no group can hold is split over the groups with the most free seats.
func TestPlaceInGroups(t *testing.T) {
	cases := []struct {
		name       string
		free       []int
		size       int
		wantFreeAt []int
	}{
		{"pair takes the smallest group that fits", []int{3, 4, 3, 4, 7}, 2, []int{1, 4, 3, 4, 7}},
		{"four skips the threes", []int{3, 4, 3, 4, 7}, 4, []int{3, 0, 3, 4, 7}},
		{"six goes to the back row", []int{3, 4, 3, 4, 7}, 6, []int{3, 4, 3, 4, 1}},
		{"tie goes to the front", []int{3, 3}, 3, []int{0, 3}},
		{"too big for any group: split from the fullest", []int{3, 4, 3, 2, 1}, 6, []int{1, 0, 3, 2, 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			free := append([]int(nil), c.free...)
			seats.PlaceInGroups(free, c.size)
			for i := range free {
				if free[i] != c.wantFreeAt[i] {
					t.Fatalf("free after seating %d: %v, want %v", c.size, free, c.wantFreeAt)
				}
			}
		})
	}
}

// Callouts [user]: with a mix of party sizes, cast call a party that fits forward when the head party
// cannot use the spare seats, so cabs leave nearly full and fuller than without callouts.
func TestCalloutsFillCabs(t *testing.T) {
	seatsUsed := func(callout int) float64 {
		cfg := *fixture(t)
		cfg.Vehicles.SeatsPerVehicle, cfg.Vehicles.BoardingGroups = 10, []int{3, 3, 4}
		cfg.Cycle.CalloutParties = callout
		sim := engine.New(1)
		r, err := New(sim, &cfg, Options{FixedTimes: true})
		if err != nil {
			t.Fatal(err)
		}
		r.Start()
		prof := guest.Profile{PartySizes: guest.PartySizesFrom(map[int]float64{1: 0.1, 2: 0.35, 3: 0.2, 4: 0.25, 5: 0.07, 6: 0.03})}
		guest.StartArrivalsWith(sim, guest.Curve{{RatePerHr: 4000}}, prof, r, engine.Time(4*engine.Hour))
		sim.RunUntil(engine.Time(4 * engine.Hour))
		riders, cabs := 0, 0
		for _, e := range sim.Log.Events() {
			if e.Type == engine.EvDispatch && e.T > engine.Time(engine.Hour) {
				riders += e.N
				cabs++
			}
		}
		return float64(riders) / float64(cabs)
	}
	without, with := seatsUsed(0), seatsUsed(10)
	t.Logf("riders per 10-seat cab: %.2f without callouts, %.2f with", without, with)
	if with < 9.5 || with <= without {
		t.Errorf("callouts: %.2f riders per cab (without %.2f); want nearly full and fuller", with, without)
	}
}
