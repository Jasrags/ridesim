// Package scenario wires a ride config, arrivals and the engine into one run. Scenario files (M6) will
// populate these structs; for now the CLI and tests build them directly.
package scenario

import (
	"fmt"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/coaster"
)

// CoasterRun is one standby-only run of a multi_train_coaster.
type CoasterRun struct {
	Ride       *config.CoasterFile
	Trains     int // 0 = default_in_service
	Seed       uint64
	FixedTimes bool
	Length     engine.Duration
	Arrivals   guest.Curve
}

// Run executes the scenario and returns its event log.
func (r CoasterRun) Run() (*engine.Log, error) {
	sim := engine.New(r.Seed)
	c, err := coaster.New(sim, r.Ride, coaster.Options{Trains: r.Trains, FixedTimes: r.FixedTimes})
	if err != nil {
		return nil, err
	}
	trains := r.Trains
	if trains == 0 {
		trains = r.Ride.Trains.DefaultInService
	}
	var brakes []string
	for _, b := range r.Ride.Blocks {
		if b.Kind == config.BlockBrake {
			brakes = append(brakes, b.ID)
		}
	}
	sim.Emit(engine.Event{Type: engine.EvRunStart, Info: fmt.Sprintf(
		"ride=%s trains=%d seats=%d seed=%d fixed_times=%t length_s=%.0f arrivals=%s load=%s brakes=%s",
		r.Ride.Ride.ID, trains, r.Ride.Trains.SeatsPerTrain, r.Seed, r.FixedTimes, r.Length.Seconds(),
		curveString(r.Arrivals), c.LoadBlock(), strings.Join(brakes, ","))})

	end := engine.Time(0).Add(r.Length)
	c.Start()
	guest.StartArrivals(sim, r.Arrivals, c, end)
	sim.RunUntil(end)
	sim.Emit(engine.Event{Type: engine.EvRunEnd, N: c.QueueGuests()})
	return sim.Log, nil
}

func curveString(c guest.Curve) string {
	parts := make([]string, len(c))
	for i, s := range c {
		parts[i] = fmt.Sprintf("%g:%g", s.Start.Seconds()/60, s.RatePerHr)
	}
	return strings.Join(parts, ",")
}
