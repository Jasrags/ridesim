package metrics

import (
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/engine"
)

func s(sec float64) engine.Time { return engine.Time(engine.Seconds(sec)) }

// A hand-built log over a 1000 s window with no warmup:
//   - weather 100-300 s and a brief stop 200-400 s overlap: union 300 s stopped, 200 s each by cause
//   - a cascade stop 900-1100 s runs past the run end at 1000 s: 100 s counted
//   - dispatches at 100, 160, 220 s with 34, 30, 34 riders of 34 seats
//   - guest 1 joins at 50 s when 120 s is posted and boards at 110 s (true 60 s: +60 s error)
//   - guest 2 joins at 60 s, boards at 170 s (true 110 s, posted 120 s: +10 s error)
//   - one brake hold of 8 s and one ride_stopped hold (not stacking)
func TestComputeHandBuiltLog(t *testing.T) {
	ev := []engine.Event{
		{T: 0, Type: engine.EvRunStart, Info: "seats=34 load=LOAD brakes=B1,B2"},
		{T: 0, Type: engine.EvPostedWait, N: 120, Info: "queue_len_over_throughput"},
		{T: s(50), Type: engine.EvQueueJoin, Guest: 1, N: 1},
		{T: s(60), Type: engine.EvQueueJoin, Guest: 2, N: 1},
		{T: s(100), Type: engine.EvDispatch, Block: "LOAD", Train: 1, N: 34},
		{T: s(100), Type: engine.EvRideStop, Info: "weather_hold"},
		{T: s(110), Type: engine.EvBoard, Guest: 1, Train: 2, N: 1},
		{T: s(120), Type: engine.EvHoldStart, Train: 3, Block: "B1", Info: "next_occupied"},
		{T: s(128), Type: engine.EvHoldEnd, Train: 3, Block: "B1"},
		{T: s(130), Type: engine.EvHoldStart, Train: 4, Block: "B2", Info: "ride_stopped"},
		{T: s(140), Type: engine.EvHoldEnd, Train: 4, Block: "B2"},
		{T: s(160), Type: engine.EvDispatch, Block: "LOAD", Train: 2, N: 30},
		{T: s(170), Type: engine.EvBoard, Guest: 2, Train: 3, N: 1},
		{T: s(200), Type: engine.EvRideStop, Info: "brief_stop"},
		{T: s(220), Type: engine.EvDispatch, Block: "LOAD", Train: 3, N: 34},
		{T: s(300), Type: engine.EvRideResume, Info: "weather_hold"},
		{T: s(400), Type: engine.EvRideResume, Info: "brief_stop"},
		{T: s(900), Type: engine.EvCascade, Train: 1, Block: "B1"},
		{T: s(900), Type: engine.EvRideStop, Info: "cascade_stop"},
		{T: s(1000), Type: engine.EvRunEnd},
	}
	r, err := Compute(ev, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"weather_hold": 200, "brief_stop": 200, "cascade_stop": 100}
	for _, d := range r.Ops.Downtime {
		if d.TotalS != want[d.Cause] || d.Count != 1 {
			t.Errorf("%s: %d stops %.0f s, want 1 and %.0f s", d.Cause, d.Count, d.TotalS, want[d.Cause])
		}
	}
	checks := []struct {
		name      string
		got, want float64
	}{
		{"stopped union", r.Ops.StoppedS, 400},
		{"availability", r.Ops.Availability, 0.6},
		{"cascades", float64(r.Ops.Cascades), 1},
		{"empty seats mean", r.Ops.EmptySeats.Mean, 4.0 / 3},
		{"stacking", r.Ops.StackingS, 8},
		{"posted |err|", r.Ops.Posted.MeanAbsS, 35},
		{"posted bias", r.Ops.Posted.BiasS, 35},
		{"riders", float64(r.Riders), 98},
		{"interval mean", r.Interval.Mean, 60},
		{"wait mean", r.Wait.Mean, 85},
		{"B1 hold", holdTotal(r, "B1"), 8},
	}
	for _, c := range checks {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func holdTotal(r *Report, block string) float64 {
	for _, h := range r.Holds {
		if h.Block == block {
			return h.TotalS
		}
	}
	return -1
}

func TestComputeRejectsMalformedLogs(t *testing.T) {
	for name, ev := range map[string][]engine.Event{
		"empty":      nil,
		"no start":   {{Type: engine.EvRunEnd}},
		"no end":     {{Type: engine.EvRunStart}},
		"short":      {{Type: engine.EvRunStart}, {T: s(10), Type: engine.EvRunEnd}},
		"orphan end": {{Type: engine.EvRunStart}, {T: s(5), Type: engine.EvHoldEnd, Train: 1}, {T: s(100), Type: engine.EvRunEnd}},
	} {
		warmup := engine.Duration(0)
		if name == "short" {
			warmup = 20 * engine.Second
		}
		if _, err := Compute(ev, warmup); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
