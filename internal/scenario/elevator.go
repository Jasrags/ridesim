package scenario

import (
	"fmt"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/elevator"
)

// ElevatorRun is one run of an elevator_drop ride.
type ElevatorRun struct {
	ID         string
	Ride       *config.ElevatorFile
	Seed       uint64
	FixedTimes bool
	Length     engine.Duration
	Warmup     engine.Duration
	Arrivals   guest.Curve
	LLArrivals guest.Curve
	Guests     guest.Profile
	Failures   []string
	Weather    []Window
	SRClosed   []Window
}

// WarmupDuration implements Runner.
func (r ElevatorRun) WarmupDuration() engine.Duration { return r.Warmup }

// Run executes the scenario and returns its event log.
func (r ElevatorRun) Run() (*engine.Log, error) {
	sim := engine.New(r.Seed)
	e, err := elevator.New(sim, r.Ride, elevator.Options{FixedTimes: r.FixedTimes, Failures: r.Failures})
	if err != nil {
		return nil, err
	}
	failures := "none"
	if len(r.Failures) > 0 {
		failures = strings.Join(r.Failures, ",")
	}
	id := r.ID
	if id == "" {
		id = "adhoc"
	}
	sim.Emit(engine.Event{Type: engine.EvRunStart, Info: fmt.Sprintf(
		"scenario=%s ride=%s archetype=elevator_drop seats=%d side_interval_s=%.1f seed=%d fixed_times=%t length_s=%.0f warmup_s=%.0f arrivals=%s failures=%s load=%s brakes=%s",
		id, r.Ride.Ride.ID, r.Ride.Vehicles.SeatsPerVehicle, r.Ride.SideInterval(), r.Seed, r.FixedTimes,
		r.Length.Seconds(), r.Warmup.Seconds(), curveString(r.Arrivals), failures, elevator.LoadBlock, "") +
		runInfoTailScenario(r.Ride.Preshow, r.Ride.Queues)})

	end := engine.Time(0).Add(r.Length)
	e.Start()
	guest.StartArrivalsWith(sim, r.Arrivals, r.Guests, e, end)
	if len(r.LLArrivals) > 0 {
		ll := guest.Profile{Lane: guest.LightningLane, TimeStream: engine.StreamLLArrivals, ChoiceStream: engine.StreamLLChoice,
			IDBase: 1 << 40, PartySizes: r.Guests.PartySizes}
		guest.StartArrivalsWith(sim, r.LLArrivals, ll, e, end)
	}
	for _, w := range r.SRClosed {
		sim.At(engine.Time(w.From), func() { e.SetSingleRiderOpen(false) })
		sim.At(engine.Time(w.To), func() { e.SetSingleRiderOpen(true) })
	}
	for _, w := range r.Weather {
		sim.At(engine.Time(w.From), func() { e.Stop(elevator.CauseWeatherHold) })
		sim.At(engine.Time(w.To), func() { e.Resume(elevator.CauseWeatherHold) })
	}
	sim.RunUntil(end)
	sim.Emit(engine.Event{Type: engine.EvRunEnd, N: e.WaitingGuests()})
	return sim.Log, nil
}

func runInfoTailScenario(ps *config.Preshow, q config.Queues) string {
	batch := 0
	if ps != nil && ps.Enabled {
		batch = ps.BatchSize
	}
	phase := "none"
	if q.SkipLine.Enabled && len(q.SkipLine.Merge.Phases) > 0 {
		phase = q.SkipLine.Merge.Phases[0].Name
	}
	return fmt.Sprintf(" preshow_batch=%d ll_phase=%s", batch, phase)
}

func elevatorFromFile(ds *config.Dataset, ride *config.ElevatorFile, f *config.ScenarioFile) (ElevatorRun, error) {
	shared, err := parseShared(ds, f, ride.Queues)
	if err != nil {
		return ElevatorRun{}, err
	}
	s := f.Scenario
	return ElevatorRun{ID: s.ID, Ride: ride, Seed: s.Seed, FixedTimes: s.FixedTimes,
		Length: engine.Seconds(s.DurationH * 3600), Warmup: engine.Seconds(s.WarmupMin * 60), Failures: s.Failures,
		Arrivals: shared.Arrivals, LLArrivals: shared.LLArrivals, Guests: shared.Guests, Weather: shared.Weather, SRClosed: shared.SRClosed}, nil
}
