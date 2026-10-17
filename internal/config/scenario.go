package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Failure kinds a scenario can enable. They must also be configured in the ride's failures section.
const (
	FailBriefStop    = "brief_stop"
	FailLongDowntime = "long_downtime"
	FailCascadeStop  = "cascade_stop"
)

// Ops actions.
const (
	OpAddTrain    = "add_train"
	OpRemoveTrain = "remove_train"
)

// Dispatch rules beyond DispatchHoldForFullOrMaxWait.
const (
	DispatchWhenReady   = "dispatch_when_ready" // dispatch as soon as the restraint check finishes
	DispatchHoldForFull = "hold_for_full"       // wait for every seat to fill, however long
)

// Posted-wait algorithms (spec §8).
const (
	PostedQueueLenOverThroughput = "queue_len_over_throughput"
	PostedLastNObserved          = "last_n_observed"
	PostedPadded                 = "padded"
)

var (
	validDispatchRules = map[string]bool{DispatchHoldForFullOrMaxWait: true, DispatchWhenReady: true, DispatchHoldForFull: true}
	validPostedWait    = map[string]bool{PostedQueueLenOverThroughput: true, PostedLastNObserved: true, PostedPadded: true}
	validFailureKinds  = map[string]bool{FailBriefStop: true, FailLongDowntime: true, FailCascadeStop: true}
)

// ScenarioFile is one run definition: which ride, how long, what demand, which failures, and scripted
// operations. Times are "HH:MM" or "HH:MM:SS" offsets from the start of the run.
type ScenarioFile struct {
	SchemaVersion string   `yaml:"schema_version"`
	Scenario      Scenario `yaml:"scenario"`
}

type Scenario struct {
	ID           string              `yaml:"id"`
	Description  string              `yaml:"description"`
	RideID       string              `yaml:"ride_id"`
	Seed         uint64              `yaml:"seed"`
	DurationH    float64             `yaml:"duration_h"`
	WarmupMin    float64             `yaml:"warmup_min"`
	Trains       int                 `yaml:"trains"` // 0 = trains.default_in_service
	FixedTimes   bool                `yaml:"fixed_times"`
	Failures     []string            `yaml:"failures"`
	DispatchRule string              `yaml:"dispatch_rule"` // "" = the ride file's rule
	PostedWait   *ScenarioPostedWait `yaml:"posted_wait"`
	Arrivals     []ArrivalStep       `yaml:"arrivals"`
	Ops          []OpsEvent          `yaml:"ops"`
	Weather      []WeatherWindow     `yaml:"weather"`
	Guests       *ScenarioGuests     `yaml:"guests"`
	// LightningLane holds Lightning Lane return arrivals (guests/hr). Party sizes follow guests.
	LightningLane *ScenarioLL `yaml:"lightning_lane"`
}

// ScenarioLL is Lightning Lane demand at the merge point. Smooth Poisson within each step for now;
// return-window clustering comes with the park model (M7, decision 2026-10-17).
type ScenarioLL struct {
	Arrivals []ArrivalStep `yaml:"arrivals"`
}

// DefaultSingleRiderShare is used when a ride has a single-rider line and the scenario gives no share.
const DefaultSingleRiderShare = 0.05 // [assumed]

// ScenarioGuests describes who arrives. Missing party sizes and patience come from the park file named
// by ParkID; without a guests section arrivals are single guests with unlimited patience.
type ScenarioGuests struct {
	ParkID           string          `yaml:"park_id"`
	PartySize        map[int]float64 `yaml:"party_size"`
	SingleRiderShare *float64        `yaml:"single_rider_share"`
	PatienceMin      *Dist           `yaml:"patience_min"`
	Abandonment      *bool           `yaml:"abandonment"` // false disables balking and reneging; default true
}

// ScenarioPostedWait overrides the ride's posted-wait settings. Zero fields keep the defaults.
type ScenarioPostedWait struct {
	Algorithm  string   `yaml:"algorithm"`
	PaddingPct *float64 `yaml:"padding_pct"`
	LastN      int      `yaml:"last_n"`
	RefreshS   float64  `yaml:"refresh_s"`
}

// ArrivalStep starts a constant standby arrival rate at At.
type ArrivalStep struct {
	At        string  `yaml:"at"`
	RatePerHr float64 `yaml:"rate_per_hr"`
}

type OpsEvent struct {
	At     string `yaml:"at"`
	Action string `yaml:"action"`
}

type WeatherWindow struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// ParseOffset parses "HH:MM" or "HH:MM:SS" into seconds from the start of the run.
func ParseOffset(s string) (float64, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("time %q: want HH:MM or HH:MM:SS", s)
	}
	total := 0.0
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (i > 0 && n >= 60) {
			return 0, fmt.Errorf("time %q: bad field %q", s, p)
		}
		total = total*60 + float64(n)
	}
	if len(parts) == 2 {
		total *= 60
	}
	return total, nil
}

