package config

import (
	"fmt"
	"strings"
)

// durationToleranceS is the slack allowed between summed block transits and ride_duration_s.
const durationToleranceS = 2.0

var coasterChecks = []check[*CoasterFile]{
	{"coaster.block_kinds", SevError, checkBlockKinds},
	{"coaster.transit_present", SevError, checkTransitPresent},
	{"coaster.duration_sum", SevError, checkDurationSum},
	{"coaster.block_count", SevError, checkBlockCount},
	{"coaster.fleet_order", SevError, checkFleetOrder},
	{"coaster.brake_fields", SevError, checkBrakeFields},
	{"coaster.refs", SevError, checkCoasterRefs},
	{"coaster.loop_closed", SevError, checkLoopClosed},
	{"coaster.dispatch_interval_order", SevError, checkDispatchIntervalOrder},
	{"coaster.barn_capacity", SevError, checkBarnCapacity},
	{"coaster.dispatch_rule", SevError, checkDispatchRule},
	{"coaster.cascade_scope", SevError, checkCascadeScope},
	{"coaster.seat_rows", SevError, checkSeatRows},
}

func checkSeatRows(c *CoasterFile) (bool, string) {
	t := c.Trains
	ok := t.SeatsPerRow > 0 && t.SeatsPerTrain > 0 && t.SeatsPerTrain%t.SeatsPerRow == 0
	if !ok {
		return false, fmt.Sprintf("seats_per_train %d must be a positive multiple of seats_per_row %d", t.SeatsPerTrain, t.SeatsPerRow)
	}
	return true, fmt.Sprintf("%d rows of %d", t.SeatsPerTrain/t.SeatsPerRow, t.SeatsPerRow)
}

func checkCascadeScope(c *CoasterFile) (bool, string) {
	cs := c.Failures.CascadeStop
	if cs == nil {
		return true, "no cascade_stop"
	}
	ok := cs.Scope == "" || cs.Scope == CascadeScopeBrakesOnly || cs.Scope == CascadeScopeBrakesAndStations
	return ok && cs.BlockedToleranceS > 0, fmt.Sprintf("scope %q, blocked_tolerance_s %g", cs.Scope, cs.BlockedToleranceS)
}

func checkDispatchRule(c *CoasterFile) (bool, string) {
	r := c.Station.DispatchRule
	return validDispatchRules[r], fmt.Sprintf("dispatch_rule %q", r)
}

var validBlockKinds = map[string]bool{BlockStation: true, BlockLift: true, BlockLaunch: true, BlockCourse: true, BlockBrake: true}

