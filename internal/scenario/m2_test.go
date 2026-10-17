package scenario

import (
	"bytes"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/metrics"
)

func loadData(t *testing.T) *config.Dataset {
	t.Helper()
	ds, err := config.LoadDir("../../data")
	if err != nil {
		t.Fatal(err)
	}
	if rep := config.Validate(ds); rep.HasErrors() {
		t.Fatal("data has validation errors")
	}
	return ds
}

func scenarioRun(t *testing.T, ds *config.Dataset, id string) CoasterRun {
	t.Helper()
	for _, f := range ds.Files {
		if f.Kind == config.KindScenario && f.Scenario.Scenario.ID == id {
			r, err := FromFile(ds, f.Scenario)
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
	}
	t.Fatalf("no scenario %s", id)
	return CoasterRun{}
}

func compute(t *testing.T, r CoasterRun) (*engine.Log, *metrics.Report) {
	t.Helper()
	log, err := r.Run()
	if err != nil {
		t.Fatal(err)
	}
	m, err := metrics.Compute(log.Events(), r.Warmup)
	if err != nil {
		t.Fatal(err)
	}
	return log, m
}

func TestScenarioFilesRunDeterministically(t *testing.T) {
	ds := loadData(t)
	for _, id := range []string{"everest_baseline_day", "everest_ops_demo"} {
		r := scenarioRun(t, ds, id)
		r.CascadeToleranceS = 45 // keep the run healthy whatever the file's tolerance is
		a, _ := compute(t, r)
		b, _ := compute(t, r)
		if !bytes.Equal(encode(t, a), encode(t, b)) {
			t.Errorf("%s: same seed, different logs with failures on", id)
		}
	}
}

// Fixed times, saturated, failures off: a 30 min weather hold in a 5 h window removes ~10% of riders.
func TestWeatherHoldCostsItsShareOfThroughput(t *testing.T) {
	base := CoasterRun{Ride: loadEverest(t), Trains: 5, Seed: 1, FixedTimes: true, Length: 6 * engine.Hour,
		Warmup: engine.Hour, Arrivals: guest.Curve{{RatePerHr: saturated}}}
	_, clear := compute(t, base)
	held := base
	held.Weather = []Window{{From: 3 * engine.Hour, To: 3*engine.Hour + 30*engine.Minute}}
	_, wet := compute(t, held)
	if got := wet.Ops.Availability; !within(got, 0.9, 0.1) {
		t.Errorf("availability %.4f, want 0.9", got)
	}
	if loss := 1 - wet.RidersPerHr/clear.RidersPerHr; !within(loss, 0.10, 8) {
		t.Errorf("throughput loss %.1f%%, want ~10%%", loss*100)
	}
}

// Ops demo: removing a train at 03:00 runs 4 trains (shadow-room hold ignored, no yields) until the
// train comes back at 05:00.
func TestOpsDemoSwapsTrainsAndTogglesShadowHold(t *testing.T) {
	r := scenarioRun(t, loadData(t), "everest_ops_demo")
	r.CascadeToleranceS = 45
	log, m := compute(t, r)
	if m.Ops.Swaps != 2 {
		t.Fatalf("%d barn swaps, want 2", m.Ops.Swaps)
	}
	var out, back engine.Time = -1, -1
	for _, e := range log.Events() {
		if e.Type == engine.EvTrainToBarn && e.N == 4 {
			out = e.T
		}
		if e.Type == engine.EvTrainFromBarn && e.N == 5 {
			back = e.T
		}
	}
	if out < engine.Time(3*engine.Hour) || back < engine.Time(5*engine.Hour) || back < out {
		t.Fatalf("train out at %v, back at %v", out, back)
	}
	for _, e := range log.Events() {
		if e.Type == engine.EvHoldStart && e.Info == "hold_until_clear" && e.T > out && e.T < back {
			t.Fatalf("hold_until_clear at %v with 4 trains", e.T)
		}
	}
	found := false
	for _, d := range m.Ops.Downtime {
		found = found || (d.Cause == "weather_hold" && within(d.TotalS, 1800, 0.1))
	}
	if !found {
		t.Errorf("weather hold downtime missing or not 30 min: %+v", m.Ops.Downtime)
	}
}

// Brief stops and long downtime alone (no cascades): availability tracks the hand math
// 1 - (0.3/hr x 150 s + 35 min / 40 h) / 3600 s = 97.3%, averaged over long runs.
func TestRandomFailureDerateMatchesHandMath(t *testing.T) {
	want := 1 - (0.3*150+35.0*60/40)/3600
	var avail float64
	const seeds = 4
	for seed := uint64(1); seed <= seeds; seed++ {
		_, m := compute(t, CoasterRun{Ride: loadEverest(t), Trains: 5, Seed: seed, FixedTimes: true,
			Length: 200 * engine.Hour, Warmup: engine.Hour, Arrivals: guest.Curve{{RatePerHr: 2300}},
			Failures: []string{config.FailBriefStop, config.FailLongDowntime}})
		avail += m.Ops.Availability / seeds
	}
	if !within(avail, want, 0.6) {
		t.Fatalf("availability %.4f, want %.4f", avail, want)
	}
}
