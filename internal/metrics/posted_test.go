package metrics

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

func at(min float64) engine.Time { return engine.Time(engine.Seconds(min * 60)) }

// Posted waits are skipped while the ride is closed for long downtime and for 15 min after it reopens.
func TestPostedStatsSkipLongDowntime(t *testing.T) {
	evs := []engine.Event{
		{T: at(0), Type: engine.EvPostedWait, N: 600},                         // 10 min
		{T: at(30), Type: engine.EvRideStop, Info: config.FailBriefStop},      // brief stops do not close
		{T: at(60), Type: engine.EvRideStop, Info: config.FailLongDowntime},   // closed 60-90
		{T: at(70), Type: engine.EvPostedWait, N: 6000},                       // 100 min: skipped
		{T: at(90), Type: engine.EvRideResume, Info: config.FailLongDowntime}, // + 15 min recovery
		{T: at(100), Type: engine.EvPostedWait, N: 3000},                      // 50 min: skipped (recovery)
		{T: at(105), Type: engine.EvPostedWait, N: 1200},                      // 20 min
	}
	mean, peak := postedStats(evs, at(0), at(120))
	// open: 0-60 at 10 min, 105-120 at 20 min -> (60*10 + 15*20) / 75 = 12
	if mean < 11.99 || mean > 12.01 {
		t.Fatalf("mean %.2f, want 12", mean)
	}
	if peak < 19.99 || peak > 20.01 { // second hour: only 105-120 is open, at 20 min
		t.Fatalf("peak %.2f, want 20", peak)
	}
}
