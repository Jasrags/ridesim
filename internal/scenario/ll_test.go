package scenario

import (
	"bytes"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/queue"
)

func llRun(cfg *config.CoasterFile, trains int, llRate float64) CoasterRun {
	return CoasterRun{Ride: cfg, Trains: trains, Seed: 3, FixedTimes: true, Length: 8 * engine.Hour, Warmup: engine.Hour,
		Arrivals: guest.Curve{{RatePerHr: saturated}}, LLArrivals: guest.Curve{{RatePerHr: llRate}}}
}

// Below the merge's cap, Lightning Lane gets all its demand: share = 400 / 2,264 riders/hr (Everest
// fixed times, standby saturated) and the LL wait stays short.
func TestLLShareIsDemandOverThroughput(t *testing.T) {
	_, m := compute(t, llRun(loadEverest(t), 5, 400))
	if want := 400 / m.RidersPerHr; !within(m.LLShare, want, 3) {
		t.Errorf("LL share %.3f, want %.3f", m.LLShare, want)
	}
	if !within(m.RidersPerHr, 2264, 1) {
		t.Errorf("%.0f riders/hr, want the ride's 2,264 (merge must not cost capacity)", m.RidersPerHr)
	}
	if ll := m.LaneWait["lightning_lane"]; ll.Mean > 120 {
		t.Errorf("LL wait %.0f s, want under 2 min", ll.Mean)
	}
}

// LL demand far above capacity: the LL line passes 200 and the recovery phase admits 10 LL guests per
// standby guest, so LL takes 10/11 of riders.
func TestLLSaturatedHitsRecoveryCap(t *testing.T) {
	_, m := compute(t, llRun(loadEverest(t), 5, 4000))
	if !within(m.LLShare, 10.0/11, 1) {
		t.Errorf("LL share %.3f, want 10/11", m.LLShare)
	}
	if rec := m.PhaseS["recovery"]; rec < 0.95*m.WindowEnd.Sub(m.WindowStart).Seconds() {
		t.Errorf("recovery phase %.0f s of the window, want nearly all", rec)
	}
}

func flatMerge(cfg *config.CoasterFile) *config.CoasterFile {
	out := *cfg
	q := out.Queues
	m := q.SkipLine.Merge
	m.Phases = []config.MergePhase{{Name: "flat", LLPerStandby: 2}}
	q.SkipLine.Merge = m
	out.Queues = q
	return &out
}

// After a 30 min weather hold the LL line holds ~200 guests. The phased merge (up to 10:1) clears it
// faster than a flat 2:1 merge would.
func TestLLRecoveryAfterDowntime(t *testing.T) {
	clearTime := func(cfg *config.CoasterFile) engine.Duration {
		r := llRun(cfg, 5, 400)
		r.Weather = []Window{{From: 3 * engine.Hour, To: 3*engine.Hour + 30*engine.Minute}}
		log, err := r.Run()
		if err != nil {
			t.Fatal(err)
		}
		resume := engine.Time(3*engine.Hour + 30*engine.Minute)
		q := 0
		peak := 0
		for _, e := range log.Events() {
			if e.Info != "lightning_lane" {
				continue
			}
			switch e.Type {
			case engine.EvQueueJoin:
				q += e.N
			case engine.EvBoard:
				q -= e.N
			}
			peak = max(peak, q)
			if e.T > resume && q <= 10 {
				t.Logf("LL backlog peak %d, cleared %.0f s after resume", peak, e.T.Sub(resume).Seconds())
				return e.T.Sub(resume)
			}
		}
		t.Fatal("LL backlog never cleared")
		return 0
	}
	phased, flat := clearTime(loadEverest(t)), clearTime(flatMerge(loadEverest(t)))
	if phased >= flat {
		t.Fatalf("phased merge cleared the LL backlog in %v ms, flat 2:1 in %v ms; want phased faster", phased, flat)
	}
}

// Rock 'n' Roller merges in the preshow hallway: LL guests ride through the preshow and still get
// their demand share below the cap.
func TestLLMergesInRnRHallway(t *testing.T) {
	log, m := compute(t, llRun(loadRnR(t), 4, 300))
	if want := 300 / m.RidersPerHr; !within(m.LLShare, want, 4) {
		t.Errorf("LL share %.3f, want %.3f", m.LLShare, want)
	}
	if !within(m.RidersPerHr, 1800, 1) {
		t.Errorf("%.0f riders/hr, want 1,800", m.RidersPerHr)
	}
	shows := 0
	for _, e := range log.Events() {
		if e.Type == queue.EvPreshowShow {
			shows++
		}
	}
	if shows == 0 {
		t.Fatal("no preshow ran")
	}
}

func TestLLDeterministic(t *testing.T) {
	r := llRun(loadEverest(t), 5, 600)
	r.FixedTimes = false
	r.Failures = []string{config.FailBriefStop, config.FailCascadeStop}
	a, _ := compute(t, r)
	b, _ := compute(t, r)
	if !bytes.Equal(encode(t, a), encode(t, b)) {
		t.Fatal("same seed, different logs with Lightning Lane")
	}
}
