package park

import (
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// TravelTime is how long a party takes to cross an edge. Both walking modes share it (spec §9).
type TravelTime interface {
	// Enter puts n guests on e and returns their crossing time, or ok=false when e is at jam density.
	Enter(e *Edge, n int) (d engine.Duration, ok bool)
	// Leave takes n guests off e.
	Leave(e *Edge, n int)
	// Estimate is the crossing time a party would get now, counting guests queued to enter; for routing.
	Estimate(e *Edge) engine.Duration
}

// NewTravelTime returns the configured mode.
func NewTravelTime(p *config.ParkFile) TravelTime {
	if p.Park.WalkingMode == config.WalkingFixedDelay {
		return fixedDelay{p.Congestion.FreeSpeedFps}
	}
	return congestion{p.Congestion}
}

// fixedDelay is free-flow time on every edge; guests are still counted for metrics.
type fixedDelay struct{ speed float64 }

func (f fixedDelay) Enter(e *Edge, n int) (engine.Duration, bool) {
	e.guests += n
	return engine.Seconds(e.LengthFt / f.speed), true
}

func (f fixedDelay) Leave(e *Edge, n int) { e.guests -= n }

func (f fixedDelay) Estimate(e *Edge) engine.Duration { return engine.Seconds(e.LengthFt / f.speed) }

// congestion is BPR: t = t0 * (1 + alpha * (n / C)^beta) with C = length x width x comfortable density,
// n counting the entering party. A ride queue past its physical capacity takes walkway width
// (spillback). No one enters an edge past jam density, except an empty edge always admits one party so
// a queue covering the whole path cannot deadlock the park.
type congestion struct{ c config.Congestion }

// minWidthShare is the walkway width spillback can never take (decision: keep a passable lane).
const minWidthShare = 0.1

// Width is the edge width left after spillback, in ft.
func (c congestion) Width(e *Edge) float64 {
	w := e.WidthFt
	if e.spill != nil && c.c.Spillback.Enabled {
		w -= float64(e.spill()) / 100 * c.c.Spillback.WidthConsumedFtPer100Guests
	}
	return math.Max(w, e.WidthFt*minWidthShare)
}

// area is the edge's walking area for capacity and jam: its width times its length, or the configured
// minimum length, so how finely a path is split does not change how many guests it carries.
func (c congestion) area(e *Edge) float64 {
	return math.Max(e.LengthFt, c.c.MinCapacityLengthFt) * c.Width(e)
}

func (c congestion) Enter(e *Edge, n int) (engine.Duration, bool) {
	area := c.area(e)
	if e.guests > 0 && float64(e.guests+n) > area*c.c.JamDensityPerSqft {
		return 0, false
	}
	e.guests += n
	return c.time(e, e.guests), true
}

func (c congestion) time(e *Edge, guests int) engine.Duration {
	t0 := e.LengthFt / c.c.FreeSpeedFps
	ratio := float64(guests) / (c.area(e) * c.c.ComfortableDensityPerSqft)
	return engine.Seconds(t0 * (1 + c.c.Alpha*math.Pow(ratio, c.c.Beta)))
}

func (c congestion) Estimate(e *Edge) engine.Duration {
	waiting := 0
	for _, w := range e.wait {
		waiting += w.a.size
	}
	return c.time(e, e.guests+waiting)
}

func (c congestion) Leave(e *Edge, n int) { e.guests -= n }
