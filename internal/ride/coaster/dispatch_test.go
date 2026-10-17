package coaster

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// No demand: the LOAD train finishes loading at 25 s and checking takes 7 s.
func TestDispatchRules(t *testing.T) {
	cases := []struct {
		rule string
		want engine.Time // first dispatch, -1 = none in 10 min
	}{
		{config.DispatchWhenReady, sec(32)},
		{config.DispatchHoldForFullOrMaxWait, sec(42)}, // + max_wait_after_ready_s 10
		{config.DispatchHoldForFull, -1},
	}
	for _, c := range cases {
		h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true, DispatchRule: c.rule}, 0, 10*engine.Minute)
		h.sim.RunUntil(sec(600))
		if got := firstAt(h.sim.Log.Events(), engine.EvDispatch, "", 0); got != c.want {
			t.Errorf("%s: first dispatch %v, want %v", c.rule, got, c.want)
		}
	}
}

// hold_for_full dispatches as soon as the last seat fills: 20 guests arriving 1 per second from t=30.
func TestHoldForFullDispatchesWhenFull(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true, DispatchRule: config.DispatchHoldForFull}, 0, 10*engine.Minute)
	for i := 0; i < 20; i++ {
		id := int64(i + 1)
		h.sim.At(sec(30+float64(i)), func() { h.c.Arrive(&guest.Party{ID: id, Size: 1}, guest.Standby) })
	}
	h.sim.RunUntil(sec(600))
	// 20th guest at 49 s, then restraint check 5 + clearance 2
	if got := firstAt(h.sim.Log.Events(), engine.EvDispatch, "", 0); got != sec(56) {
		t.Fatalf("dispatch at %v, want 56 s", got)
	}
}

func queueWhileStopped(h *harness, n int) {
	h.c.Stop(CauseWeatherHold)
	for i := 0; i < n; i++ {
		h.c.Arrive(&guest.Party{ID: int64(i + 1), Size: 1}, guest.Standby)
	}
}

// Before 15 minutes of history, throughput falls back to seats / target = 20 / 50 s = 0.4 guests/s.
func TestPostedWaitAlgorithms(t *testing.T) {
	pad := 20.0
	cases := []struct {
		opts PostedWaitOptions
		want engine.Duration
	}{
		{PostedWaitOptions{}, 250 * engine.Second}, // 100 / 0.4
		{PostedWaitOptions{Algorithm: config.PostedPadded, PaddingPct: &pad}, 300 * engine.Second},
		{PostedWaitOptions{Algorithm: config.PostedLastNObserved}, 250 * engine.Second}, // no history: falls back
	}
	for _, c := range cases {
		h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true, PostedWait: c.opts}, 0, engine.Hour)
		queueWhileStopped(h, 100)
		if got := h.c.PostedWait(); got != c.want {
			t.Errorf("%+v: posted %v ms, want %v", c.opts, got, c.want)
		}
	}
}

func TestLastNObservedAveragesRecentWaits(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true, PostedWait: PostedWaitOptions{Algorithm: config.PostedLastNObserved, LastN: 2}}, 0, engine.Hour)
	h.c.recordWait(10 * engine.Second)
	h.c.recordWait(20 * engine.Second)
	h.c.recordWait(40 * engine.Second) // only the last 2 count
	if got := h.c.PostedWait(); got != 30*engine.Second {
		t.Fatalf("posted %v ms, want 30 s", got)
	}
}

func TestPostedWaitIsLoggedEveryRefresh(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true, PostedWait: PostedWaitOptions{RefreshS: 60}}, 0, engine.Hour)
	h.sim.RunUntil(sec(600))
	n := 0
	for _, e := range h.sim.Log.Events() {
		if e.Type == engine.EvPostedWait {
			n++
		}
	}
	if n != 11 { // t = 0, 60, ..., 600
		t.Fatalf("%d posted_wait events, want 11", n)
	}
}
