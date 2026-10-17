package coaster

import (
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

const (
	defaultLastN        = 100
	defaultRefreshS     = 300 // matches the park's posted_wait_refresh_s
	throughputWindow    = 15 * engine.Minute
	maxRecentDispatches = 256
)

// PostedWaitOptions override the ride file's queues.posted_wait. Zero values keep the file/defaults.
type PostedWaitOptions struct {
	Algorithm  string
	PaddingPct *float64
	LastN      int
	RefreshS   float64
}

type dispatchRecord struct {
	at     engine.Time
	riders int
}

func (c *Coaster) postedAlgorithm() string {
	if a := c.opts.PostedWait.Algorithm; a != "" {
		return a
	}
	if a := c.cfg.Queues.PostedWait.Algorithm; a != "" {
		return a
	}
	return config.PostedQueueLenOverThroughput
}

func (c *Coaster) paddingPct() float64 {
	if p := c.opts.PostedWait.PaddingPct; p != nil {
		return *p
	}
	return c.cfg.Queues.PostedWait.PaddingPct
}

// PostedWait implements ride.Ride: what the sign says, as opposed to the wait guests will actually get.
func (c *Coaster) PostedWait() engine.Duration {
	switch c.postedAlgorithm() {
	case config.PostedLastNObserved:
		if len(c.recentWaits) > 0 {
			var sum engine.Duration
			for _, w := range c.recentWaits {
				sum += w
			}
			return sum / engine.Duration(len(c.recentWaits))
		}
		return c.queueLenOverThroughput()
	case config.PostedPadded:
		return engine.Seconds(c.queueLenOverThroughput().Seconds() * (1 + c.paddingPct()/100))
	default:
		return c.queueLenOverThroughput()
	}
}

// queueLenOverThroughput is the standby posted wait: standby-side guests waiting (everyone but Lightning
// Lane) divided by standby-side riders per second over the last 15 minutes (decision 2026-10-17: the
// merge gives standby only part of capacity). It falls back to the theoretical rate (seats x 3600 /
// target interval) before 15 minutes of history or when nothing boarded recently (downtime).
func (c *Coaster) queueLenOverThroughput() engine.Duration {
	riders := c.lines.StandbyBoardedSince(c.sim.Now().Add(-throughputWindow))
	perS := float64(riders) / throughputWindow.Seconds()
	if riders == 0 || c.sim.Now() < engine.Time(throughputWindow) {
		perS = float64(c.cfg.Trains.SeatsPerTrain) / c.cfg.Station.TargetDispatchIntervalS
	}
	return engine.Seconds(float64(c.lines.StandbyWaiting()) / perS)
}

func (c *Coaster) recordDispatch(riders int) {
	c.recentDispatch = append(c.recentDispatch, dispatchRecord{c.sim.Now(), riders})
	if len(c.recentDispatch) > maxRecentDispatches {
		c.recentDispatch = append([]dispatchRecord(nil), c.recentDispatch[len(c.recentDispatch)/2:]...)
	}
}

func (c *Coaster) recordWait(w engine.Duration) {
	n := c.opts.PostedWait.LastN
	if n <= 0 {
		n = defaultLastN
	}
	c.recentWaits = append(c.recentWaits, w)
	if len(c.recentWaits) > n {
		c.recentWaits = append([]engine.Duration(nil), c.recentWaits[len(c.recentWaits)-n:]...)
	}
}

// startPosting emits the posted wait now and every refresh interval, so metrics can compare what each
// guest was shown at join time with the wait they got.
func (c *Coaster) startPosting() {
	every := c.opts.PostedWait.RefreshS
	if every <= 0 {
		every = defaultRefreshS
	}
	var post func()
	post = func() {
		c.sim.Emit(engine.Event{Type: engine.EvPostedWait, N: int(c.PostedWait() / engine.Second), Info: c.postedAlgorithm()})
		c.sim.After(engine.Seconds(every), post)
	}
	post()
}
