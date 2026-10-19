package config

import (
	"fmt"
	"math"
	"strings"
)

// arrivalShareTolerance is how far a multi-gate park's entrance shares may sum from 1.
const arrivalShareTolerance = 1e-6

// checkGates: one entrance and one exit need nothing more; with several entrances (M17, EPCOT) every entrance gives
// an arrival share (summing to 1) and the exit its parties leave by, and every exit is someone's.
func checkGates(p *ParkFile) (bool, string) {
	kinds := map[string]string{}
	var entrances []ParkNode
	exits := 0
	for _, n := range p.Nodes {
		kinds[n.ID] = n.Kind
		switch n.Kind {
		case NodeEntrance:
			entrances = append(entrances, n)
		case NodeExit:
			exits++
		}
	}
	if len(entrances) == 1 && exits == 1 {
		if e := entrances[0]; e.ArrivalShare != 0 || e.Exit != "" {
			return false, "arrival_share and exit are for parks with several entrances"
		}
		return true, "one entrance, one exit"
	}
	var bad []string
	sum := 0.0
	used := map[string]bool{}
	for _, e := range entrances {
		if e.ArrivalShare <= 0 {
			bad = append(bad, fmt.Sprintf("%s: arrival_share must be > 0", e.ID))
		}
		sum += e.ArrivalShare
		if kinds[e.Exit] != NodeExit {
			bad = append(bad, fmt.Sprintf("%s: exit %q is not an exit node", e.ID, e.Exit))
		}
		used[e.Exit] = true
	}
	if math.Abs(sum-1) > arrivalShareTolerance {
		bad = append(bad, fmt.Sprintf("entrance arrival shares sum to %g, want 1", sum))
	}
	for _, n := range p.Nodes {
		if n.Kind == NodeExit && !used[n.ID] {
			bad = append(bad, fmt.Sprintf("exit %s is no entrance's exit", n.ID))
		}
		if n.Kind != NodeEntrance && (n.ArrivalShare != 0 || n.Exit != "") {
			bad = append(bad, fmt.Sprintf("%s: arrival_share and exit are for entrances", n.ID))
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%d entrances", len(entrances))
}

// hoursKinds are the activities that can keep their own hours.
var hoursKinds = map[string]bool{NodeRide: true, NodeRideSimple: true, NodeMeet: true, NodeDining: true, NodeShop: true}

// checkNodeHours: a node's hours lie inside the park's and open before they close.
func checkNodeHours(p *ParkFile) (bool, string) {
	var bad []string
	n := 0
	for _, nd := range p.Nodes {
		if nd.Hours == "" {
			continue
		}
		n++
		if !hoursKinds[nd.Kind] {
			bad = append(bad, fmt.Sprintf("%s: %s nodes cannot have hours", nd.ID, nd.Kind))
			continue
		}
		from, to, err := ParseClockWindow(nd.Hours)
		switch {
		case err != nil:
			bad = append(bad, fmt.Sprintf("%s: %v", nd.ID, err))
		case from >= to:
			bad = append(bad, fmt.Sprintf("%s: hours %s close before they open", nd.ID, nd.Hours))
		case from < float64(p.Park.OpenHour*3600) || to > float64(p.Park.CloseHour*3600):
			bad = append(bad, fmt.Sprintf("%s: hours %s outside the park's %02d:00-%02d:00", nd.ID, nd.Hours, p.Park.OpenHour, p.Park.CloseHour))
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%d node(s) with their own hours", n)
}
