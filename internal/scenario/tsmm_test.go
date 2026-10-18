package scenario

import (
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/guest"
)

func loadTSMM(t *testing.T) *config.ContinuousFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-toy-story-mania.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status != config.StatusPass {
			t.Fatalf("toy story mania: %s %s %s", r.Status, r.CheckID, r.Detail)
		}
	}
	return f.Continuous
}

// Capacity (Jason, 2026-10-18): 90% of 2,400 = 2,160/hr: three tracks, an 8-seat tram every 40 s on each
// (one somewhere every 13.3 s). Solo guests fill every seat.
func TestTSMMHitsTheCapacityTarget(t *testing.T) {
	_, m := computeRun(t, smugglersRun(loadTSMM(t), 1, true, 6, guest.Curve{{RatePerHr: 4000}}, guest.Profile{}))
	if !within(m.RidersPerHr, 2160, 1) || !within(m.Interval.Mean, 40.0/3, 1) {
		t.Errorf("%.0f riders/hr, %.2f s between trams; want 2,160/hr, 13.3 s", m.RidersPerHr, m.Interval.Mean)
	}
}

// With the park's party mix parties share trams and callouts fill the gaps, so trams leave nearly full.
func TestTSMMSharedTramsWithPartyMix(t *testing.T) {
	ds := loadData(t)
	cfg := loadTSMM(t)
	prof, err := guestProfile(ds, cfg.Queues, &config.ScenarioGuests{ParkID: "wdw_dhs"})
	if err != nil {
		t.Fatal(err)
	}
	prof.PatienceMin = nil
	_, m := computeRun(t, smugglersRun(cfg, 1, true, 6, guest.Curve{{RatePerHr: 4000}}, prof))
	t.Logf("%.0f riders/hr, %.2f empty seats per tram", m.RidersPerHr, m.Ops.EmptySeats.Mean)
	if m.RidersPerHr < 2050 {
		t.Errorf("%.0f riders/hr with the park mix; want close to 2,160", m.RidersPerHr)
	}
}
