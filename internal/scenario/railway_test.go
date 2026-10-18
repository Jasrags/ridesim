package scenario

import (
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
)

func loadRailway(t *testing.T) *config.CoasterFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-runaway-railway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status == config.StatusFail {
			t.Fatalf("railway invalid: %s %s", r.CheckID, r.Detail)
		}
	}
	return f.Coaster
}

// Capacity target (Jason, 2026-10-18): 90% of 2,400 listed = 2,160/hr: one 32-seat train every
// 42.3 + 8 + 3 = 53.3 s. The lap 300 + 30 + 53.3 = 383 s fits 8 trains (8 x 53.3 = 426 s), and the two
// preshow rooms (2 x 32 / 95 s = 2,425/hr) never hold up the station.
func TestRailwayHitsTheCapacityTarget(t *testing.T) {
	_, m := compute(t, rnrRun(loadRailway(t), 8, 1, true, 6, saturated))
	if !within(m.Interval.Mean, 53.3, 1) || !within(m.RidersPerHr, 2160, 1.5) {
		t.Errorf("interval %.2f s, %.0f/hr; want 53.3 s, 2,160/hr", m.Interval.Mean, m.RidersPerHr)
	}
	if m.LoadIdleS > 0 {
		t.Errorf("load station idle %.0f s: 8 trains should keep it busy", m.LoadIdleS)
	}
	if m.Ops.Preshow.Shows == 0 {
		t.Error("preshow never ran")
	}
}

// Seven trains cannot cover the 383 s lap at 53.3 s, so the ride paces at the lap instead.
func TestRailwaySevenTrainsAreLapBound(t *testing.T) {
	_, m := compute(t, rnrRun(loadRailway(t), 7, 1, true, 6, saturated))
	if m.Interval.Mean < 54.5 {
		t.Errorf("7 trains: interval %.2f s; want lap-bound (~383 / 7 = 55 s)", m.Interval.Mean)
	}
}
