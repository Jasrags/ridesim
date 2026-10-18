package scenario

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func loadStarTours(t *testing.T) *config.BaysFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-star-tours.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status != config.StatusPass {
			t.Fatalf("star tours: %s %s %s", r.Status, r.CheckID, r.Detail)
		}
	}
	return f.Bays
}

func starToursRun(cfg *config.BaysFile, seed uint64, fixed bool, hours float64, arrivals guest.Curve, prof guest.Profile) BaysRun {
	return BaysRun{Ride: cfg, Seed: seed, FixedTimes: fixed, Length: engine.Seconds(hours * 3600), Warmup: engine.Hour, Arrivals: arrivals, Guests: prof}
}

// Capacity (Jason, 2026-10-18): 90% of 1,800 = 1,620/hr, a 533 s cycle per bay. Solo guests fill
// every seat, so 6 x 40 x 3600 / 533.3 = 1,620/hr.
func TestStarToursHitsTheCapacityTarget(t *testing.T) {
	_, m := computeRun(t, starToursRun(loadStarTours(t), 1, true, 6, guest.Curve{{RatePerHr: 4000}}, guest.Profile{}))
	if !within(m.RidersPerHr, 1620, 1) {
		t.Errorf("%.0f riders/hr, want 1,620", m.RidersPerHr)
	}
}

// With the park's party mix and rows assigned by cast (boarding groups + callouts), bays leave nearly
// full; and the full flow with every failure kind is deterministic and loses nobody.
func TestStarToursPartyMixAndFullFlow(t *testing.T) {
	ds := loadData(t)
	cfg := loadStarTours(t)
	prof, err := guestProfile(ds, cfg.Queues, &config.ScenarioGuests{ParkID: "wdw_dhs"})
	if err != nil {
		t.Fatal(err)
	}
	noPatience := prof
	noPatience.PatienceMin = nil
	_, m := computeRun(t, starToursRun(cfg, 1, true, 6, guest.Curve{{RatePerHr: 3000}}, noPatience))
	t.Logf("party mix: %.0f riders/hr, %.2f empty seats per flight", m.RidersPerHr, m.Ops.EmptySeats.Mean)
	if m.RidersPerHr < 1550 {
		t.Errorf("%.0f riders/hr with the park mix; want close to 1,620", m.RidersPerHr)
	}
	run := starToursRun(cfg, 3, false, 12, guest.Curve{{Start: 0, RatePerHr: 1500}, {Start: 3 * engine.Hour, RatePerHr: 700}}, prof)
	run.LLArrivals = guest.Curve{{RatePerHr: 200}}
	run.Failures = []string{config.FailBriefStop, config.FailLongDowntime, config.FailSideDown}
	a, _ := computeRun(t, run)
	b, _ := computeRun(t, run)
	if !bytes.Equal(encode(t, a), encode(t, b)) {
		t.Fatal("same seed, different logs")
	}
	joins, out, waiting := 0, 0, 0
	for _, e := range a.Events() {
		switch e.Type {
		case engine.EvQueueJoin:
			joins += e.N
		case engine.EvBoard, engine.EvRenege:
			out += e.N
		case engine.EvRunEnd:
			waiting = e.N
		}
	}
	if joins != out+waiting {
		t.Errorf("joined %d, boarded or reneged %d, still waiting %d", joins, out, waiting)
	}
}