var scenarioChecks = []check[*ScenarioFile]{
	{"scenario.basics", SevError, checkScenarioBasics},
	{"scenario.arrivals", SevError, checkScenarioArrivals},
	{"scenario.ops", SevError, checkScenarioOps},
	{"scenario.weather", SevError, checkScenarioWeather},
	{"scenario.enums", SevError, checkScenarioEnums},
	{"scenario.guests", SevError, checkScenarioGuests},
}

func checkScenarioGuests(f *ScenarioFile) (bool, string) {
	g := f.Scenario.Guests
	if g == nil {
		return true, "no guests section: single guests, unlimited patience"
	}
	if len(g.PartySize) > 0 {
		sum := 0.0
		for size, w := range g.PartySize {
			if size < 1 || w < 0 {
				return false, fmt.Sprintf("party_size %d: %g must be a size >= 1 with weight >= 0", size, w)
			}
			sum += w
		}
		if !approxEqual(sum, 1, shareTolerance) {
			return false, fmt.Sprintf("party_size shares sum to %.3f, want 1", sum)
		}
	}
	if s := g.SingleRiderShare; s != nil && (*s < 0 || *s > 1) {
		return false, fmt.Sprintf("single_rider_share %g outside 0..1", *s)
	}
	if p := g.PatienceMin; p != nil && (p.Mean <= 0 || p.SD < 0) {
		return false, fmt.Sprintf("patience_min mean %g must be > 0 and sd >= 0", p.Mean)
	}
	if g.ParkID == "" && len(g.PartySize) == 0 && g.PatienceMin == nil && g.SingleRiderShare == nil {
		return false, "guests section is empty; give park_id or explicit values"
	}
	return true, fmt.Sprintf("park_id %q", g.ParkID)
}

// Park returns the loaded park file with the given id, or nil.
func (d *Dataset) Park(id string) *ParkFile {
	for _, f := range d.Files {
		if f.Kind == KindPark && f.Park.Park.ID == id {
			return f.Park
		}
	}
	return nil
}

