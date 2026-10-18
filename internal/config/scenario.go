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
	FailChainStop    = "chain_stop" // continuous_load: the whole chain halts
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
	validFailureKinds  = map[string]bool{FailBriefStop: true, FailLongDowntime: true, FailCascadeStop: true, FailChainStop: true}
)

// ScenarioFile is one run definition: which ride, how long, what demand, which failures, and scripted
// operations. Times are "HH:MM" or "HH:MM:SS" offsets from the start of the run.
type ScenarioFile struct {
	SchemaVersion string   `yaml:"schema_version"`
	Scenario      Scenario `yaml:"scenario"`
}

type Scenario struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	RideID      string `yaml:"ride_id"`
	// Park runs (M7) name park_id instead of ride_id, with a crowd level or an explicit attendance.
	ParkID          string  `yaml:"park_id"`
	CrowdLevel      string  `yaml:"crowd_level"` // low | medium | high: the park's daily_attendance
	Attendance      int     `yaml:"attendance"`
	WalkingMode     string  `yaml:"walking_mode"` // "" = the park file's mode
	NoLightningLane bool    `yaml:"no_lightning_lane"`
	Seed            uint64  `yaml:"seed"`
	DurationH       float64 `yaml:"duration_h"`
	WarmupMin       float64 `yaml:"warmup_min"`
	// AccessibilityHolds turns continuous_load accessibility holds off when false (default on).
	AccessibilityHolds *bool               `yaml:"accessibility_holds"`
	Trains             int                 `yaml:"trains"` // 0 = trains.default_in_service
	FixedTimes         bool                `yaml:"fixed_times"`
	Failures           []string            `yaml:"failures"`
	DispatchRule       string              `yaml:"dispatch_rule"` // "" = the ride file's rule
	PostedWait         *ScenarioPostedWait `yaml:"posted_wait"`
	Arrivals           []ArrivalStep       `yaml:"arrivals"`
	Ops                []OpsEvent          `yaml:"ops"`
	Weather            []WeatherWindow     `yaml:"weather"`
	Guests             *ScenarioGuests     `yaml:"guests"`
	// LightningLane holds Lightning Lane return arrivals (guests/hr). Party sizes follow guests.
	LightningLane *ScenarioLL `yaml:"lightning_lane"`
	// SingleRiderClosed are windows when operations close the single-rider line ([user]: it is not
	// always open; the criteria are unknown, so they are scripted).
	SingleRiderClosed []WeatherWindow `yaml:"single_rider_closed"`
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

// OpsEvent is a scripted operations action. Ride scenarios use add_train / remove_train on their ride; park
// scenarios (M8 live levers) name the node and may set minutes (close_ride) or value (crowd_scale).
type OpsEvent struct {
	At      string   `yaml:"at"`
	Action  string   `yaml:"action"`
	Node    string   `yaml:"node,omitempty"`
	Minutes float64  `yaml:"minutes,omitempty"`
	Value   *float64 `yaml:"value,omitempty"`
}

// Park ops actions (besides add_train / remove_train) and the closure cause they use.
const (
	OpSingleRiderOpen  = "single_rider_open"
	OpSingleRiderClose = "single_rider_close"
	OpCloseRide        = "close_ride"
	OpCrowdScale       = "crowd_scale"

	CauseOpsClosure = "ops_closure" // operations closed the ride: the board shows it closed

	MaxCrowdScale = 2.0 // crowd scale thins a day drawn at this multiple of attendance
)

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

