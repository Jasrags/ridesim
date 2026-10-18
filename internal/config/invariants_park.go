package config

import (
	"fmt"
	"math"
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
	{"park.decision", SevError, checkDecision},
	{"park.lightning_lane", SevError, checkParkLightningLane},
	{"park.map_positions", SevError, checkMapPositions},
}

// checkMapPositions: map coordinates are optional, but all nodes have both x and y or none do.
func checkMapPositions(p *ParkFile) (bool, string) {
	with := 0
	var bad []string
	for _, n := range p.Nodes {
		switch {
		case n.X != nil && n.Y != nil:
			with++
		case n.X != nil || n.Y != nil:
			bad = append(bad, n.ID)
		}
	}
	if len(bad) > 0 || (with != 0 && with != len(p.Nodes)) {
		return false, fmt.Sprintf("%d of %d nodes have x and y; give every node both or none (half set: %s)", with, len(p.Nodes), strings.Join(bad, ", "))
	}
	if with == 0 {
		return true, "no map positions (the front end lays the graph out itself)"
	}
	return true, fmt.Sprintf("all %d nodes have map positions", with)
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

// RideSimpleIntervalS is the ride_simple dispatch interval [assumed]; capacity is spread over it.
const RideSimpleIntervalS = 60

func checkNodeFields(p *ParkFile) (bool, string) {
	var bad []string
	maxParty := 0
	for s := range p.Guests.PartySize {
		maxParty = max(maxParty, s)
	}
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
			} else if seats := int(math.Max(1, math.Round(n.CapacityPerHr*RideSimpleIntervalS/3600))); seats < maxParty {
				bad = append(bad, fmt.Sprintf("%s: %d seats per %d s vehicle cannot take a party of %d (parties never split)", n.ID, seats, RideSimpleIntervalS, maxParty))
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
		case NodeJunction:
		case NodeMeet:
			if n.MeetS == nil || n.MeetS.Mean <= 0 {
				miss = append(miss, "meet_s")
			}
		case NodeElsewhere, NodeShop:
			if n.VisitMin == nil || n.VisitMin.Mean <= 0 {
				miss = append(miss, "visit_min")
			}
		case NodeDining:
			if n.Seats <= 0 {
				miss = append(miss, "seats")
			} else if n.Seats < maxParty {
				bad = append(bad, fmt.Sprintf("%s: %d seats cannot take a party of %d", n.ID, n.Seats, maxParty))
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
	queues := map[string]bool{}
	for _, n := range p.Nodes {
		queues[n.ID] = n.Kind == NodeRide || n.Kind == NodeRideSimple
	}
	for _, id := range p.Congestion.Spillback.AppliesTo {
		e, ok := edgeByID(p, id)
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("spillback.applies_to %q", id))
		case queues[e.From] == queues[e.To]:
			bad = append(bad, fmt.Sprintf("spillback.applies_to %q must touch exactly one ride node (whose queue spills)", id))
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

func edgeByID(p *ParkFile, id string) (ParkEdge, bool) {
	for _, e := range p.Edges {
		if e.ID == id {
			return e, true
		}
	}
	return ParkEdge{}, false
}

// Decision method and choice set names.
const (
	DecisionSoftmaxUtility = "softmax_utility"
	ChoiceTopK             = "top_k_by_utility"
)

func checkDecision(p *ParkFile) (bool, string) {
	d := p.Decision
	var bad []string
	if d.Method != DecisionSoftmaxUtility || d.ChoiceSet != ChoiceTopK {
		bad = append(bad, fmt.Sprintf("method %q / choice_set %q (supported: %s / %s)", d.Method, d.ChoiceSet, DecisionSoftmaxUtility, ChoiceTopK))
	}
	if d.RerouteS < 0 || p.Congestion.MinCapacityLengthFt < 0 {
		bad = append(bad, fmt.Sprintf("reroute_s %g and congestion.min_capacity_length_ft %g must be >= 0", d.RerouteS, p.Congestion.MinCapacityLengthFt))
	}
	if d.Temperature <= 0 || d.K < 1 || d.PostedWaitRefreshS <= 0 {
		bad = append(bad, fmt.Sprintf("temperature %g, k %d, posted_wait_refresh_s %g must all be > 0", d.Temperature, d.K, d.PostedWaitRefreshS))
	}
	for _, w := range d.MealWindows {
		if from, to, err := ParseClockWindow(w); err != nil || from >= to {
			bad = append(bad, fmt.Sprintf("meal window %q must be HH:MM-HH:MM, start before end", w))
		}
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("softmax over top %d, temperature %g, %d meal window(s)", d.K, d.Temperature, len(d.MealWindows))
}

// ParseClockWindow parses "HH:MM-HH:MM" into seconds after midnight.
func ParseClockWindow(s string) (from, to float64, err error) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("window %q: want HH:MM-HH:MM", s)
	}
	if from, err = ParseClock(a); err != nil {
		return 0, 0, err
	}
	to, err = ParseClock(b)
	return from, to, err
}

// ParseClock parses a wall-clock "HH:MM" into seconds after midnight.
func ParseClock(s string) (float64, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hh < 0 || hh > 24 || mm < 0 || mm > 59 {
		return 0, fmt.Errorf("clock %q: want HH:MM", s)
	}
	return float64(hh*3600 + mm*60), nil
}

func checkParkLightningLane(p *ParkFile) (bool, string) {
	ll := p.LightningLane
	if ll == nil {
		if p.HasLLTiers() {
			return false, "nodes declare lightning_lane tiers but there is no lightning_lane section"
		}
		return true, "no lightning_lane section: park runs have no Lightning Lane"
	}
	var bad []string
	for name, v := range map[string]float64{"purchase_share": ll.PurchaseShare, "inventory_share": ll.InventoryShare, "elsewhere_share": ll.ElsewhereShare} {
		if v < 0 || v > 1 {
			bad = append(bad, fmt.Sprintf("%s %g must be in [0, 1]", name, v))
		}
	}
	if ll.WindowMin <= 0 || ll.GraceMin < 0 || ll.AdvanceBookings < 0 {
		bad = append(bad, fmt.Sprintf("window_min %g > 0, grace_min %g >= 0, advance_bookings %d >= 0", ll.WindowMin, ll.GraceMin, ll.AdvanceBookings))
	}
	bad = append(bad, checkLLTiers(p)...)
	if len(bad) > 0 {
		sort.Strings(bad)
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("%.0f%% buy, %d in advance, %g-min windows, %.0f%% of capacity sold", ll.PurchaseShare*100, ll.AdvanceBookings, ll.WindowMin, ll.InventoryShare*100)
}

// checkLLTiers checks node tiers and the tier settings they need.
func checkLLTiers(p *ParkFile) []string {
	if !p.HasLLTiers() {
		return nil
	}
	var bad []string
	for _, n := range p.Nodes {
		switch n.LightningLane {
		case "":
		case LLTier1, LLTier2, LLSinglePass:
			if n.Kind != NodeRide && n.Kind != NodeRideSimple {
				bad = append(bad, fmt.Sprintf("%s: lightning_lane on a %s node (rides only)", n.ID, n.Kind))
			}
			// Lightning Lane takes at most 2/3 of a simple ride's vehicle; the largest party must fit in that.
			if seats := int(math.Max(1, math.Round(n.CapacityPerHr*RideSimpleIntervalS/3600))); n.Kind == NodeRideSimple && seats*2/3 < maxPartySize(p) {
				bad = append(bad, fmt.Sprintf("%s: Lightning Lane may take %d of %d seats per vehicle, less than the largest party (%d)", n.ID, seats*2/3, seats, maxPartySize(p)))
			}
		default:
			bad = append(bad, fmt.Sprintf("%s: lightning_lane %q (want tier1, tier2 or single_pass)", n.ID, n.LightningLane))
		}
	}
	ll := p.LightningLane
	if len(ll.AdvanceByTier) == 0 {
		bad = append(bad, "nodes declare tiers: advance_by_tier is required")
	}
	for k, v := range ll.AdvanceByTier {
		if (k != LLTier1 && k != LLTier2) || v < 0 {
			bad = append(bad, fmt.Sprintf("advance_by_tier %s: %d (keys tier1, tier2; counts >= 0)", k, v))
		}
	}
	if ll.SinglePassShare < 0 || ll.SinglePassShare > 1 {
		bad = append(bad, fmt.Sprintf("single_pass_share %g must be in [0, 1]", ll.SinglePassShare))
	}
	return bad
}

func maxPartySize(p *ParkFile) int {
	m := 0
	for s := range p.Guests.PartySize {
		m = max(m, s)
	}
	return m
}
