package scenario

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func loadSmugglers(t *testing.T) *config.ContinuousFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-smugglers-run.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status == config.StatusFail {
			t.Fatalf("smugglers invalid: %s %s", r.CheckID, r.Detail)
		}
	}
	return f.Continuous
}

func smugglersRun(cfg *config.ContinuousFile, seed uint64, fixed bool, hours float64, arrivals guest.Curve, prof guest.Profile) ContinuousRun {
	return ContinuousRun{Ride: cfg, Seed: seed, FixedTimes: fixed, Length: engine.Seconds(hours * 3600), Warmup: engine.Hour,
		Arrivals: arrivals, Guests: prof}
}

// Solo guests fill every seat: 4 turntables x 6 seats x 3600 / 45 s = 1,920/hr, a cockpit leaving
// every 45 / 4 = 11.25 s, and both stages in front keep up (Hondo 2,160/hr, chess rooms 2,880/hr).
func TestSmugglersFullCockpitsHitTheSourcedRotation(t *testing.T) {
	_, m := computeRun(t, smugglersRun(loadSmugglers(t), 1, true, 6, guest.Curve{{RatePerHr: 4000}}, guest.Profile{}))
	if !within(m.RidersPerHr, 1920, 1) || !within(m.Interval.Mean, 11.25, 1) {
		t.Errorf("%.0f riders/hr, %.2f s between cockpits; want 1,920/hr, 11.25 s", m.RidersPerHr, m.Interval.Mean)
	}
}

// Crews [user]: parties fill the four front seats, single riders the engineer seats, then parties; the
// grouper calls a party that fits from up to 10 back. With the park's party mix cockpits leave nearly
// full and the ride lands near Disney's 1,800/hr (strict line order gave ~1,480).
func TestSmugglersSharedCrewsWithPartyMix(t *testing.T) {
	ds := loadData(t)
	cfg := loadSmugglers(t)
	prof, err := guestProfile(ds, cfg.Queues, &config.ScenarioGuests{ParkID: "wdw_dhs"})
	if err != nil {
		t.Fatal(err)
	}
	prof.PatienceMin = nil                                                                     // keep the line long: this measures crew fill, not demand
	_, m := computeRun(t, smugglersRun(cfg, 1, true, 6, guest.Curve{{RatePerHr: 3000}}, prof)) // above capacity: the line never empties
	t.Logf("%.0f riders/hr, %.2f empty seats per cockpit, lanes %v", m.RidersPerHr, m.Ops.EmptySeats.Mean, m.Lanes)
	if m.RidersPerHr < 1700 || m.RidersPerHr > 1920 {
		t.Errorf("%.0f riders/hr with the park mix; want ~1,800", m.RidersPerHr)
	}
	if m.Lanes["single_rider"] == 0 {
		t.Error("no single riders boarded")
	}
}

// Full guest flow with Lightning Lane and every failure kind, including one turntable out: same seed,
// same log, and every guest is accounted for.
func TestSmugglersFullFlowDeterministic(t *testing.T) {
	ds := loadData(t)
	cfg := loadSmugglers(t)
	prof, err := guestProfile(ds, cfg.Queues, &config.ScenarioGuests{ParkID: "wdw_dhs"})
	if err != nil {
		t.Fatal(err)
	}
	run := smugglersRun(cfg, 4, false, 12, guest.Curve{{Start: 0, RatePerHr: 1700}, {Start: 3 * engine.Hour, RatePerHr: 900}}, prof)
	run.LLArrivals = guest.Curve{{RatePerHr: 250}}
	run.Failures = []string{config.FailChainStop, config.FailLongDowntime, config.FailSideDown}
	a, m := computeRun(t, run)
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
	t.Logf("%.0f riders/hr, LL %.0f%%, wait %.1f min", m.RidersPerHr, m.LLShare*100, m.Wait.Mean/60)
	if joins != out+waiting {
		t.Errorf("joined %d, boarded or reneged %d, still waiting %d", joins, out, waiting)
	}
}
