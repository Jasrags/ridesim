// Package scenario wires a ride config, arrivals, failures and scripted operations into one run, either
// built directly (tests, ad hoc CLI runs) or from a scenario file under data/scenarios/.
package scenario

import (
	"fmt"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/coaster"
)

// OpsAt is a scripted ops action at an offset from the start of the run.
type OpsAt struct {
	At     engine.Duration
	Action string
}

// Window is a closed interval of simulated time, as offsets from the start.
type Window struct{ From, To engine.Duration }

// CoasterRun is one standby-only run of a multi_train_coaster.
type CoasterRun struct {
	ID         string
	Ride       *config.CoasterFile
	Trains     int // 0 = default_in_service
	Seed       uint64
	FixedTimes bool
	Length     engine.Duration
	Warmup     engine.Duration
	Arrivals   guest.Curve

	Failures     []string
	DispatchRule string
	PostedWait   coaster.PostedWaitOptions
	Ops          []OpsAt
	Weather      []Window
	SRClosed     []Window // single-rider line closed by operations

	Guests     guest.Profile // zero = single guests, unlimited patience
	LLArrivals guest.Curve   // Lightning Lane returns, guests/hr; nil = none

	CascadeScope      string  // analysis override
	CascadeToleranceS float64 // analysis override
}

// FromFile builds the run described by a validated scenario file. ds supplies the ride.
func FromFile(ds *config.Dataset, f *config.ScenarioFile) (CoasterRun, error) {
	s := f.Scenario
	rf := ds.Ride(s.RideID)
	if rf == nil || rf.Kind != config.KindCoaster {
		return CoasterRun{}, fmt.Errorf("scenario %s: ride %s not found or not a coaster", s.ID, s.RideID)
	}
	off := func(v string) (engine.Duration, error) {
		sec, err := config.ParseOffset(v)
		return engine.Seconds(sec), err
	}
	r := CoasterRun{
		ID: s.ID, Ride: rf.Coaster, Trains: s.Trains, Seed: s.Seed, FixedTimes: s.FixedTimes,
		Length: engine.Seconds(s.DurationH * 3600), Warmup: engine.Seconds(s.WarmupMin * 60),
		Failures: s.Failures, DispatchRule: s.DispatchRule,
	}
	if pw := s.PostedWait; pw != nil {
		r.PostedWait = coaster.PostedWaitOptions{Algorithm: pw.Algorithm, PaddingPct: pw.PaddingPct, LastN: pw.LastN, RefreshS: pw.RefreshS}
	}
	prof, err := guestProfile(ds, rf.Coaster, s.Guests)
	if err != nil {
		return CoasterRun{}, fmt.Errorf("scenario %s: %w", s.ID, err)
	}
	r.Guests = prof
	for _, a := range s.Arrivals {
		at, err := off(a.At)
		if err != nil {
			return CoasterRun{}, err
		}
		r.Arrivals = append(r.Arrivals, guest.Segment{Start: at, RatePerHr: a.RatePerHr})
	}
	if ll := s.LightningLane; ll != nil {
		for _, a := range ll.Arrivals {
			at, err := off(a.At)
			if err != nil {
				return CoasterRun{}, err
			}
			r.LLArrivals = append(r.LLArrivals, guest.Segment{Start: at, RatePerHr: a.RatePerHr})
		}
	}
	for _, o := range s.Ops {
		at, err := off(o.At)
		if err != nil {
			return CoasterRun{}, err
		}
		r.Ops = append(r.Ops, OpsAt{At: at, Action: o.Action})
	}
	for _, w := range s.SingleRiderClosed {
		from, err1 := off(w.From)
		to, err2 := off(w.To)
		if err1 != nil || err2 != nil {
			return CoasterRun{}, fmt.Errorf("scenario %s: single_rider_closed window %s-%s", s.ID, w.From, w.To)
		}
		r.SRClosed = append(r.SRClosed, Window{From: from, To: to})
	}
	for _, w := range s.Weather {
		from, err1 := off(w.From)
		to, err2 := off(w.To)
		if err1 != nil || err2 != nil {
			return CoasterRun{}, fmt.Errorf("scenario %s: weather window %s-%s", s.ID, w.From, w.To)
		}
		r.Weather = append(r.Weather, Window{From: from, To: to})
	}
	return r, nil
}

// Run executes the scenario and returns its event log.
func (r CoasterRun) Run() (*engine.Log, error) {
	sim := engine.New(r.Seed)
	c, err := coaster.New(sim, r.Ride, coaster.Options{
		Trains: r.Trains, FixedTimes: r.FixedTimes, Failures: r.Failures, DispatchRule: r.DispatchRule,
		PostedWait: r.PostedWait, CascadeScope: r.CascadeScope, CascadeToleranceS: r.CascadeToleranceS,
	})
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
		"scenario=%s ride=%s trains=%d seats=%d seed=%d fixed_times=%t length_s=%.0f warmup_s=%.0f arrivals=%s failures=%s load=%s brakes=%s preshow_batch=%d ll_phase=%s",
		id, r.Ride.Ride.ID, trains, r.Ride.Trains.SeatsPerTrain, r.Seed, r.FixedTimes, r.Length.Seconds(), r.Warmup.Seconds(),
		curveString(r.Arrivals), failures, c.LoadBlock(), strings.Join(brakes, ","), batch, llPhase)})

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
		sim.At(engine.Time(w.From), func() { c.Stop(coaster.CauseWeatherHold) })
		sim.At(engine.Time(w.To), func() { c.Resume(coaster.CauseWeatherHold) })
	}
	for _, o := range r.Ops {
		switch o.Action {
		case config.OpAddTrain:
			sim.At(engine.Time(o.At), c.AddTrain)
		case config.OpRemoveTrain:
			sim.At(engine.Time(o.At), c.RemoveTrain)
		default:
			return nil, fmt.Errorf("unknown ops action %q", o.Action)
		}
	}
	sim.RunUntil(end)
	sim.Emit(engine.Event{Type: engine.EvRunEnd, N: c.QueueGuests()})
	return sim.Log, nil
}

// guestProfile resolves a scenario guests section against the park file and ride.
func guestProfile(ds *config.Dataset, ride *config.CoasterFile, g *config.ScenarioGuests) (guest.Profile, error) {
	if g == nil {
		return guest.Profile{}, nil
	}
	var prof guest.Profile
	sizes, patience := g.PartySize, g.PatienceMin
	if g.ParkID != "" {
		park := ds.Park(g.ParkID)
		if park == nil {
			return prof, fmt.Errorf("park %s not found", g.ParkID)
		}
		if len(sizes) == 0 {
			sizes = park.Guests.PartySize
		}
		if patience == nil {
			p := park.Guests.PatienceMin
			patience = &p
		}
	}
	prof.PartySizes = guest.PartySizesFrom(sizes)
	if g.Abandonment == nil || *g.Abandonment {
		prof.PatienceMin = patience
	}
	if ride.Queues.SingleRider.Enabled {
		prof.SingleRiderShare = config.DefaultSingleRiderShare
		if g.SingleRiderShare != nil {
			prof.SingleRiderShare = *g.SingleRiderShare
		}
	}
	return prof, nil
}

func curveString(c guest.Curve) string {
	parts := make([]string, len(c))
	for i, s := range c {
		parts[i] = fmt.Sprintf("%g:%g", s.Start.Seconds()/60, s.RatePerHr)
	}
	return strings.Join(parts, ",")
}
