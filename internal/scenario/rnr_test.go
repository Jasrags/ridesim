package scenario

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/coaster"
)

func loadRnR(t *testing.T) *config.CoasterFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-rock-n-roller-coaster.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return f.Coaster
}

func rnrRun(cfg *config.CoasterFile, trains int, seed uint64, fixed bool, hours, rate float64) CoasterRun {
	return CoasterRun{Ride: cfg, Trains: trains, Seed: seed, FixedTimes: fixed, Length: engine.Seconds(hours * 3600),
		Warmup: engine.Hour, Arrivals: guest.Curve{{RatePerHr: rate}}}
}

// Spec §11 (retuned 2026-10-17): station 37 + 8 + 3 = 48 s -> 3600 / 48 x 24 = 1,800/hr with 4 trains.
// The lap is 82 ride + 36 unload + 48 load = 166 s < 4 x 48 = 192 s, so 4 trains keep the station busy.
func TestRnRFourTrainsHitSpecTargets(t *testing.T) {
	_, m := compute(t, rnrRun(loadRnR(t), 4, 1, true, 6, 4000))
	if !within(m.Interval.Mean, 48, 0.2) || !within(m.RidersPerHr, 1800, 1) {
		t.Errorf("interval %.2f s, %.0f/hr; want 48 s, 1,800/hr", m.Interval.Mean, m.RidersPerHr)
	}
	if m.LoadIdleS > 0 {
		t.Errorf("load station idle %.0f s: 4 trains should be sufficient", m.LoadIdleS)
	}
	// Preshow supplies 130 per (150 + 30 s) = 2,600/hr > 1,800: it is never the bottleneck, so its
	// release spends time blocked by the full load area.
	if m.Ops.Preshow.Shows == 0 || m.Ops.Preshow.BlockedS == 0 || m.Ops.Preshow.Partial != 0 {
		t.Errorf("preshow %+v: want full shows and release blocked by the station", m.Ops.Preshow)
	}
}

// As authored before the retune (load_s 40): 51 s, 1,694/hr.
func TestRnRPreTuneHandMath(t *testing.T) {
	cfg := loadRnR(t)
	cfg.Station.LoadS.Mean = 40
	_, m := compute(t, rnrRun(cfg, 4, 1, true, 6, 4000))
	if !within(m.Interval.Mean, 51, 0.2) || !within(m.RidersPerHr, 3600.0/51*24, 1) {
		t.Errorf("interval %.2f s, %.0f/hr; want 51 s, 1,694/hr", m.Interval.Mean, m.RidersPerHr)
	}
}

func TestRnRRejectsFifthTrain(t *testing.T) {
	if _, err := rnrRun(loadRnR(t), 5, 1, true, 1, 100).Run(); err == nil {
		t.Fatal("5 trains accepted; max_in_service is 4 [fan]")
	}
}

// A slow station (load_s +20 s) backs guests up into the preshow: release is blocked longer and the
// outdoor line grows, while throughput follows the station (60 + 8 + 3 = 68 s).
func TestRnRSlowStationCreatesBackPressure(t *testing.T) {
	_, base := compute(t, rnrRun(loadRnR(t), 4, 1, true, 6, 2000))
	slow := loadRnR(t)
	slow.Station.LoadS.Mean += 20
	_, m := compute(t, rnrRun(slow, 4, 1, true, 6, 2000))
	if !within(m.Interval.Mean, 68, 0.5) {
		t.Errorf("slow interval %.2f s, want 68", m.Interval.Mean)
	}
	if m.Ops.Preshow.BlockedS <= base.Ops.Preshow.BlockedS*1.5 {
		t.Errorf("blocked %.0f s vs %.0f s at normal pace: want clearly more back-pressure", m.Ops.Preshow.BlockedS, base.Ops.Preshow.BlockedS)
	}
	if m.QueueMax <= base.QueueMax*2 {
		t.Errorf("max queue %d vs %d: want the line to back up", m.QueueMax, base.QueueMax)
	}
}

// Park party mix with 5% single riders: rows cost some seats, single riders recover part of them.
func TestRnRPartiesAndSingleRiders(t *testing.T) {
	ds := loadData(t)
	prof, err := guestProfile(ds, loadRnR(t).Queues, &config.ScenarioGuests{ParkID: "synthetic_park_v0", Abandonment: new(bool)})
	if err != nil {
		t.Fatal(err)
	}
	run := rnrRun(loadRnR(t), 4, 2, true, 6, 4000)
	run.Guests = prof
	_, m := compute(t, run)
	noSR := run
	noSR.Guests.SingleRiderShare = 0
	_, m0 := compute(t, noSR)
	t.Logf("with single riders: %.0f/hr, %.2f empty seats/dispatch, lanes %v; without: %.0f/hr, %.2f empty", m.RidersPerHr, m.Ops.EmptySeats.Mean, m.Lanes, m0.RidersPerHr, m0.Ops.EmptySeats.Mean)
	if m.Lanes["single_rider"] == 0 {
		t.Fatal("no single riders boarded")
	}
	if m0.Ops.EmptySeats.Mean <= 0 {
		t.Fatal("party mix without single riders should leave empty seats")
	}
	if m.Ops.EmptySeats.Mean >= m0.Ops.EmptySeats.Mean {
		t.Errorf("single riders did not reduce empty seats: %.2f vs %.2f", m.Ops.EmptySeats.Mean, m0.Ops.EmptySeats.Mean)
	}
}

// Determinism and Little's Law (with reneging) for the full M3 guest flow under failures.
func TestRnRFullFlowDeterministicAndLittle(t *testing.T) {
	ds := loadData(t)
	prof, err := guestProfile(ds, loadRnR(t).Queues, &config.ScenarioGuests{ParkID: "synthetic_park_v0"})
	if err != nil {
		t.Fatal(err)
	}
	run := CoasterRun{Ride: loadRnR(t), Trains: 4, Seed: 7, Length: 12 * engine.Hour, Warmup: 0,
		Arrivals: guest.Curve{{Start: 0, RatePerHr: 2600}, {Start: 2 * engine.Hour, RatePerHr: 900}},
		Guests:   prof, Failures: []string{config.FailBriefStop}, CascadeScope: coaster.CascadeBrakesOnly}
	a, m := compute(t, run)
	b, _ := compute(t, run)
	if !bytes.Equal(encode(t, a), encode(t, b)) {
		t.Fatal("same seed, different logs")
	}
	t.Logf("balks %d reneges %d, wait mean %.1f min, L %.1f vs lambda*W %.1f (%.2f%%)", m.Balks, m.Reneges, m.Wait.Mean/60, m.Little.L, m.Little.LambdaW, m.Little.ErrPct)
	if m.Reneges+m.Balks == 0 {
		t.Error("surge produced no abandonment")
	}
	if m.Unserved > 50 || m.Little.ErrPct > 2 {
		t.Errorf("Little's Law: L %.1f vs lambda*W %.1f (%.1f%%), unserved %d", m.Little.L, m.Little.LambdaW, m.Little.ErrPct, m.Unserved)
	}
}
