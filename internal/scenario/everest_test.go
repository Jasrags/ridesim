package scenario

import (
	"bytes"
	"math"
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/metrics"
)

const everestPath = "../../data/rides/wdw-ak-expedition-everest.yaml"

// saturated keeps the queue non-empty so every train leaves full and the ride, not demand, sets the pace.
const saturated = 6000.0

func loadEverest(t *testing.T) *config.CoasterFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean(everestPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status == config.StatusFail {
			t.Fatalf("everest invalid: %s %s", r.CheckID, r.Detail)
		}
	}
	return f.Coaster
}

// proposedTuning applies the [assumed]-only retune proposed in docs/m1-report.md, in memory. The YAML
// is untouched until Jason signs off. Sourced values (PEAK_BRAKE 6 s, REVERSE 20 s [fan]) are kept.
func proposedTuning(c *config.CoasterFile) *config.CoasterFile {
	out := *c
	out.Blocks = append([]config.Block(nil), c.Blocks...)
	transit := map[string]float64{"LIFT": 40, "SHADOW_BRAKE": 20, "MOUNTAIN_EXIT": 32, "HELIX_TO_BRAKES": 20, "YETI_BRAKE": 6, "PRE_UNLOAD_HOLD": 26}
	for i, b := range out.Blocks {
		if m, ok := transit[b.ID]; ok {
			d := *b.TransitS
			d.Mean = m
			out.Blocks[i].TransitS = &d
		}
	}
	out.Station.LoadS.Mean = 43
	return &out
}

