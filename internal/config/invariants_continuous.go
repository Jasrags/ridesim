package config

import (
	"fmt"
	"math"
	"strings"
)

const (
	routeDurationToleranceS = 5.0
	// loopTolerancePct bounds the spacing and loop-time cross-checks (warn level; timings are map-scaled).
	loopTolerancePct = 15.0
)

var continuousChecks = []check[*ContinuousFile]{
	{"continuous.route_structure", SevError, checkRouteStructure},
	{"continuous.route_duration", SevError, checkRouteDuration},
	{"continuous.spacing_vs_route", SevWarn, checkSpacingVsRoute},
	{"continuous.loop_time", SevWarn, checkLoopTime},
	{"continuous.capacity_ceiling", SevError, checkCapacityCeiling},
	{"continuous.seats", SevError, checkSeats},
	{"continuous.party_policy", SevError, checkPartyPolicy},
	{"continuous.stages", SevError, func(c *ContinuousFile) (bool, string) {
		return checkStageChain(c.Preshow, c.Stages, c.Ride.ListedCapacityPerHr)
	}},
}

var validRouteKinds = map[string]bool{RouteLoad: true, RouteScene: true, RouteUnload: true, RouteReturn: true}

// Exactly one load and one unload step, valid kinds, unique ids, positive timings, and every
// accessibility freeze zone names a load or unload step.
func checkRouteStructure(c *ContinuousFile) (bool, string) {
	var bad []string
	ids := map[string]string{}
	count := map[string]int{}
	for _, s := range c.Route {
		if !validRouteKinds[s.Kind] {
			bad = append(bad, fmt.Sprintf("%s: unknown kind %q", s.ID, s.Kind))
		}
		if _, dup := ids[s.ID]; dup {
			bad = append(bad, "duplicate route id "+s.ID)
		}
		if s.EstS <= 0 {
			bad = append(bad, s.ID+": est_s must be > 0")
		}
		ids[s.ID] = s.Kind
		count[s.Kind]++
	}
	if count[RouteLoad] != 1 || count[RouteUnload] != 1 {
		bad = append(bad, fmt.Sprintf("need exactly one load and one unload step, have %d and %d", count[RouteLoad], count[RouteUnload]))
	}
	for _, z := range c.LoadingShared.AccessibilityFreezeZones {
		if k := ids[z]; k != RouteLoad && k != RouteUnload {
			bad = append(bad, fmt.Sprintf("accessibility freeze zone %q is not a load/unload route step", z))
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%d steps: 1 load, %d scenes, 1 unload, %d return", len(c.Route), count[RouteScene], count[RouteReturn])
}

func routeSum(c *ContinuousFile, includeReturn bool) float64 {
	sum := 0.0
	for _, s := range c.Route {
		if s.Kind != RouteReturn || includeReturn {
			sum += s.EstS
		}
	}
	return sum
}

func checkRouteDuration(c *ContinuousFile) (bool, string) {
	sum := routeSum(c, false)
	return approxEqual(sum, c.Ride.RideDurationS, routeDurationToleranceS),
		fmt.Sprintf("sum(est_s, load+scene+unload) = %g s vs ride_duration_s %g s (+/- %g)", sum, c.Ride.RideDurationS, routeDurationToleranceS)
}

func pctOff(got, ref float64) float64 {
	if ref == 0 {
		return math.Inf(1)
	}
	return math.Abs(got-ref) / ref * 100
}

// Known data question on Haunted Mansion: 160 x 3.6 = 576 s vs a 473 s route, ~22% off.
func checkSpacingVsRoute(c *ContinuousFile) (bool, string) {
	loop := float64(c.Vehicles.Count) * c.Vehicles.SpacingS
	route := routeSum(c, true)
	off := pctOff(loop, route)
	return off <= loopTolerancePct, fmt.Sprintf("count x spacing = %d x %g = %.0f s vs sum(route.est_s) = %.0f s: %.1f%% off (limit %g%%); implied spacing %.2f s",
		c.Vehicles.Count, c.Vehicles.SpacingS, loop, route, off, loopTolerancePct, route/float64(c.Vehicles.Count))
}

// The data defines loop_time_s as count x spacing and says it "should land near total_experience_s +
// load/unload dwell". That reference includes the preshow, which is not on the loop, so this check is
// informational and kept at warn.
func checkLoopTime(c *ContinuousFile) (bool, string) {
	loop := float64(c.Vehicles.Count) * c.Vehicles.SpacingS
	ref := c.Ride.TotalExperienceS + c.LoadZone.LoadWindowS.Mean + c.UnloadZone.UnloadWindowS.Mean
	off := pctOff(loop, ref)
	return off <= loopTolerancePct, fmt.Sprintf("loop_time_s %.0f s vs total_experience_s + load + unload windows = %.0f s: %.1f%% off (limit %g%%)",
		loop, ref, off, loopTolerancePct)
}

func checkCapacityCeiling(c *ContinuousFile) (bool, string) {
	if c.Vehicles.SpacingS <= 0 {
		return false, "vehicles.spacing_s must be > 0"
	}
	if c.Conveyor.Chains < 0 {
		return false, fmt.Sprintf("conveyor.chains %d must be >= 0", c.Conveyor.Chains)
	}
	n := c.Conveyor.ChainCount()
	ceiling := float64(n) * 3600 / c.Vehicles.SpacingS * float64(c.Vehicles.SeatsMax)
	// The ride must reach what it is built to: theoretical_riders_per_hr_at_target when set (a ride built to
	// 90% of its listed figure, Jason's M9 rule), else the listed capacity.
	want, what := c.Ride.ListedCapacityPerHr, "listed"
	if t := c.ValidationTargets.TheoreticalRidersPerHrAtTarget; t > 0 {
		want, what = t, "target"
	}
	return want <= ceiling,
		fmt.Sprintf("%s %.0f/hr <= %d chain(s) x 3600 / %g s x %d seats = %.0f/hr", what, want, n, c.Vehicles.SpacingS, c.Vehicles.SeatsMax, ceiling)
}

func checkSeats(c *ContinuousFile) (bool, string) {
	v := c.Vehicles
	ok := v.Count > 0 && v.SeatsNominal > 0 && v.SeatsNominal <= v.SeatsMax && v.OccupancyTarget.Mean <= float64(v.SeatsMax)
	return ok, fmt.Sprintf("count %d, seats %d nominal / %d max, occupancy %.1f", v.Count, v.SeatsNominal, v.SeatsMax, v.OccupancyTarget.Mean)
}

// Parties larger than a vehicle must use contiguous vehicles (both policies do). The ride file does not
// know party sizes, so a policy is required outright.
func checkPartyPolicy(c *ContinuousFile) (bool, string) {
	p, ls := c.LoadingShared.PartyVehiclePolicy, c.LoadingShared
	if ls.PartySeats < 0 || ls.PartySeats > c.Vehicles.SeatsMax || ls.CalloutParties < 0 {
		return false, fmt.Sprintf("party_seats %d must be 0..seats_max %d and callout_parties %d >= 0", ls.PartySeats, c.Vehicles.SeatsMax, ls.CalloutParties)
	}
	if sd := c.Failures.SideDown; sd != nil && (c.Conveyor.ChainCount() < 2 || sd.RatePerHr < 0 || sd.DurationS.Mean <= 0) {
		return false, "failures.side_down needs conveyor.chains >= 2, rate_per_hr >= 0 and a positive duration_s"
	}
	if ps := c.Preshow; ps != nil && ps.BypassForSingleRider && !c.Queues.SingleRider.Enabled {
		return false, "preshow.bypass_for_single_rider needs an enabled single_rider line"
	}
	if p != SharedVehicles && (ls.PartySeats > 0 || ls.CalloutParties > 0) {
		return false, "party_seats and callout_parties apply only to party_vehicle_policy shared"
	}
	return p == ContiguousVehicles || p == SharedVehicles, fmt.Sprintf("party_vehicle_policy %q (want %q or %q)", p, ContiguousVehicles, SharedVehicles)
}
