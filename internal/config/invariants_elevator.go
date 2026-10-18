package config

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// elevatorCapacityTolerance is how far the theoretical capacity may sit from the listed figure before
// validation warns.
const elevatorCapacityTolerance = 0.05

var elevatorChecks = []check[*ElevatorFile]{
	{"elevator.sides", SevError, checkElevatorSides},
	{"elevator.fleet", SevError, checkElevatorFleet},
	{"elevator.no_gridlock", SevError, checkElevatorNoGridlock},
	{"elevator.seat_rows", SevError, checkElevatorSeatRows},
	{"elevator.times", SevError, checkElevatorTimes},
	{"elevator.duration_sum", SevError, checkElevatorDuration},
	{"elevator.capacity", SevWarn, checkElevatorCapacity},
}

func checkElevatorSides(c *ElevatorFile) (bool, string) {
	s := c.Sides
	ok := s.Count >= 1 && s.ShowShafts >= 1 && s.DropShafts >= 1 && s.UnloadDoors >= 1 &&
		(len(s.Names) == 0 || len(s.Names) == s.Count)
	seen := map[string]bool{}
	for _, n := range s.Names { // names become place ids (SHOW_<name>_1), so they must be distinct words
		if n == "" || strings.ContainsAny(n, " \t_") || seen[n] {
			ok = false
		}
		seen[n] = true
	}
	return ok, fmt.Sprintf("%d sides (%s), each %d show shafts, %d drop shafts, %d unload doors",
		s.Count, strings.Join(s.Names, ","), s.ShowShafts, s.DropShafts, s.UnloadDoors)
}

func checkElevatorFleet(c *ElevatorFile) (bool, string) {
	v, n := c.Vehicles, c.Sides.Count
	return v.PerSide >= 1 && v.PerSide*n <= v.FleetSize, fmt.Sprintf("%d per side x %d sides <= fleet %d", v.PerSide, n, v.FleetSize)
}

// checkElevatorNoGridlock: every cab waits for the next resource while holding its current one. With
// fewer cabs than places to hold them, at least one place is always free, so the cycle cannot lock.
func checkElevatorNoGridlock(c *ElevatorFile) (bool, string) {
	s := c.Sides
	places := s.ShowShafts + s.DropShafts + s.UnloadDoors
	return c.Vehicles.PerSide < places, fmt.Sprintf("%d cabs per side < %d shafts and doors", c.Vehicles.PerSide, places)
}

func checkElevatorSeatRows(c *ElevatorFile) (bool, string) {
	v := c.Vehicles
	if c.Cycle.CalloutParties < 0 {
		return false, fmt.Sprintf("callout_parties %d must be >= 0", c.Cycle.CalloutParties)
	}
	if g := v.BoardingGroups; len(g) > 0 {
		sum := 0
		for _, n := range g {
			if n < 1 {
				return false, fmt.Sprintf("boarding_groups %v: every group needs a seat", g)
			}
			sum += n
		}
		return sum == v.SeatsPerVehicle, fmt.Sprintf("boarding_groups %v sum to %d, seats_per_vehicle %d", g, sum, v.SeatsPerVehicle)
	}
	ok := v.SeatsPerRow > 0 && v.SeatsPerVehicle > 0 && v.SeatsPerVehicle%v.SeatsPerRow == 0
	return ok, fmt.Sprintf("%d seats in rows of %d", v.SeatsPerVehicle, v.SeatsPerRow)
}

func checkElevatorTimes(c *ElevatorFile) (bool, string) {
	y := c.Cycle
	var bad []string
	for name, d := range map[string]Dist{"unload_s": y.UnloadS, "load_s": y.LoadS, "show_s": y.ShowS, "drop_s": y.DropS} {
		if d.Mean <= 0 || d.SD < 0 {
			bad = append(bad, name)
		}
	}
	for name, d := range map[string]Dist{"to_show_shaft_s": y.ToShowShaftS, "lift_to_load_s": y.LiftToLoadS,
		"restraint_check_s": y.RestraintCheckS, "dispatch_clearance_s": y.DispatchClearanceS, "to_unload_s": y.ToUnloadS} {
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

// checkElevatorDuration: the ride time runs from dispatch at load to arrival at the unload door.
func checkElevatorDuration(c *ElevatorFile) (bool, string) {
	y := c.Cycle
	sum := y.ShowS.Mean + y.DropS.Mean + y.ToUnloadS.Mean
	return approxEqual(sum, c.Ride.RideDurationS, durationToleranceS),
		fmt.Sprintf("show + drop + to_unload = %g s vs ride_duration_s %g s (+/- %g)", sum, c.Ride.RideDurationS, durationToleranceS)
}

func checkElevatorCapacity(c *ElevatorFile) (bool, string) {
	iv := c.SideInterval()
	if iv <= 0 || c.Ride.ListedCapacityPerHr <= 0 {
		return true, "no listed capacity to compare"
	}
	theo := float64(c.Sides.Count*c.Vehicles.SeatsPerVehicle) * 3600 / iv
	off := theo/c.Ride.ListedCapacityPerHr - 1
	return math.Abs(off) <= elevatorCapacityTolerance, fmt.Sprintf("theoretical %.0f/hr (%.1f s per side) vs listed %.0f/hr: %+.1f%%",
		theo, iv, c.Ride.ListedCapacityPerHr, off*100)
}

// checkElevatorScenario is the scenario.ride_refs check for an elevator_drop ride.
func checkElevatorScenario(ds *Dataset, rf *File, f *ScenarioFile) Result {
	r := Result{CheckID: "scenario.ride_refs", Severity: SevError, Status: StatusFail}
	s, c := f.Scenario, rf.Elevator
	switch {
	case s.Trains != 0 || len(s.Ops) > 0:
		r.Detail = fmt.Sprintf("%s is elevator_drop: trains and barn ops do not apply", s.RideID)
		return r
	case s.DispatchRule != "":
		r.Detail = "dispatch_rule does not apply to elevator_drop"
		return r
	case s.LightningLane != nil && !c.Queues.SkipLine.Enabled:
		r.Detail = fmt.Sprintf("lightning_lane arrivals but %s has no skip_line", s.RideID)
		return r
	case len(s.SingleRiderClosed) > 0 && !c.Queues.SingleRider.Enabled:
		r.Detail = fmt.Sprintf("single_rider_closed windows but %s has no single-rider line", s.RideID)
		return r
	}
	for _, k := range s.Failures {
		if !elevatorFailureConfigured(c, k) {
			r.Detail = fmt.Sprintf("failure %s not configured for elevator_drop ride %s", k, s.RideID)
			return r
		}
	}
	if g := s.Guests; g != nil && g.ParkID != "" && ds.Park(g.ParkID) == nil {
		r.Detail = fmt.Sprintf("guests.park_id %s not found", g.ParkID)
		return r
	}
	r.Status, r.Detail = StatusPass, fmt.Sprintf("%s (%s), elevator_drop", s.RideID, rf.Path)
	return r
}

func elevatorFailureConfigured(c *ElevatorFile, kind string) bool {
	f := c.Failures
	switch kind {
	case FailBriefStop:
		return f.BriefStop != nil
	case FailLongDowntime:
		return f.LongDowntime != nil
	case FailSideDown:
		return f.SideDown != nil
	}
	return false
}