func run(t *testing.T, cfg *config.CoasterFile, trains int, seed uint64, fixed bool, hours, rate float64) (*engine.Log, *metrics.Report) {
	t.Helper()
	log, err := CoasterRun{
		Ride: cfg, Trains: trains, Seed: seed, FixedTimes: fixed,
		Length: engine.Seconds(hours * 3600), Arrivals: guest.Curve{{Start: 0, RatePerHr: rate}},
	}.Run()
	if err != nil {
		t.Fatal(err)
	}
	m, err := metrics.Compute(log.Events(), engine.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return log, m
}

func holds(m *metrics.Report, block string) metrics.BlockHolds {
	for _, h := range m.Holds {
		if h.Block == block {
			return h
		}
	}
	return metrics.BlockHolds{Block: block}
}

func within(got, want, tolPct float64) bool { return math.Abs(got-want) <= want*tolPct/100 }

// Hand math for the data as authored, with fixed times and clearance rules applied literally.
// dispatch_out_of_mountain makes MOUNTAIN_EXIT + HELIX_TO_BRAKES one 45 + 24 = 69 s region gated at
// SHADOW_BRAKE, where the next train is always waiting, so it also pays the 2 s brake release:
// headway 71 s -> 3600 / 71 x 34 = 1,724 riders/hr. This misses the spec target of 54 s / 2,267/hr;
// see docs/m1-report.md. Four trains give the same headway because 4 x 71 = 284 s > the 263 s lap.
func TestEverestAsAuthoredMatchesClearanceHandMath(t *testing.T) {
	const wantInterval = 45 + 24 + 2.0
	wantPerHr := 3600 / wantInterval * 34
	for _, trains := range []int{5, 4} {
		_, m := run(t, loadEverest(t), trains, 1, true, 6, saturated)
		if !within(m.Interval.Mean, wantInterval, 0.2) || m.Interval.Max-m.Interval.Min > 0.01 {
			t.Errorf("%d trains: interval %+v, want constant %.0f s", trains, m.Interval, wantInterval)
		}
		if !within(m.RidersPerHr, wantPerHr, 1) {
			t.Errorf("%d trains: %.0f riders/hr, want %.0f", trains, m.RidersPerHr, wantPerHr)
		}
		shadow := holds(m, "SHADOW_BRAKE")
		if shadow.Holds == 0 {
			t.Errorf("%d trains: expected holds at the SHADOW_BRAKE bottleneck", trains)
		}
		if trains == 5 && shadow.Yields == 0 {
			t.Errorf("5 trains: expected hold_until_clear yields (circular wait with literal clearance)")
		}
		if trains == 4 && shadow.Yields != 0 {
			t.Errorf("4 trains: hold_until_clear must be ignored, got %d yields", shadow.Yields)
		}
	}
}

// With the proposed [assumed] retune no clearance region is longer than the 54 s station cycle
// (load 43 + check 8 + clearance 3), so the station sets the pace.
func TestEverestProposedTuningHitsSpecTargets(t *testing.T) {
	_, m := run(t, proposedTuning(loadEverest(t)), 5, 1, true, 6, saturated)
	if !within(m.Interval.Mean, 54, 0.2) {
		t.Errorf("interval %.2f s, want 54", m.Interval.Mean)
	}
	if !within(m.RidersPerHr, 3600.0/54*34, 1) {
		t.Errorf("%.0f riders/hr, want ~2,267", m.RidersPerHr)
	}
	if holds(m, "SHADOW_BRAKE").Holds == 0 {
		t.Error("expected brake holds at the shadow room with 5 trains")
	}
	if m.LoadIdleS > 0 {
		t.Errorf("load station idle %.0f s with 5 trains, want 0 (station-bound)", m.LoadIdleS)
	}
}

// Four trains leave the slot where the fifth would be: three dispatches at the 54 s station pace,
// then a long gap, averaging lap / 4 = (170 ride + 38 unload + 54 load) / 4 = 65.5 s.
func TestEverestProposedTuningFourTrainsShowGap(t *testing.T) {
	_, m := run(t, proposedTuning(loadEverest(t)), 4, 1, true, 6, saturated)
	if !within(m.Interval.Mean, 262.0/4, 1) {
		t.Errorf("mean interval %.2f s, want ~65.5", m.Interval.Mean)
	}
	if m.Interval.P50 > 54.5 || m.Interval.Max < 54+40 {
		t.Errorf("interval %+v, want mostly 54 s with a ~100 s gap", m.Interval)
	}
	if m.LoadIdleS == 0 {
		t.Error("load station never idle with 4 trains, want a gap")
	}
}

func encode(t *testing.T, log *engine.Log) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := log.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSameSeedGivesByteIdenticalLog(t *testing.T) {
	cfg := loadEverest(t)
	a, _ := run(t, cfg, 5, 7, false, 3, 1800)
	b, _ := run(t, cfg, 5, 7, false, 3, 1800)
	c, _ := run(t, cfg, 5, 8, false, 3, 1800)
	ea, eb, ec := encode(t, a), encode(t, b), encode(t, c)
	if !bytes.Equal(ea, eb) {
		t.Fatal("same seed produced different event logs")
	}
	if bytes.Equal(ea, ec) {
		t.Fatal("different seeds produced identical logs; RNG streams are not in use")
	}
	if len(ea) < 100_000 {
		t.Fatalf("log suspiciously small: %d bytes", len(ea))
	}
}

func TestReplayedLogGivesSameMetrics(t *testing.T) {
	log, live := run(t, loadEverest(t), 5, 3, false, 3, 1500)
	events, err := engine.ReadLog(bytes.NewReader(encode(t, log)))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := metrics.Compute(events, engine.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	_ = live.Write(&a)
	_ = replayed.Write(&b)
	if a.String() != b.String() {
		t.Fatalf("replayed metrics differ:\n%s\nvs\n%s", a.String(), b.String())
	}
}

// Little's Law at steady state: Poisson arrivals at 97% of the 1,724/hr capacity (data as authored),
// stochastic times, 24 h measured after a 1 h warmup.
func TestLittlesLawSteadyState(t *testing.T) {
	_, m := run(t, loadEverest(t), 5, 11, false, 25, 1680)
	if m.Wait.N < 30000 || m.Little.L < 5 {
		t.Fatalf("too little queueing to test: n=%d L=%.1f", m.Wait.N, m.Little.L)
	}
	if m.Little.ErrPct > 5 {
		t.Fatalf("L = %.2f vs lambda*W = %.2f (%.1f%% apart), want within 5%%", m.Little.L, m.Little.LambdaW, m.Little.ErrPct)
	}
	t.Logf("L %.2f vs lambda*W %.2f (%.2f%% apart); wait mean %.1f s", m.Little.L, m.Little.LambdaW, m.Little.ErrPct, m.Wait.Mean)
}

// Little's Law over a rope-drop surge: 2,600/hr for 2 h then 1,000/hr. The queue builds to ~1,700
// guests and drains, so measured from t=0 to a drained end the sample-path identity L = lambda W holds.
func TestLittlesLawSurgeAndDrain(t *testing.T) {
	log, err := CoasterRun{
		Ride: loadEverest(t), Trains: 5, Seed: 5, Length: 8 * engine.Hour,
		Arrivals: guest.Curve{{Start: 0, RatePerHr: 2600}, {Start: 2 * engine.Hour, RatePerHr: 1000}},
	}.Run()
	if err != nil {
		t.Fatal(err)
	}
	m, err := metrics.Compute(log.Events(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.Unserved > 50 || m.QueueMax < 1000 || m.Wait.Mean < 600 {
		t.Fatalf("scenario did not surge and drain: max queue %d, unserved %d, wait %.0f s", m.QueueMax, m.Unserved, m.Wait.Mean)
	}
	if m.Little.ErrPct > 2 {
		t.Fatalf("L = %.1f vs lambda*W = %.1f (%.1f%% apart), want within 2%%", m.Little.L, m.Little.LambdaW, m.Little.ErrPct)
	}
	t.Logf("max queue %d, wait mean %.1f min p95 %.1f; L %.1f vs lambda*W %.1f (%.2f%%)", m.QueueMax, m.Wait.Mean/60, m.Wait.P95/60, m.Little.L, m.Little.LambdaW, m.Little.ErrPct)
}
