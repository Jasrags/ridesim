package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const shareTolerance = 0.001

var parkChecks = []check[*ParkFile]{
	{"park.node_ids", SevError, checkNodeIDs},
	{"park.node_fields", SevError, checkNodeFields},
	{"park.edge_refs", SevError, checkEdgeRefs},
	{"park.connected", SevError, checkConnected},
	{"park.walking_mode", SevError, checkWalkingMode},
	{"park.hours", SevError, checkHours},
	{"park.arrival_shares", SevError, checkArrivalShares},
	{"park.party_sizes", SevError, checkPartySizes},
}

func checkNodeIDs(p *ParkFile) (bool, string) {
	seen := map[string]bool{}
	var dup []string
	for _, n := range p.Nodes {
		if seen[n.ID] {
			dup = append(dup, n.ID)
		}
		seen[n.ID] = true
	}
	for _, e := range p.Edges {
		if seen[e.ID] {
			dup = append(dup, e.ID)
		}
		seen[e.ID] = true
	}
	if len(dup) > 0 {
		return false, "duplicate ids: " + strings.Join(dup, ", ")
	}
	return true, fmt.Sprintf("%d nodes, %d edges, ids unique", len(p.Nodes), len(p.Edges))
}

func checkNodeFields(p *ParkFile) (bool, string) {
	var bad []string
	entrances, exits := 0, 0
	for _, n := range p.Nodes {
		var miss []string
		switch n.Kind {
		case NodeEntrance:
			entrances++
		case NodeExit:
			exits++
		case NodePlaza:
			if n.ComfortableGuests <= 0 {
				miss = append(miss, "comfortable_guests")
			}
		case NodeRide:
			if n.RideID == "" {
				miss = append(miss, "ride_id")
			}
		case NodeRideSimple:
			if n.CapacityPerHr <= 0 {
				miss = append(miss, "capacity_per_hr")
			}
			if n.DurationS <= 0 {
				miss = append(miss, "duration_s")
			}
		case NodeShow:
			if n.CapacityGuests <= 0 {
				miss = append(miss, "capacity_guests")
			}
			if n.DurationMin <= 0 {
				miss = append(miss, "duration_min")
			}
			if len(n.Schedule) == 0 {
				miss = append(miss, "schedule")
			}
		case NodeDining:
			if n.Seats <= 0 {
				miss = append(miss, "seats")
			}
			if n.ServiceTimeMin == nil {
				miss = append(miss, "service_time_min")
			}
		default:
			bad = append(bad, fmt.Sprintf("%s: unknown kind %q", n.ID, n.Kind))
		}
		if len(miss) > 0 {
			bad = append(bad, fmt.Sprintf("%s (%s): missing %s", n.ID, n.Kind, strings.Join(miss, ", ")))
		}
	}
	if entrances != 1 || exits != 1 {
		bad = append(bad, fmt.Sprintf("need one entrance and one exit, have %d and %d", entrances, exits))
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, "every node has the fields its kind needs"
}

func checkEdgeRefs(p *ParkFile) (bool, string) {
	nodes := map[string]bool{}
	for _, n := range p.Nodes {
		nodes[n.ID] = true
	}
	edges := map[string]bool{}
	var bad []string
	for _, e := range p.Edges {
		edges[e.ID] = true
		for _, end := range []string{e.From, e.To} {
			if !nodes[end] {
				bad = append(bad, fmt.Sprintf("edge %s: unknown node %q", e.ID, end))
			}
		}
		if e.LengthFt <= 0 || e.WidthFt <= 0 {
			bad = append(bad, fmt.Sprintf("edge %s: length and width must be > 0", e.ID))
		}
	}
	for _, n := range p.Nodes {
		if n.ExitEdge != "" && !edges[n.ExitEdge] {
			bad = append(bad, fmt.Sprintf("node %s: exit_edge %q", n.ID, n.ExitEdge))
		}
	}
	for _, id := range p.Congestion.Spillback.AppliesTo {
		if !edges[id] {
			bad = append(bad, fmt.Sprintf("spillback.applies_to %q", id))
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, "edge endpoints, exit_edge and spillback refs resolve"
}

// Every node must be reachable from the entrance (edges are bidirectional).
func checkConnected(p *ParkFile) (bool, string) {
	adj := map[string][]string{}
	start := ""
	for _, n := range p.Nodes {
		if n.Kind == NodeEntrance {
			start = n.ID
		}
	}
	for _, e := range p.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
	}
	seen := map[string]bool{start: true}
	stack := []string{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, nb := range adj[cur] {
			if !seen[nb] {
				seen[nb] = true
				stack = append(stack, nb)
			}
		}
	}
	var unreached []string
	for _, n := range p.Nodes {
		if !seen[n.ID] {
			unreached = append(unreached, n.ID)
		}
	}
	if start == "" || len(unreached) > 0 {
		return false, fmt.Sprintf("unreachable from entrance %q: %s", start, strings.Join(unreached, ", "))
	}
	return true, fmt.Sprintf("all %d nodes reachable from %s", len(p.Nodes), start)
}

func checkWalkingMode(p *ParkFile) (bool, string) {
	m := p.Park.WalkingMode
	return m == WalkingCongestion || m == WalkingFixedDelay, fmt.Sprintf("walking_mode %q", m)
}

func checkHours(p *ParkFile) (bool, string) {
	open, close := p.Park.OpenHour, p.Park.CloseHour
	if open < 0 || close > 24 || open >= close {
		return false, fmt.Sprintf("open %d, close %d", open, close)
	}
	for _, h := range sortedHours(p.Guests.ArrivalCurveShareByHour) {
		if h < open || h >= close {
			return false, fmt.Sprintf("arrival hour %d outside opening hours %d-%d", h, open, close)
		}
	}
	return true, fmt.Sprintf("open %02d:00-%02d:00, arrivals inside", open, close)
}

// sortedHours parses the hour keys and returns them ascending; unparseable keys become -1 so
// checkHours rejects them.
func sortedHours(m map[string]float64) []int {
	hours := make([]int, 0, len(m))
	for k := range m {
		h, err := strconv.Atoi(k)
		if err != nil {
			h = -1
		}
		hours = append(hours, h)
	}
	sort.Ints(hours)
	return hours
}

func checkArrivalShares(p *ParkFile) (bool, string) {
	sum := 0.0
	for _, h := range sortedHours(p.Guests.ArrivalCurveShareByHour) {
		sum += p.Guests.ArrivalCurveShareByHour[strconv.Itoa(h)]
	}
	return approxEqual(sum, 1, shareTolerance), fmt.Sprintf("arrival shares sum to %.3f", sum)
}

func checkPartySizes(p *ParkFile) (bool, string) {
	sizes := make([]int, 0, len(p.Guests.PartySize))
	for s := range p.Guests.PartySize {
		sizes = append(sizes, s)
	}
	sort.Ints(sizes)
	sum := 0.0
	for _, s := range sizes {
		if s <= 0 {
			return false, fmt.Sprintf("party size %d must be > 0", s)
		}
		sum += p.Guests.PartySize[s]
	}
	return approxEqual(sum, 1, shareTolerance), fmt.Sprintf("party size shares sum to %.3f (sizes %v)", sum, sizes)
}
