package scenario

import (
	"fmt"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/continuous"
)

// Runner is any runnable scenario.
type Runner interface {
	Run() (*engine.Log, error)
	WarmupDuration() engine.Duration
}

// WarmupDuration implements Runner.
func (r CoasterRun) WarmupDuration() engine.Duration { return r.Warmup }

// ContinuousRun is one run of a continuous_load ride.
type ContinuousRun struct {
	ID         string
	Ride       *config.ContinuousFile
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

	NoAccessibilityHolds bool
}

// WarmupDuration implements Runner.
func (r ContinuousRun) WarmupDuration() engine.Duration { return r.Warmup }

// Run executes the scenario and returns its event log.
func (r ContinuousRun) Run() (*engine.Log, error) {
	sim := engine.New(r.Seed)
	c, err := continuous.New(sim, r.Ride, continuous.Options{FixedTimes: r.FixedTimes, Failures: r.Failures, NoAccessibilityHolds: r.NoAccessibilityHolds})
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
	batch := 0
	if p := r.Ride.Preshow; p != nil && p.Enabled {
		batch = p.BatchSize
	}
	llPhase := "none"
	if sl := r.Ride.Queues.SkipLine; sl.Enabled && len(sl.Merge.Phases) > 0 {
		llPhase = sl.Merge.Phases[0].Name
	}
	sim.Emit(engine.Event{Type: engine.EvRunStart, Info: fmt.Sprintf(
		"scenario=%s ride=%s archetype=continuous_load seats=%d spacing_s=%g ride_time_s=%.0f route_scale=%.3f seed=%d fixed_times=%t length_s=%.0f warmup_s=%.0f arrivals=%s failures=%s accessibility_holds=%t load=%s brakes= preshow_batch=%d ll_phase=%s",
		id, r.Ride.Ride.ID, r.Ride.Vehicles.SeatsMax, r.Ride.Vehicles.SpacingS, c.RideTime().Seconds(), c.RouteScale(), r.Seed, r.FixedTimes,
		r.Length.Seconds(), r.Warmup.Seconds(), curveString(r.Arrivals), failures, !r.NoAccessibilityHolds, continuous.LoadZone, batch, llPhase)})

	end := engine.Time(0).Add(r.Length)
	c.Start()
	guest.StartArrivalsWith(sim, r.Arrivals, r.Guests, c, end)
	if len(r.LLArrivals) > 0 {
		ll := guest.Profile{Lane: guest.LightningLane, TimeStream: engine.StreamLLArrivals, ChoiceStream: engine.StreamLLChoice,
			IDBase: 1 << 40, PartySizes: r.Guests.PartySizes}
		guest.StartArrivalsWith(sim, r.LLArrivals, ll, c, end)
	}
	for _, w := range r.SRClosed {
		sim.At(engine.Time(w.From), func() { c.SetSingleRiderOpen(false) })
		sim.At(engine.Time(w.To), func() { c.SetSingleRiderOpen(true) })
	}
	for _, w := range r.Weather {
		sim.At(engine.Time(w.From), func() { c.Stop(continuous.CauseWeatherHold) })
		sim.At(engine.Time(w.To), func() { c.Resume(continuous.CauseWeatherHold) })
	}
	sim.RunUntil(end)
	sim.Emit(engine.Event{Type: engine.EvRunEnd, N: c.WaitingGuests()})
	return sim.Log, nil
}

// Load builds the runner for any validated scenario file.
func Load(ds *config.Dataset, f *config.ScenarioFile) (Runner, error) {
	rf := ds.Ride(f.Scenario.RideID)
	if rf == nil {
		return nil, fmt.Errorf("scenario %s: ride %s not found", f.Scenario.ID, f.Scenario.RideID)
	}
	if rf.Kind == config.KindCoaster {
		return FromFile(ds, f)
	}
	return continuousFromFile(ds, rf.Continuous, f)
}

func continuousFromFile(ds *config.Dataset, ride *config.ContinuousFile, f *config.ScenarioFile) (ContinuousRun, error) {
	// Reuse the coaster parser for the shared fields, then copy them over.
	shared, err := parseShared(ds, f, ride.Queues)
	if err != nil {
		return ContinuousRun{}, err
	}
	s := f.Scenario
	r := ContinuousRun{ID: s.ID, Ride: ride, Seed: s.Seed, FixedTimes: s.FixedTimes,
		Length: engine.Seconds(s.DurationH * 3600), Warmup: engine.Seconds(s.WarmupMin * 60), Failures: s.Failures,
		Arrivals: shared.Arrivals, LLArrivals: shared.LLArrivals, Guests: shared.Guests, Weather: shared.Weather, SRClosed: shared.SRClosed}
	if s.AccessibilityHolds != nil && !*s.AccessibilityHolds {
		r.NoAccessibilityHolds = true
	}
	return r, nil
}

// WithSeed returns run with its seed replaced.
func WithSeed(run Runner, seed uint64) Runner {
	switch r := run.(type) {
	case CoasterRun:
		r.Seed = seed
		return r
	case ContinuousRun:
		r.Seed = seed
		return r
	}
	return run
}
