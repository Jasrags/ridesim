package scenario

import (
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
)

func loadSlinky(t *testing.T) *config.CoasterFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-slinky-dog-dash.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status == config.StatusFail {
			t.Fatalf("slinky invalid: %s %s", r.CheckID, r.Detail)
		}
	}
	return f.Coaster
}

// Capacity target (Jason, 2026-10-18): ~90% of the 1,440/hr listed figure, i.e. one train every 50 s.
// Separate unload and load positions, so the station interval is load 39 + restraint 8 + clearance 3 = 50 s
// and 18 seats x 3600 / 50 = 1,296/hr. The lap is 120 ride + 24 unload + 50 load = 194 s < 4 x 50 = 200 s,
// so 4 trains keep the load position busy, and every clearance region (44, 34, 20 s) is shorter than 50 s.
func TestSlinkyFourTrainsHitCapacityTarget(t *testing.T) {
	_, m := compute(t, rnrRun(loadSlinky(t), 4, 1, true, 6, saturated))
	if !within(m.Interval.Mean, 50, 0.5) || !within(m.RidersPerHr, 1296, 1) {
		t.Errorf("interval %.2f s, %.0f/hr; want 50 s, 1,296/hr", m.Interval.Mean, m.RidersPerHr)
	}
	if m.LoadIdleS > 0 {
		t.Errorf("load position idle %.0f s: 4 trains should be sufficient", m.LoadIdleS)
	}
}

// Three trains: the lap (194 s) is longer than 3 x 50 s, so the ride paces at ~194 / 3 s per train.
func TestSlinkyThreeTrainsAreLapBound(t *testing.T) {
	_, m := compute(t, rnrRun(loadSlinky(t), 3, 1, true, 6, saturated))
	if m.Interval.Mean < 60 || m.RidersPerHr > 1100 {
		t.Errorf("3 trains: interval %.2f s, %.0f/hr; want lap-bound (> 60 s, < 1,100/hr)", m.Interval.Mean, m.RidersPerHr)
	}
}

func TestSlinkyRejectsFifthTrain(t *testing.T) {
	if _, err := rnrRun(loadSlinky(t), 5, 1, true, 1, 100).Run(); err == nil {
		t.Fatal("5 trains accepted; the fleet is 4 [wiki]")
	}
}