func checkContinuousScenario(ds *Dataset, rf *File, f *ScenarioFile) Result {
	r := Result{CheckID: "scenario.ride_refs", Severity: SevError, Status: StatusFail}
	s, c := f.Scenario, rf.Continuous
	switch {
	case s.Trains != 0 || len(s.Ops) > 0:
		r.Detail = fmt.Sprintf("%s is continuous_load: trains and barn ops do not apply", s.RideID)
		return r
	case s.DispatchRule != "":
		r.Detail = "dispatch_rule does not apply to continuous_load"
		return r
	case s.LightningLane != nil && !c.Queues.SkipLine.Enabled:
		r.Detail = fmt.Sprintf("lightning_lane arrivals but %s has no skip_line", s.RideID)
		return r
	case len(s.SingleRiderClosed) > 0 && !c.Queues.SingleRider.Enabled:
		r.Detail = fmt.Sprintf("single_rider_closed windows but %s has no single-rider line", s.RideID)
		return r
	}
	for _, k := range s.Failures {
		if (k == FailChainStop && c.Failures.ChainStop == nil) || (k == FailLongDowntime && c.Failures.LongDowntime == nil) ||
			(k == FailSideDown && (c.Failures.SideDown == nil || c.Conveyor.ChainCount() < 2)) || k == FailBriefStop || k == FailCascadeStop {
			r.Detail = fmt.Sprintf("failure %s not configured for continuous_load ride %s", k, s.RideID)
			return r
		}
	}
	if g := s.Guests; g != nil && g.ParkID != "" && ds.Park(g.ParkID) == nil {
		r.Detail = fmt.Sprintf("guests.park_id %s not found", g.ParkID)
		return r
	}
	r.Status, r.Detail = StatusPass, fmt.Sprintf("%s (%s), continuous_load", s.RideID, rf.Path)
	return r
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

// IsPark reports whether the scenario is a park run.
func (s *Scenario) IsPark() bool { return s.ParkID != "" }

// Crowd levels.
const (
	CrowdLow    = "low"
	CrowdMedium = "medium"
	CrowdHigh   = "high"
)

// parkOnlyConflicts lists ride-scenario fields a park scenario must not set.
func parkOnlyConflicts(s *Scenario) []string {
	var bad []string
	set := map[string]bool{
		"ride_id": s.RideID != "", "trains": s.Trains != 0, "dispatch_rule": s.DispatchRule != "",
		"posted_wait": s.PostedWait != nil, "arrivals": len(s.Arrivals) > 0,
		"weather": len(s.Weather) > 0, "guests": s.Guests != nil, "lightning_lane": s.LightningLane != nil,
		"single_rider_closed": len(s.SingleRiderClosed) > 0, "accessibility_holds": s.AccessibilityHolds != nil,
	}
	for _, k := range []string{"ride_id", "trains", "dispatch_rule", "posted_wait", "arrivals", "weather", "guests",
		"lightning_lane", "single_rider_closed", "accessibility_holds"} {
		if set[k] {
			bad = append(bad, k+" is for ride scenarios; park runs derive it from the park")
		}
	}
	if (s.CrowdLevel == "") == (s.Attendance == 0) {
		bad = append(bad, "give exactly one of crowd_level and attendance")
	}
	if s.Attendance < 0 {
		bad = append(bad, "attendance must be > 0")
	}
	return bad
}

func checkScenarioBasics(f *ScenarioFile) (bool, string) {
	s := f.Scenario
	var bad []string
	if s.ID == "" {
		bad = append(bad, "missing id")
	}
	if s.IsPark() {
		bad = append(bad, parkOnlyConflicts(&s)...)
	} else {
		if !validRideID(s.RideID) {
			bad = append(bad, fmt.Sprintf("ride_id %q is not <resort>.<park>.<ride>", s.RideID))
		}
		if s.CrowdLevel != "" || s.Attendance != 0 || s.WalkingMode != "" || s.NoLightningLane {
			bad = append(bad, "crowd_level, attendance, walking_mode and no_lightning_lane are for park scenarios")
		}
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
	on := s.RideID
	if s.IsPark() {
		on = "park " + s.ParkID
	}
	return true, fmt.Sprintf("%s on %s, %g h, warmup %g min", s.ID, on, s.DurationH, s.WarmupMin)
}

func checkScenarioArrivals(f *ScenarioFile) (bool, string) {
	if f.Scenario.IsPark() {
		return true, "park run: arrivals come from the park's attendance and hourly shares"
	}
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
		}
		if bad := OpShape(o, f.Scenario.IsPark()); bad != "" {
			return false, fmt.Sprintf("ops event at %s: %s", o.At, bad)
		}
		prev = at
	}
	return true, fmt.Sprintf("%d ops event(s)", len(f.Scenario.Ops))
}

func checkScenarioWeather(f *ScenarioFile) (bool, string) {
	end := f.Scenario.DurationH * 3600
	windows := append(append([]WeatherWindow(nil), f.Scenario.Weather...), f.Scenario.SingleRiderClosed...)
	for _, w := range windows {
		from, err1 := ParseOffset(w.From)
		to, err2 := ParseOffset(w.To)
		switch {
		case err1 != nil:
			return false, err1.Error()
		case err2 != nil:
			return false, err2.Error()
		case from >= to || to > end:
			return false, fmt.Sprintf("window %s-%s must be non-empty and inside the run", w.From, w.To)
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
	if c := s.CrowdLevel; c != "" && c != CrowdLow && c != CrowdMedium && c != CrowdHigh {
		bad = append(bad, fmt.Sprintf("unknown crowd_level %q (low, medium, high)", c))
	}
	if m := s.WalkingMode; m != "" && m != WalkingFixedDelay && m != WalkingCongestion {
		bad = append(bad, fmt.Sprintf("unknown walking_mode %q", m))
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
	if s.IsPark() {
		park := ds.Park(s.ParkID)
		if park == nil {
			return fail("park_id %s not found", s.ParkID)
		}
		if bad := checkParkOps(ds, park, s.Ops); bad != "" {
			return fail("%s", bad)
		}
		r.Status, r.Detail = StatusPass, fmt.Sprintf("park %s, %d guests, %d ops", s.ParkID, s.ParkAttendance(park), len(s.Ops))
		return r
	}
	rf := ds.Ride(s.RideID)
	if rf == nil {
		return fail("ride_id %s not found", s.RideID)
	}
	switch rf.Kind {
	case KindContinuous:
		return checkContinuousScenario(ds, rf, f)
	case KindElevator:
		return checkElevatorScenario(ds, rf, f)
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
	if len(s.SingleRiderClosed) > 0 && !c.Queues.SingleRider.Enabled {
		return fail("single_rider_closed windows but %s has no single-rider line", s.RideID)
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

// ParkAttendance is the day's guests: attendance, or the park's figure for crowd_level.
func (s *Scenario) ParkAttendance(p *ParkFile) int {
	if s.Attendance > 0 {
		return s.Attendance
	}
	switch s.CrowdLevel {
	case CrowdLow:
		return p.Guests.DailyAttendance.Low
	case CrowdHigh:
		return p.Guests.DailyAttendance.High
	}
	return p.Guests.DailyAttendance.Medium
}

// OpShape checks an ops event has the fields its action needs ("" = fine).
func OpShape(o OpsEvent, park bool) string {
	if !park {
		if o.Action != OpAddTrain && o.Action != OpRemoveTrain {
			return fmt.Sprintf("unknown ops action %q", o.Action)
		}
		if o.Node != "" || o.Minutes != 0 || o.Value != nil {
			return "node, minutes and value are for park scenarios"
		}
		return ""
	}
	switch o.Action {
	case OpAddTrain, OpRemoveTrain, OpSingleRiderOpen, OpSingleRiderClose:
		if o.Node == "" || o.Minutes != 0 || o.Value != nil {
			return o.Action + " needs node only"
		}
	case OpCloseRide:
		if o.Node == "" || o.Minutes <= 0 || o.Value != nil {
			return "close_ride needs node and minutes > 0"
		}
	case OpCrowdScale:
		if o.Node != "" || o.Minutes != 0 || o.Value == nil || *o.Value < 0 || *o.Value > MaxCrowdScale {
			return fmt.Sprintf("crowd_scale needs value in [0, %g] and nothing else", MaxCrowdScale)
		}
	default:
		return fmt.Sprintf("unknown park ops action %q", o.Action)
	}
	return ""
}

// checkParkOps is the cross-file check: each op names a node that supports it.
func checkParkOps(ds *Dataset, park *ParkFile, ops []OpsEvent) string {
	nodes := map[string]ParkNode{}
	for _, n := range park.Nodes {
		nodes[n.ID] = n
	}
	for _, o := range ops {
		if o.Action == OpCrowdScale {
			continue
		}
		n, ok := nodes[o.Node]
		if !ok {
			return fmt.Sprintf("ops %s at %s: node %q not in park %s", o.Action, o.At, o.Node, park.Park.ID)
		}
		var rf *File
		if n.Kind == NodeRide {
			rf = ds.Ride(n.RideID)
		}
		switch o.Action {
		case OpAddTrain, OpRemoveTrain:
			if rf == nil || rf.Kind != KindCoaster || rf.Coaster.Barn == nil {
				return fmt.Sprintf("ops %s at %s: %s is not a coaster with a barn", o.Action, o.At, o.Node)
			}
		case OpSingleRiderOpen, OpSingleRiderClose:
			if rf == nil || !rf.RideInfoQueues().SingleRider.Enabled {
				return fmt.Sprintf("ops %s at %s: %s has no single-rider line", o.Action, o.At, o.Node)
			}
		case OpCloseRide:
			if rf == nil {
				return fmt.Sprintf("ops close_ride at %s: %s is not a modeled ride", o.At, o.Node)
			}
		}
	}
	return ""
}