func checkBlockKinds(c *CoasterFile) (bool, string) {
	var bad []string
	stations := 0
	for _, b := range c.Blocks {
		if !validBlockKinds[b.Kind] {
			bad = append(bad, fmt.Sprintf("%s: unknown kind %q", b.ID, b.Kind))
		}
		if b.Kind == BlockStation {
			stations++
		}
		if b.Kind == BlockLaunch && b.Launch == nil {
			bad = append(bad, fmt.Sprintf("%s: kind=launch without launch section", b.ID))
		}
		if b.HoldUntilClear && b.Kind != BlockBrake {
			bad = append(bad, fmt.Sprintf("%s: hold_until_clear only applies to brake blocks", b.ID))
		}
	}
	if stations == 0 {
		bad = append(bad, "no station block")
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%d blocks, %d station", len(c.Blocks), stations)
}

func checkTransitPresent(c *CoasterFile) (bool, string) {
	var bad []string
	for _, b := range c.Blocks {
		switch {
		case b.Kind != BlockStation && (b.TransitS == nil || b.TransitS.Mean <= 0):
			bad = append(bad, b.ID+": missing or non-positive transit_s")
		case b.Kind == BlockStation && b.TransitS != nil:
			bad = append(bad, b.ID+": station blocks take times from the station section, not transit_s")
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, "every non-station block has transit_s"
}

func transitSum(c *CoasterFile) float64 {
	sum := 0.0
	for _, b := range c.Blocks {
		if b.Kind != BlockStation && b.TransitS != nil {
			sum += b.TransitS.Mean
		}
	}
	return sum
}

func checkDurationSum(c *CoasterFile) (bool, string) {
	sum := transitSum(c)
	detail := fmt.Sprintf("sum(transit_s.mean) = %g s vs ride_duration_s %g s (+/- %g)", sum, c.Ride.RideDurationS, durationToleranceS)
	return approxEqual(sum, c.Ride.RideDurationS, durationToleranceS), detail
}

func checkBlockCount(c *CoasterFile) (bool, string) {
	need := c.Trains.MaxInService + 1
	return len(c.Blocks) >= need, fmt.Sprintf("%d blocks, need >= max_in_service+1 = %d", len(c.Blocks), need)
}

func checkFleetOrder(c *CoasterFile) (bool, string) {
	t := c.Trains
	ok := t.DefaultInService >= 1 && t.DefaultInService <= t.MaxInService && t.MaxInService <= t.FleetSize
	return ok, fmt.Sprintf("default %d <= max %d <= fleet %d", t.DefaultInService, t.MaxInService, t.FleetSize)
}

func checkBrakeFields(c *CoasterFile) (bool, string) {
	var bad []string
	n := 0
	for _, b := range c.Blocks {
		if b.Kind != BlockBrake {
			if b.Brake != nil {
				bad = append(bad, b.ID+": brake section on non-brake block")
			}
			continue
		}
		n++
		var missing []string
		switch {
		case b.Brake == nil:
			missing = []string{"brake"}
		default:
			if b.Brake.HoldCapable == nil {
				missing = append(missing, "hold_capable")
			}
			if b.Brake.StopPolicy == "" {
				missing = append(missing, "stop_policy")
			} else if b.Brake.StopPolicy != StopIfNextOccupied {
				bad = append(bad, fmt.Sprintf("%s: unknown stop_policy %q", b.ID, b.Brake.StopPolicy))
			}
			if b.Brake.ReleaseDelayS == nil {
				missing = append(missing, "release_delay_s")
			}
		}
		if len(missing) > 0 {
			bad = append(bad, fmt.Sprintf("%s: missing %s", b.ID, strings.Join(missing, ", ")))
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%d brake blocks complete", n)
}

func checkCoasterRefs(c *CoasterFile) (bool, string) {
	blocks := map[string]bool{}
	switches := map[string]bool{}
	var bad []string
	for _, b := range c.Blocks {
		if blocks[b.ID] {
			bad = append(bad, "duplicate block id "+b.ID)
		}
		blocks[b.ID] = true
		for _, s := range b.Contains {
			if switches[s] {
				bad = append(bad, "switch "+s+" in more than one block")
			}
			switches[s] = true
		}
	}
	for _, b := range c.Blocks {
		if !blocks[b.Next] {
			bad = append(bad, fmt.Sprintf("%s.next %q", b.ID, b.Next))
		}
	}
	ruleIDs := map[string]bool{}
	for _, r := range c.Clearance.Rules {
		if ruleIDs[r.ID] {
			bad = append(bad, "duplicate clearance rule "+r.ID)
		}
		ruleIDs[r.ID] = true
		if !blocks[r.From] {
			bad = append(bad, fmt.Sprintf("clearance %s.from %q", r.ID, r.From))
		}
		for _, id := range r.RequiresEmpty {
			if !blocks[id] {
				bad = append(bad, fmt.Sprintf("clearance %s.requires_empty %q", r.ID, id))
			}
		}
		for _, cond := range r.Conditions {
			switch cond.Kind {
			case CondSwitchAligned:
				if !switches[cond.Switch] {
					bad = append(bad, fmt.Sprintf("clearance %s: switch %q not in any block's contains", r.ID, cond.Switch))
				}
			case CondLoadClearToAdvance:
			default:
				bad = append(bad, fmt.Sprintf("clearance %s: unknown condition %q", r.ID, cond.Kind))
			}
		}
	}
	if len(switches) > 0 && c.Clearance.SwitchRealignS == nil {
		bad = append(bad, "blocks contain switches but clearance.switch_realign_s is missing")
	}
	if len(bad) > 0 {
		return false, "unresolved: " + strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%d blocks, %d clearance rules, %d switches resolve", len(c.Blocks), len(c.Clearance.Rules), len(switches))
}

// The track must be one closed loop through every block, starting at the first block.
func checkLoopClosed(c *CoasterFile) (bool, string) {
	if len(c.Blocks) == 0 {
		return false, "no blocks"
	}
	next := map[string]string{}
	for _, b := range c.Blocks {
		next[b.ID] = b.Next
	}
	start := c.Blocks[0].ID
	seen := map[string]bool{}
	cur := start
	path := []string{}
	for !seen[cur] {
		if _, ok := next[cur]; !ok {
			return false, fmt.Sprintf("loop breaks at unknown block %q after %s", cur, strings.Join(path, " -> "))
		}
		seen[cur] = true
		path = append(path, cur)
		cur = next[cur]
	}
	if cur != start || len(seen) != len(c.Blocks) {
		return false, fmt.Sprintf("path %s -> %s does not return to %s through all %d blocks", strings.Join(path, " -> "), cur, start, len(c.Blocks))
	}
	return true, strings.Join(path, " -> ") + " -> " + start
}

func checkDispatchIntervalOrder(c *CoasterFile) (bool, string) {
	s := c.Station
	return s.TargetDispatchIntervalS >= s.MinDispatchIntervalS && s.MinDispatchIntervalS > 0,
		fmt.Sprintf("target %g s >= min %g s", s.TargetDispatchIntervalS, s.MinDispatchIntervalS)
}

func checkBarnCapacity(c *CoasterFile) (bool, string) {
	spare := c.Trains.FleetSize - c.Trains.DefaultInService
	switch {
	case c.Barn == nil:
		return true, fmt.Sprintf("no barn: %d spare train(s) unavailable", spare)
	case c.Barn.StorageBays == nil:
		return true, fmt.Sprintf("storage_bays unknown: assume room for %d spare train(s)", spare)
	}
	return spare <= *c.Barn.StorageBays, fmt.Sprintf("%d spare train(s) vs %d bay(s)", spare, *c.Barn.StorageBays)
}
