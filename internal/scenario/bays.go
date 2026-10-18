package scenario

import (
	"fmt"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/bays"
)

// BaysRun is one run of a simulator_bays ride.
type BaysRun struct {
	ID         string
	Ride       *config.BaysFile
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
func (r BaysRun) WarmupDuration() engine.Duration { return r.Warmup }

// Run executes the scenario and returns its event log.
func (r BaysRun) Run() (*engine.Log, error) {
	sim := engine.New(r.Seed)
	e, err := bays.New(sim, r.Ride, bays.Options{FixedTimes: r.FixedTimes, Failures: r.Failures})
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
		"scenario=%s ride=%s archetype=simulator_bays seats=%d cycle_s=%.1f seed=%d fixed_times=%t length_s=%.0f warmup_s=%.0f arrivals=%s failures=%s load=%s brakes=%s",
		id, r.Ride.Ride.ID, r.Ride.Bays.SeatsPerBay, r.Ride.CycleS(), r.Seed, r.FixedTimes,
		r.Length.Seconds(), r.Warmup.Seconds(), curveString(r.Arrivals), failures, bays.LoadBlock, "") +
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
		sim.At(engine.Time(w.From), func() { e.Stop(bays.CauseWeatherHold) })
		sim.At(engine.Time(w.To), func() { e.Resume(bays.CauseWeatherHold) })
	}
	sim.RunUntil(end)
	sim.Emit(engine.Event{Type: engine.EvRunEnd, N: e.WaitingGuests()})
	return sim.Log, nil
}

func baysFromFile(ds *config.Dataset, ride *config.BaysFile, f *config.ScenarioFile) (BaysRun, error) {
	shared, err := parseShared(ds, f, ride.Queues)
	if err != nil {
		return BaysRun{}, err
	}
	s := f.Scenario
	return BaysRun{ID: s.ID, Ride: ride, Seed: s.Seed, FixedTimes: s.FixedTimes,
		Length: engine.Seconds(s.DurationH * 3600), Warmup: engine.Seconds(s.WarmupMin * 60), Failures: s.Failures,
		Arrivals: shared.Arrivals, LLArrivals: shared.LLArrivals, Guests: shared.Guests, Weather: shared.Weather, SRClosed: shared.SRClosed}, nil
}
