package scenario

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func loadRise(t *testing.T) *config.CoasterFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-dhs-rise-of-the-resistance.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range config.CheckFile(f) {
		if r.Status == config.StatusFail {
			t.Fatalf("rise invalid: %s %s", r.CheckID, r.Detail)
		}
	}
	return f.Coaster
}

// stageShows counts preshow_show events per stage ("" is the briefing room) in the window after warmup.
func stageShows(log *engine.Log) map[string]int {
	n := map[string]int{}
	for _, e := range log.Events() {
		if e.Type == "preshow_show" && e.T >= engine.Time(engine.Hour) {
			n[e.Block]++
		}
	}
	return n
}

// Capacity target (Jason, 2026-10-18): TouringPlans' counted ~1,540/hr. The station gives one 16-seat pair
// every 26.4 + 8 + 3 = 37.4 s; every stage in front has spare throughput (briefing 2,743, ship 2,400,
// cells 2,304/hr), so with fixed times the station sets the pace and every stage keeps running.
func TestRiseHitsTheCountedCapacity(t *testing.T) {
	log, m := compute(t, rnrRun(loadRise(t), 12, 1, true, 6, saturated))
	if !within(m.Interval.Mean, 37.4, 1) || !within(m.RidersPerHr, 1540, 1.5) {
		t.Errorf("interval %.2f s, %.0f/hr; want 37.4 s, ~1,540/hr", m.Interval.Mean, m.RidersPerHr)
	}
	shows := stageShows(log)
	for _, id := range []string{"", "transport_ship", "holding_cell"} {
		if shows[id] == 0 {
			t.Errorf("stage %q never showed: %v", id, shows)
		}
	}
	// One cell group fills one pair: about as many cell shows as dispatches.
	if d := float64(shows["holding_cell"]) / float64(m.Dispatches); d < 0.95 || d > 1.05 {
		t.Errorf("%d cell shows for %d dispatches", shows["holding_cell"], m.Dispatches)
	}
}

// Back-pressure runs through the chain: a slow transport ship (3 x 40 / (600 + 30) s = 686/hr) caps the
// ride below the station's pace, and the briefing room ahead of it stalls.
func TestRiseSlowShipCapsTheRide(t *testing.T) {
	cfg := *loadRise(t)
	cfg.Stages = append([]config.Preshow(nil), cfg.Stages...)
	cfg.Stages[0].DurationS.Mean = 600
	_, m := compute(t, rnrRun(&cfg, 12, 1, true, 8, saturated))
	if m.RidersPerHr > 3*40*3600/630.0*1.02 || m.RidersPerHr < 600 {
		t.Errorf("%.0f riders/hr with a 10-minute ship; want the ship's ~686/hr", m.RidersPerHr)
	}
	if m.Ops.Preshow.BlockedS == 0 {
		t.Error("briefing rooms never blocked behind the slow ship")
	}
}

// Downtime fit (Jason, 2026-10-18): ~11% of the day [fan TouringPlans], counting long and brief stops and
// late openings. Over 20 twelve-hour days the ride is stopped 9-14% of the time.
func TestRiseDowntimeNearElevenPercent(t *testing.T) {
	var stopped, window float64
	for seed := uint64(1); seed <= 20; seed++ {
		run := rnrRun(loadRise(t), 12, seed, false, 12, 900)
		run.Failures = []string{config.FailBriefStop, config.FailLongDowntime, config.FailOpeningDelay}
		run.Warmup = 0 // TouringPlans' share covers the whole day, opening included
		_, m := compute(t, run)
		stopped += m.Ops.StoppedS
		window += m.WindowEnd.Sub(m.WindowStart).Seconds()
	}
	if share := stopped / window; share < 0.09 || share > 0.14 {
		t.Errorf("stopped %.1f%% of the time; want ~11%%", share*100)
	}
}

// Full guest flow with the park mix, Lightning Lane and failures: same seed, same log; every guest who
// joined boarded, reneged or is still in the chain; Little's Law holds on a day whose surge drains.
func TestRiseFullFlowDeterministic(t *testing.T) {
	ds := loadData(t)
	cfg := loadRise(t)
	prof, err := guestProfile(ds, cfg.Queues, &config.ScenarioGuests{ParkID: "wdw_dhs"})
	if err != nil {
		t.Fatal(err)
	}
	run := rnrRun(cfg, 12, 5, false, 12, 0)
	run.Arrivals = guest.Curve{{Start: 0, RatePerHr: 1900}, {Start: 3 * engine.Hour, RatePerHr: 800}}
	run.Warmup = 0 // Little's Law over the whole day: nobody is in line when the window opens
	run.Guests, run.LLArrivals = prof, guest.Curve{{RatePerHr: 200}}
	run.Failures = []string{config.FailBriefStop, config.FailLongDowntime}
	a, m := compute(t, run)
	b, _ := compute(t, run)
	if !bytes.Equal(encode(t, a), encode(t, b)) {
		t.Fatal("same seed, different logs")
	}
	t.Logf("%.0f riders/hr, LL %.0f%%, wait %.1f min, availability %.1f%%, Little err %.2f%%", m.RidersPerHr, m.LLShare*100, m.Wait.Mean/60, m.Ops.Availability*100, m.Little.ErrPct)
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
		t.Errorf("joined %d, boarded or reneged %d, still waiting %d: guests lost in the stage chain", joins, out, waiting)
	}
	if m.Little.ErrPct > 2 {
		t.Errorf("Little's Law off by %.1f%%", m.Little.ErrPct)
	}
}
