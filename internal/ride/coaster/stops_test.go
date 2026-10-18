package coaster

import (
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

type harness struct {
	sim *engine.Sim
	c   *Coaster
}

func newHarness(t *testing.T, cfg *config.CoasterFile, opts Options, rate float64, d engine.Duration) *harness {
	t.Helper()
	sim := engine.New(1)
	c, err := New(sim, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	c.Start()
	if rate > 0 {
		guest.StartArrivals(sim, guest.Curve{{RatePerHr: rate}}, fill{c}, engine.Time(d))
	}
	return &harness{sim, c}
}

func firstAt(evs []engine.Event, typ, block string, after engine.Time) engine.Time {
	for _, e := range evs {
		if e.Type == typ && (block == "" || e.Block == block) && e.T >= after {
			return e.T
		}
	}
	return -1
}

func sec(s float64) engine.Time { return engine.Time(engine.Seconds(s)) }

// One train starts in LOAD: 25 s load + 5 check + 2 clearance = dispatch at 32 s. A 30 s stop during
// loading pauses the station, so the dispatch moves exactly 30 s.
func TestStopPausesStationWorkExactly(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true}, 20000, 10*engine.Minute)
	h.sim.At(sec(10), func() { h.c.StopFor(CauseWeatherHold, 30*engine.Second) })
	h.sim.RunUntil(sec(600))
	evs := h.sim.Log.Events()
	if got := firstAt(evs, engine.EvDispatch, "", 0); got != sec(62) {
		t.Fatalf("first dispatch at %v, want 62 s", got)
	}
	if got := firstAt(evs, engine.EvDispatch, "", sec(63)); got != sec(62+152) {
		t.Fatalf("second dispatch at %v, want 214 s (lap unchanged after resume)", got)
	}
}

// Run to next brake: stopped at 40 s while on LIFT (32-62 s), the train keeps going through LIFT and
// COURSE (62-102 s), stops on BRAKE at 102 s with its full 30 s transit left, resumes at 240 s, and
// reaches UNLOAD at 270 s (132 s without the stop).
func TestStopRunsTrackTrainsToNextBrake(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true}, 20000, 10*engine.Minute)
	h.sim.At(sec(40), func() { h.c.StopFor(CauseBriefStop, 200*engine.Second) })
	h.sim.RunUntil(sec(600))
	evs := h.sim.Log.Events()
	if got := firstAt(evs, engine.EvTrainEnter, "COURSE", 0); got != sec(62) {
		t.Errorf("entered COURSE at %v, want 62 s (lift keeps running)", got)
	}
	if got := firstAt(evs, engine.EvTrainEnter, "BRAKE", 0); got != sec(102) {
		t.Errorf("entered BRAKE at %v, want 102 s", got)
	}
	if got := firstAt(evs, engine.EvTrainEnter, "UNLOAD", sec(1)); got != sec(270) {
		t.Errorf("entered UNLOAD at %v, want 270 s", got)
	}
}

func TestStopCausesNest(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true}, 20000, 10*engine.Minute)
	h.sim.At(sec(5), func() { h.c.StopFor(CauseWeatherHold, 20*engine.Second) }) // 5-25
	h.sim.At(sec(15), func() { h.c.StopFor(CauseBriefStop, 30*engine.Second) })  // 15-45
	h.sim.RunUntil(sec(600))
	// stopped 5-45 s while loading: dispatch 32 + 40 = 72 s
	if got := firstAt(h.sim.Log.Events(), engine.EvDispatch, "", 0); got != sec(72) {
		t.Fatalf("first dispatch at %v, want 72 s", got)
	}
}

func TestNoBoardingWhileStopped(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true}, 0, 10*engine.Minute)
	h.sim.At(sec(1), func() { h.c.Stop(CauseWeatherHold) })
	h.sim.At(sec(2), func() { h.c.Arrive(&guest.Party{ID: 1, Size: 1}, guest.Standby) })
	h.sim.At(sec(20), func() { h.c.Resume(CauseWeatherHold) })
	h.sim.RunUntil(sec(60))
	if got := firstAt(h.sim.Log.Events(), engine.EvBoard, "", 0); got != sec(20) {
		t.Fatalf("boarded at %v, want 20 s (on resume)", got)
	}
}

func countEvents(evs []engine.Event, typ, info string) int {
	n := 0
	for _, e := range evs {
		if e.Type == typ && e.Info == info {
			n++
		}
	}
	return n
}

// Brief stops at 2/hr for 500 h: ~1,000 stops (Poisson, 4 sigma = 127). Long downtime with a 10 h
// MTBF: ~50 (4 sigma = 28). Failures off: none.
func TestFailureRates(t *testing.T) {
	cfg := fixture(t)
	cfg.Failures.BriefStop = &config.RateStop{RatePerHr: 2, DurationS: config.Dist{Mean: 60}}
	cfg.Failures.LongDowntime = &config.LongDowntime{MTBFHr: 10, MTTRMin: config.Dist{Mean: 30}}
	const hours = 500
	h := newHarness(t, cfg, Options{Trains: 1, FixedTimes: true, Failures: []string{config.FailBriefStop, config.FailLongDowntime}}, 0, hours*engine.Hour)
	h.sim.RunUntil(engine.Time(hours * engine.Hour))
	evs := h.sim.Log.Events()
	for _, c := range []struct {
		cause string
		want  float64
	}{{CauseBriefStop, 2 * hours}, {CauseLongDowntime, hours / 10.0}} {
		if n := float64(countEvents(evs, engine.EvRideStop, c.cause)); math.Abs(n-c.want) > 4*math.Sqrt(c.want) {
			t.Errorf("%s: %v stops in %d h, want ~%v", c.cause, n, hours, c.want)
		}
	}

	off := newHarness(t, cfg, Options{Trains: 1, FixedTimes: true}, 0, 50*engine.Hour)
	off.sim.RunUntil(engine.Time(50 * engine.Hour))
	if n := countEvents(off.sim.Log.Events(), engine.EvRideStop, CauseBriefStop); n != 0 {
		t.Fatalf("%d brief stops with failures off", n)
	}
}

func TestResumeWithoutStopPanics(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1}, 0, engine.Minute)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	h.c.Resume(CauseWeatherHold)
}

// Rise is down at opening on ~25% of days [fan]: with opening_delay enabled the ride starts closed with
// the given probability, for a drawn delay, then opens.
func TestOpeningDelay(t *testing.T) {
	cfg := *fixture(t)
	opens := func(p float64) (closedAtStart bool, stops int) {
		c := cfg
		c.Failures.OpeningDelay = &config.OpeningDelay{Probability: p, DurationS: config.Dist{Mean: 600}}
		h := newHarness(t, &c, Options{FixedTimes: true, Failures: []string{config.FailOpeningDelay}}, 0, engine.Hour)
		h.sim.RunUntil(engine.Time(engine.Second))
		closedAtStart = h.c.Closed()
		h.sim.RunUntil(engine.Time(engine.Hour))
		for _, e := range h.sim.Log.Events() {
			if e.Type == engine.EvRideStop && e.Info == CauseOpeningDelay {
				stops++
			}
		}
		if h.c.Closed() {
			t.Fatal("still closed after the delay")
		}
		return closedAtStart, stops
	}
	if closed, n := opens(1); !closed || n != 1 {
		t.Errorf("probability 1: closed at start %t, %d opening delays; want closed, 1", closed, n)
	}
	if closed, n := opens(0); closed || n != 0 {
		t.Errorf("probability 0: closed at start %t, %d opening delays; want open, 0", closed, n)
	}
}
