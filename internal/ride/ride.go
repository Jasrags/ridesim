// Package ride defines the interface every ride archetype implements (spec §3). Implementations live
// in subpackages: coaster (multi_train_coaster), continuous (continuous_load), elevator (elevator_drop)
// and bays (simulator_bays).
package ride

import (
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// Status is a ride's operating state.
type Status int

const (
	Open Status = iota + 1
	Down
	WeatherHold
)

func (s Status) String() string {
	switch s {
	case Open:
		return "open"
	case Down:
		return "down"
	case WeatherHold:
		return "weather_hold"
	}
	return "unknown"
}

// Ride is the archetype-independent view of a ride. PostedWait returns engine.Duration (int64 ms)
// rather than time.Duration so every simulation quantity uses one time type.
type Ride interface {
	ID() string
	Arrive(p *guest.Party, lane guest.Lane)
	PostedWait() engine.Duration // what guests see, not the true wait
	Status() Status
}
