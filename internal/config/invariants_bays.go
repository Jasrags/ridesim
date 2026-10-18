package config

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var baysChecks = []check[*BaysFile]{
	{"bays.count", SevError, checkBaysCount},
	{"bays.seats", SevError, checkBaysSeats},
	{"bays.times", SevError, checkBaysTimes},
	{"bays.duration", SevError, checkBaysDuration},
	{"bays.capacity", SevWarn, checkBaysCapacity},
}

func checkBaysCount(c *BaysFile) (bool, string) {
	if sd := c.Failures.SideDown; sd != nil && (c.Bays.Count < 2 || sd.DurationS.Mean <= 0 || sd.DurationS.SD < 0 || sd.RatePerHr < 0) {
		return false, "failures.side_down needs >= 2 bays, rate_per_hr >= 0 and a positive duration_s"
	}
	return c.Bays.Count >= 1, fmt.Sprintf("%d bays", c.Bays.Count)
}

func checkBaysSeats(c *BaysFile) (bool, string) {
	b := c.Bays
	if c.Cycle.CalloutParties < 0 {
		return false, fmt.Sprintf("callout_parties %d must be >= 0", c.Cycle.CalloutParties)
	}
	if g := b.BoardingGroups; len(g) > 0 {
		sum := 0
		for _, n := range g {
			if n < 1 {
				return false, fmt.Sprintf("boarding_groups %v: every group needs a seat", g)
			}
			sum += n
		}
		return sum == b.SeatsPerBay, fmt.Sprintf("boarding_groups %v sum to %d, seats_per_bay %d", g, sum, b.SeatsPerBay)
	}
	if c.Cycle.CalloutParties > 0 {
		return false, "callout_parties needs boarding_groups"
	}
	ok := b.SeatsPerRow > 0 && b.SeatsPerBay > 0 && b.SeatsPerBay%b.SeatsPerRow == 0
	return ok, fmt.Sprintf("%d seats in rows of %d", b.SeatsPerBay, b.SeatsPerRow)
}

func checkBaysTimes(c *BaysFile) (bool, string) {
	y := c.Cycle
	var bad []string
	for name, d := range map[string]Dist{"load_s": y.LoadS, "flight_s": y.FlightS} {
		if d.Mean <= 0 || d.SD < 0 {
			bad = append(bad, name)
		}
	}
	for name, d := range map[string]Dist{"exit_s": y.ExitS, "restraint_check_s": y.RestraintCheckS, "safety_video_s": y.SafetyVideoS} {
		if d.Mean < 0 || d.SD < 0 {
			bad = append(bad, name)
		}
	}
	if y.MaxWaitAfterReadyS < 0 {
		bad = append(bad, "max_wait_after_ready_s")
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return false, "bad times: " + strings.Join(bad, ", ")
	}
	return true, "cycle times present and non-negative"
}

// checkBaysDuration: ride_duration_s is the flight.
func checkBaysDuration(c *BaysFile) (bool, string) {
	return approxEqual(c.Cycle.FlightS.Mean, c.Ride.RideDurationS, durationToleranceS),
		fmt.Sprintf("flight_s %g s vs ride_duration_s %g s (+/- %g)", c.Cycle.FlightS.Mean, c.Ride.RideDurationS, durationToleranceS)
}

// checkBaysCapacity compares bays x seats x 3600 / cycle with the target (theoretical_riders_per_hr_at_target,
// else the listed capacity).
func checkBaysCapacity(c *BaysFile) (bool, string) {
	cycle := c.CycleS()
	want := c.ValidationTargets.TheoreticalRidersPerHrAtTarget
	if want <= 0 {
		want = c.Ride.ListedCapacityPerHr
	}
	if cycle <= 0 || want <= 0 {
		return true, "no capacity to compare"
	}
	theo := float64(c.Bays.Count*c.Bays.SeatsPerBay) * 3600 / cycle
	off := theo/want - 1
	return math.Abs(off) <= elevatorCapacityTolerance, fmt.Sprintf("%d bays x %d seats x 3600 / %.0f s = %.0f/hr vs %.0f/hr: %+.1f%%",
		c.Bays.Count, c.Bays.SeatsPerBay, cycle, theo, want, off*100)
}

// checkBaysScenario is the scenario.ride_refs check for a simulator_bays ride.
func checkBaysScenario(ds *Dataset, rf *File, f *ScenarioFile) Result {
	r := Result{CheckID: "scenario.ride_refs", Severity: SevError, Status: StatusFail}
	s, c := f.Scenario, rf.Bays
	switch {
	case s.Trains != 0 || len(s.Ops) > 0 || s.DispatchRule != "":
		r.Detail = fmt.Sprintf("%s is simulator_bays: trains, barn ops and dispatch rules do not apply", s.RideID)
		return r
	case s.LightningLane != nil && !c.Queues.SkipLine.Enabled:
		r.Detail = fmt.Sprintf("lightning_lane arrivals but %s has no skip_line", s.RideID)
		return r
	case len(s.SingleRiderClosed) > 0 && !c.Queues.SingleRider.Enabled:
		r.Detail = fmt.Sprintf("single_rider_closed windows but %s has no single-rider line", s.RideID)
		return r
	}
	for _, k := range s.Failures {
		f := c.Failures
		ok := (k == FailBriefStop && f.BriefStop != nil) || (k == FailLongDowntime && f.LongDowntime != nil) || (k == FailSideDown && f.SideDown != nil)
		if !ok {
			r.Detail = fmt.Sprintf("failure %s not configured for simulator_bays ride %s", k, s.RideID)
			return r
		}
	}
	if g := s.Guests; g != nil && g.ParkID != "" && ds.Park(g.ParkID) == nil {
		r.Detail = fmt.Sprintf("guests.park_id %s not found", g.ParkID)
		return r
	}
	r.Status, r.Detail = StatusPass, fmt.Sprintf("%s (%s), simulator_bays", s.RideID, rf.Path)
	return r
}
