package config

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// Severity of a check: Error blocks loading, Warn is reported and loading continues.
type Severity int

const (
	SevWarn Severity = iota + 1
	SevError
)

func (s Severity) String() string {
	if s == SevError {
		return "error"
	}
	return "warn"
}

// Status is the outcome of one check.
type Status int

const (
	StatusPass Status = iota + 1
	StatusWarn
	StatusFail
)

func (s Status) String() string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusWarn:
		return "WARN"
	case StatusFail:
		return "FAIL"
	}
	return "?"
}

// Result is one check applied to one file.
type Result struct {
	CheckID  string
	Severity Severity
	Status   Status
	Detail   string
}

// check is a named predicate. fn returns ok and a detail line (shown on pass too, so the report
// carries the numbers behind each verdict).
type check[T any] struct {
	id  string
	sev Severity
	fn  func(T) (ok bool, detail string)
}

func run[T any](checks []check[T], v T) []Result {
	out := make([]Result, 0, len(checks))
	for _, c := range checks {
		ok, detail := c.fn(v)
		st := StatusPass
		if !ok {
			st = StatusFail
			if c.sev == SevWarn {
				st = StatusWarn
			}
		}
		out = append(out, Result{CheckID: c.id, Severity: c.sev, Status: st, Detail: detail})
	}
	return out
}

// CheckFile runs every file-local check for f's kind. Cross-file checks run in Validate.
func CheckFile(f *File) []Result {
	switch f.Kind {
	case KindCoaster:
		res := run(rideChecks, rideCtx{f.Path, &f.Coaster.Ride, f.Coaster.Preshow, &f.Coaster.Queues})
		return append(res, run(coasterChecks, f.Coaster)...)
	case KindContinuous:
		res := run(rideChecks, rideCtx{f.Path, &f.Continuous.Ride, f.Continuous.Preshow, &f.Continuous.Queues})
		return append(res, run(continuousChecks, f.Continuous)...)
	case KindElevator:
		res := run(rideChecks, rideCtx{f.Path, &f.Elevator.Ride, f.Elevator.Preshow, &f.Elevator.Queues})
		return append(res, run(elevatorChecks, f.Elevator)...)
	case KindBays:
		res := run(rideChecks, rideCtx{f.Path, &f.Bays.Ride, f.Bays.Preshow, &f.Bays.Queues})
		return append(res, run(baysChecks, f.Bays)...)
	case KindPark:
		return run(parkChecks, f.Park)
	case KindScenario:
		return run(scenarioChecks, f.Scenario)
	case KindStudy:
		return run(studyChecks, f.Study)
	}
	return nil
}

// ---------------------------------------------------------------------------------------------------
// Checks shared by every ride archetype

type rideCtx struct {
	path    string
	ride    *RideInfo
	preshow *Preshow
	queues  *Queues
}

var rideChecks = []check[rideCtx]{
	{"ride.id_format", SevError, checkRideIDFormat},
	{"ride.filename", SevError, checkRideFilename},
	{"preshow.throughput", SevError, checkPreshowThroughput},
	{"queues.merge_phases", SevError, checkMergePhases},
	{"queues.posted_wait", SevError, checkPostedWaitAlgorithm},
}

func checkPostedWaitAlgorithm(c rideCtx) (bool, string) {
	a := c.queues.PostedWait.Algorithm
	return a == "" || validPostedWait[a], fmt.Sprintf("posted_wait.algorithm %q", a)
}

func validRideID(id string) bool {
	parts := strings.Split(id, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}

func checkRideIDFormat(c rideCtx) (bool, string) {
	if !validRideID(c.ride.ID) {
		return false, fmt.Sprintf("ride.id %q is not <resort>.<park>.<ride>", c.ride.ID)
	}
	return true, c.ride.ID
}

func checkRideFilename(c rideCtx) (bool, string) {
	if !validRideID(c.ride.ID) {
		return true, "skipped: ride.id malformed (see ride.id_format)"
	}
	// Spec §3 says "dots replaced by dashes"; every data file also maps underscores to dashes, so both are.
	want := strings.NewReplacer(".", "-", "_", "-").Replace(c.ride.ID)
	got := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	if got != want {
		return false, fmt.Sprintf("filename %q, want %q.yaml for id %s", filepath.Base(c.path), want, c.ride.ID)
	}
	return true, filepath.Base(c.path)
}

// preshowThroughputPerHr = rooms * batch_size * 3600 / (duration + turnover).
func preshowThroughputPerHr(p *Preshow) float64 {
	cycle := p.DurationS.Mean + p.TurnoverS.Mean
	if cycle <= 0 {
		return 0
	}
	return float64(p.Rooms*p.BatchSize) * 3600 / cycle
}

func checkPreshowThroughput(c rideCtx) (bool, string) {
	if c.preshow == nil || !c.preshow.Enabled {
		return true, "no preshow"
	}
	if s := c.preshow.Staging; s != "" && s != PreshowStagingHallway {
		return false, fmt.Sprintf("unknown preshow.staging %q", s)
	}
	tp := preshowThroughputPerHr(c.preshow)
	detail := fmt.Sprintf("%d room(s) x %d / (%.0f + %.0f s) = %.0f/hr vs listed %.0f/hr",
		c.preshow.Rooms, c.preshow.BatchSize, c.preshow.DurationS.Mean, c.preshow.TurnoverS.Mean, tp, c.ride.ListedCapacityPerHr)
	return tp >= c.ride.ListedCapacityPerHr, detail
}

// Merge phases must have strictly increasing thresholds and end in exactly one unbounded phase.
func checkMergePhases(c rideCtx) (bool, string) {
	phases := c.queues.SkipLine.Merge.Phases
	if len(phases) == 0 {
		return true, "no merge phases"
	}
	prev := math.MinInt
	for i, p := range phases {
		last := i == len(phases)-1
		switch {
		case p.WhenLLQueueGuestsBelow == nil && !last:
			return false, fmt.Sprintf("phase %q is unbounded but not last", p.Name)
		case p.WhenLLQueueGuestsBelow == nil:
		case last:
			return false, fmt.Sprintf("last phase %q must be unbounded (when_ll_queue_guests_below: null)", p.Name)
		case *p.WhenLLQueueGuestsBelow <= prev:
			return false, fmt.Sprintf("phase %q threshold %d not above previous %d", p.Name, *p.WhenLLQueueGuestsBelow, prev)
		default:
			prev = *p.WhenLLQueueGuestsBelow
		}
		if p.LLPerStandby <= 0 {
			return false, fmt.Sprintf("phase %q ll_per_standby must be > 0", p.Name)
		}
	}
	return true, fmt.Sprintf("%d phases", len(phases))
}

// approxEqual reports |a-b| <= tol.
func approxEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }
