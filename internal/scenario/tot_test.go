package scenario

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func loadToT(t *testing.T) *config.ElevatorFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-tower-of-terror.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status != config.StatusPass {
			t.Fatalf("tower of terror: %s %s %s", r.Status, r.CheckID, r.Detail)
		}
	}
	return f.Elevator
}

func totRun(cfg *config.ElevatorFile, seed uint64, fixed bool, hours, rate float64) ElevatorRun {
	return ElevatorRun{Ride: cfg, Seed: seed, FixedTimes: fixed, Length: engine.Seconds(hours * 3600), Warmup: engine.Hour,
		Arrivals: guest.Curve{{RatePerHr: rate}}}
}

// The forum calculation [fan]: each side dispatches every 90 s with 4 cabs and 2 load shafts, 21 seats:
// 2 x 21 x 3600 / 90 = 1,680/hr. The [assumed] cycle makes the drop shaft (90 s) the bottleneck and the
// library preshow (2 rooms x 60 / 165 s = 2,618/hr) never holds it up.
func TestToTHitsTheForumCapacity(t *testing.T) {
	_, m := computeRun(t, totRun(loadToT(t), 1, true, 6, saturated))
	if !within(m.RidersPerHr, 1680, 1) || !within(m.Interval.Mean, 45, 1) {
		t.Errorf("%.0f riders/hr, %.1f s between dispatches; want 1,680/hr and 45 s (90 s per side)", m.RidersPerHr, m.Interval.Mean)
	}
	if m.Ops.Preshow.Shows == 0 || m.Ops.Preshow.Partial != 0 {
		t.Errorf("preshow %+v: want full shows only", m.Ops.Preshow)
	}
}

// With one side's cabs gone the other side alone carries 840/hr.
func TestToTOneSideCarriesHalf(t *testing.T) {
	cfg := *loadToT(t)
	cfg.Sides.Count, cfg.Sides.Names = 1, []string{"AB"}
	_, m := computeRun(t, totRun(&cfg, 1, true, 6, saturated))
	if !within(m.RidersPerHr, 840, 1) {
		t.Errorf("one side: %.0f riders/hr, want 840", m.RidersPerHr)
	}
}

// Full guest flow (park party mix, Lightning Lane, all failure kinds): same seed, same log.
func TestToTFullFlowDeterministic(t *testing.T) {
	ds := loadData(t)
	cfg := loadToT(t)
	prof, err := guestProfile(ds, cfg.Queues, &config.ScenarioGuests{ParkID: "wdw_dhs"})
	if err != nil {
		t.Fatal(err)
	}
	run := totRun(cfg, 3, false, 10, 1900)
	run.Guests, run.LLArrivals = prof, guest.Curve{{RatePerHr: 300}}
	run.Failures = []string{config.FailBriefStop, config.FailLongDowntime, config.FailSideDown}
	a, m := computeRun(t, run)
	b, _ := computeRun(t, run)
	if !bytes.Equal(encode(t, a), encode(t, b)) {
		t.Fatal("same seed, different logs")
	}
	t.Logf("%.0f riders/hr, LL share %.0f%%, wait %.1f min, balks %d reneges %d", m.RidersPerHr, m.LLShare*100, m.Wait.Mean/60, m.Balks, m.Reneges)
	if m.Riders == 0 || m.LLShare == 0 {
		t.Errorf("riders %d, LL share %.2f: want both lanes riding", m.Riders, m.LLShare)
	}
}