func checkScenarioBasics(f *ScenarioFile) (bool, string) {
	s := f.Scenario
	var bad []string
	if s.ID == "" {
		bad = append(bad, "missing id")
	}
	if !validRideID(s.RideID) {
		bad = append(bad, fmt.Sprintf("ride_id %q is not <resort>.<park>.<ride>", s.RideID))
	}
	if s.DurationH <= 0 {
		bad = append(bad, "duration_h must be > 0")
	}
	if s.WarmupMin < 0 || s.WarmupMin*60 >= s.DurationH*3600 {
		bad = append(bad, fmt.Sprintf("warmup_min %g must be >= 0 and shorter than the run", s.WarmupMin))
	}
	if s.Trains < 0 {
		bad = append(bad, "trains must be >= 0")
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%s on %s, %g h, warmup %g min", s.ID, s.RideID, s.DurationH, s.WarmupMin)
}

func checkScenarioArrivals(f *ScenarioFile) (bool, string) {
	if ok, detail := checkArrivalSteps("arrivals", f.Scenario.Arrivals); !ok {
		return false, detail
	}
	if ll := f.Scenario.LightningLane; ll != nil {
		if ok, detail := checkArrivalSteps("lightning_lane.arrivals", ll.Arrivals); !ok {
			return false, detail
		}
		return true, fmt.Sprintf("%d standby and %d Lightning Lane step(s)", len(f.Scenario.Arrivals), len(ll.Arrivals))
	}
	return true, fmt.Sprintf("%d arrival step(s)", len(f.Scenario.Arrivals))
}

func checkArrivalSteps(name string, steps []ArrivalStep) (bool, string) {
	if len(steps) == 0 {
		return false, "no " + name
	}
	prev := -1.0
	for i, a := range steps {
		at, err := ParseOffset(a.At)
		switch {
		case err != nil:
			return false, err.Error()
		case i == 0 && at != 0:
			return false, fmt.Sprintf("first arrival step at %s, want 00:00", a.At)
		case at <= prev:
			return false, fmt.Sprintf("arrival step %s not after the previous one", a.At)
		case a.RatePerHr < 0:
			return false, fmt.Sprintf("arrival rate %g at %s must be >= 0", a.RatePerHr, a.At)
		}
		prev = at
	}
	return true, ""
}

func checkScenarioOps(f *ScenarioFile) (bool, string) {
	end := f.Scenario.DurationH * 3600
	prev := -1.0
	for _, o := range f.Scenario.Ops {
		at, err := ParseOffset(o.At)
		switch {
		case err != nil:
			return false, err.Error()
		case at >= end:
			return false, fmt.Sprintf("ops event at %s is after the run ends", o.At)
		case at < prev:
			return false, fmt.Sprintf("ops event at %s is out of order", o.At)
		case o.Action != OpAddTrain && o.Action != OpRemoveTrain:
			return false, fmt.Sprintf("unknown ops action %q", o.Action)
		}
		prev = at
	}
	return true, fmt.Sprintf("%d ops event(s)", len(f.Scenario.Ops))
}

func checkScenarioWeather(f *ScenarioFile) (bool, string) {
	end := f.Scenario.DurationH * 3600
	for _, w := range f.Scenario.Weather {
		from, err1 := ParseOffset(w.From)
		to, err2 := ParseOffset(w.To)
		switch {
		case err1 != nil:
			return false, err1.Error()
		case err2 != nil:
			return false, err2.Error()
		case from >= to || to > end:
			return false, fmt.Sprintf("weather window %s-%s must be non-empty and inside the run", w.From, w.To)
		}
	}
	return true, fmt.Sprintf("%d weather window(s)", len(f.Scenario.Weather))
}

func checkScenarioEnums(f *ScenarioFile) (bool, string) {
	s := f.Scenario
	var bad []string
	for _, k := range s.Failures {
		if !validFailureKinds[k] {
			bad = append(bad, fmt.Sprintf("unknown failure kind %q", k))
		}
	}
	if s.DispatchRule != "" && !validDispatchRules[s.DispatchRule] {
		bad = append(bad, fmt.Sprintf("unknown dispatch_rule %q", s.DispatchRule))
	}
	if pw := s.PostedWait; pw != nil {
		if pw.Algorithm != "" && !validPostedWait[pw.Algorithm] {
			bad = append(bad, fmt.Sprintf("unknown posted_wait.algorithm %q", pw.Algorithm))
		}
		if pw.LastN < 0 || pw.RefreshS < 0 {
			bad = append(bad, "posted_wait.last_n and refresh_s must be >= 0")
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("failures %v", s.Failures)
}

// checkScenarioRide is the cross-file check: the ride exists, is a modeled archetype, and the
// scenario's trains, ops and failures fit it.
func checkScenarioRide(ds *Dataset, f *ScenarioFile) Result {
	r := Result{CheckID: "scenario.ride_refs", Severity: SevError}
	fail := func(format string, args ...any) Result {
		r.Status, r.Detail = StatusFail, fmt.Sprintf(format, args...)
		return r
	}
	s := f.Scenario
	rf := ds.Ride(s.RideID)
	if rf == nil {
		return fail("ride_id %s not found", s.RideID)
	}
	if rf.Kind != KindCoaster {
		return fail("ride %s is %s; only %s scenarios run so far", s.RideID, rf.Kind, ArchetypeMultiTrainCoaster)
	}
	c := rf.Coaster
	trains := s.Trains
	if trains == 0 {
		trains = c.Trains.DefaultInService
	}
	if trains < 1 || trains > c.Trains.MaxInService {
		return fail("trains %d outside 1..%d", trains, c.Trains.MaxInService)
	}
	if len(s.Ops) > 0 && c.Barn == nil {
		return fail("ops events need a barn; %s has none", s.RideID)
	}
	n := trains
	for _, o := range s.Ops {
		switch o.Action {
		case OpAddTrain:
			n++
		case OpRemoveTrain:
			n--
		}
		spare := c.Trains.FleetSize - n
		switch {
		case n < 1 || n > c.Trains.MaxInService:
			return fail("after %s at %s, %d trains in service (allowed 1..%d)", o.Action, o.At, n, c.Trains.MaxInService)
		case spare < 0:
			return fail("after %s at %s, %d trains exceed the fleet of %d", o.Action, o.At, n, c.Trains.FleetSize)
		case c.Barn.StorageBays != nil && spare > *c.Barn.StorageBays:
			return fail("after %s at %s, %d trains in the barn exceed %d bays", o.Action, o.At, spare, *c.Barn.StorageBays)
		}
	}
	for _, k := range s.Failures {
		if (k == FailBriefStop && c.Failures.BriefStop == nil) || (k == FailLongDowntime && c.Failures.LongDowntime == nil) ||
			(k == FailCascadeStop && c.Failures.CascadeStop == nil) {
			return fail("failure %s enabled but not configured in %s", k, rf.Path)
		}
	}
	if s.LightningLane != nil && !c.Queues.SkipLine.Enabled {
		return fail("lightning_lane arrivals but %s has no skip_line", s.RideID)
	}
	if g := s.Guests; g != nil && g.ParkID != "" && ds.Park(g.ParkID) == nil {
		return fail("guests.park_id %s not found", g.ParkID)
	}
	r.Status, r.Detail = StatusPass, fmt.Sprintf("%s (%s), %d trains", s.RideID, rf.Path, trains)
	return r
}
