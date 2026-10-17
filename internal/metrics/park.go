package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/park"
)

// ParkReport covers a park day (M7, spec §10 "Park"): per-node reports from each scope's events, the
// in-park population, time per activity, edge congestion, Lightning Lane issuance, show-exit surges and
// posted waits against the Queue-Times calibration targets.
type ParkReport struct {
	Run         map[string]string
	WindowStart engine.Time
	WindowEnd   engine.Time

	Nodes []NodeReport // in run_start order

	Arrived, Left int // guests
	PeakInPark    int
	PeakAt        engine.Time
	MeanInPark    float64
	StateShare    map[string]float64 // share of in-park guest time by state (5-min snapshots)

	Edges []EdgeReport

	LLBooked, LLRedeemed, LLLapsed int // bookings (parties)

	ShowExits []ShowExit
	Targets   []Target
}

// NodeReport is one activity's metrics.
type NodeReport struct {
	Node, Kind string
	Report     *Report
}

// EdgeReport is congestion on one walkway: guests on it relative to its comfortable capacity C.
type EdgeReport struct {
	ID        string
	C         int
	Peak      int     // most guests on the edge at once
	MeanRatio float64 // time-weighted n / C over the window
	OverS     float64 // time with n > C
	Blocked   int     // parties that found it at jam density
}

// ShowExit compares joins at the exit target in the 15 minutes before and after a show lets out.
type ShowExit struct {
	Show, Target            string
	At                      engine.Time
	Audience                int
	JoinsBefore, JoinsAfter int     // guests joining the target's queues
	PostedBefore            float64 // min, last posted wait before the show ended
	PostedAfter             float64 // min, highest posted wait in the 30 minutes after
}

// Target compares a ride's posted waits with its Queue-Times calibration target ([fan]).
type Target struct {
	Node                  string
	AvgMin, PeakMin       float64 // target
	SimAvgMin, SimPeakMin float64
}

const showExitWindow = 15 * engine.Minute

// IsParkLog reports whether events are a park run's log.
func IsParkLog(events []engine.Event) bool {
	return len(events) > 0 && events[0].Type == engine.EvRunStart && events[0].Src == "" && parseInfo(events[0].Info)["park"] != ""
}

// ComputePark builds the park report for the window [start + warmup, run end].
func ComputePark(events []engine.Event, warmup engine.Duration) (*ParkReport, error) {
	if !IsParkLog(events) {
		return nil, fmt.Errorf("metrics: not a park log")
	}
	last := events[len(events)-1]
	if last.Type != engine.EvRunEnd {
		return nil, fmt.Errorf("metrics: log must end with %s", engine.EvRunEnd)
	}
	r := &ParkReport{Run: parseInfo(events[0].Info), WindowStart: events[0].T.Add(warmup), WindowEnd: last.T, StateShare: map[string]float64{}}
	if r.WindowEnd <= r.WindowStart {
		return nil, fmt.Errorf("metrics: warmup %v leaves no measurement window", warmup)
	}
	// Split the log by scope; every activity scope starts with run_start.
	var order []string
	byScope := map[string][]engine.Event{}
	for _, e := range events {
		if e.Src == "" || e.Src == park.Scope {
			continue
		}
		if _, ok := byScope[e.Src]; !ok {
			order = append(order, e.Src)
		}
		byScope[e.Src] = append(byScope[e.Src], e)
	}
	end := engine.Event{T: last.T, Type: engine.EvRunEnd}
	for _, src := range order {
		evs := append(byScope[src], end)
		rep, err := Compute(evs, warmup)
		if err != nil {
			return nil, fmt.Errorf("metrics: node %s: %w", src, err)
		}
		r.Nodes = append(r.Nodes, NodeReport{Node: src, Kind: rep.Run["archetype"], Report: rep})
	}
	r.parkLevel(events)
	r.showExits(byScope)
	r.targets()
	return r, nil
}

// postedStats is the time-weighted mean posted wait over the window and the highest clock-hour mean, in
// minutes. Each posted value holds until the next.
func postedStats(evs []engine.Event, from, to engine.Time) (mean, peak float64) {
	type seg struct {
		at engine.Time
		v  float64
	}
	var segs []seg
	for _, e := range evs {
		if e.Type == engine.EvPostedWait {
			segs = append(segs, seg{e.T, float64(e.N) / 60})
		}
	}
	if len(segs) == 0 {
		return 0, 0
	}
	// integral of the step function over [a, b)
	area := func(a, b engine.Time) float64 {
		sum := 0.0
		for i, s := range segs {
			next := to
			if i+1 < len(segs) {
				next = segs[i+1].at
			}
			lo, hi := max(a, s.at), min(b, next)
			if hi > lo {
				sum += s.v * hi.Sub(lo).Seconds()
			}
		}
		return sum
	}
	mean = area(from, to) / to.Sub(from).Seconds()
	for h := from; h < to; h = h.Add(engine.Hour) {
		hi := min(h.Add(engine.Hour), to)
		if m := area(h, hi) / hi.Sub(h).Seconds(); m > peak {
			peak = m
		}
	}
	return mean, peak
}

func (r *ParkReport) in(t engine.Time) bool { return t >= r.WindowStart && t <= r.WindowEnd }

func (r *ParkReport) parkLevel(events []engine.Event) {
	type edgeAcc struct {
		rep    *EdgeReport
		deltas []delta
	}
	edges := map[string]*edgeAcc{}
	var edgeOrder []string
	snapGuests := 0.0
	stateGuests := map[string]float64{}
	snaps := 0
	for _, e := range events {
		if e.Src != park.Scope {
			continue
		}
		switch e.Type {
		case park.EvParkEdge:
			edges[e.Block] = &edgeAcc{rep: &EdgeReport{ID: e.Block, C: e.N}}
			edgeOrder = append(edgeOrder, e.Block)
		case park.EvWalk:
			if a := edges[e.Block]; a != nil {
				a.deltas = append(a.deltas, delta{e.T, e.N}, delta{e.T.Add(e.D), -e.N})
			}
		case park.EvEdgeBlocked:
			if a := edges[e.Block]; a != nil && r.in(e.T) {
				a.rep.Blocked++
			}
		case park.EvParkArrive:
			r.Arrived += e.N
		case park.EvParkLeave:
			r.Left += e.N
		case park.EvLLBook:
			r.LLBooked++
		case park.EvLLRedeem:
			r.LLRedeemed++
		case park.EvLLLapse:
			r.LLLapsed++
		case park.EvParkSnapshot:
			if e.N > r.PeakInPark {
				r.PeakInPark, r.PeakAt = e.N, e.T
			}
			if !r.in(e.T) {
				continue
			}
			snaps++
			snapGuests += float64(e.N)
			for k, v := range parseInfo(e.Info) {
				n, _ := strconv.Atoi(v)
				stateGuests[k] += float64(n)
			}
		}
	}
	if snaps > 0 {
		r.MeanInPark = snapGuests / float64(snaps)
	}
	for k, v := range stateGuests {
		if snapGuests > 0 {
			r.StateShare[k] = v / snapGuests
		}
	}
	for _, id := range edgeOrder {
		a := edges[id]
		occupancy(a.deltas, r.WindowStart, r.WindowEnd, a.rep)
		r.Edges = append(r.Edges, *a.rep)
	}
}

type delta struct {
	at engine.Time
	n  int
}

// occupancy turns enter/leave deltas into peak, time-weighted n/C and time over C within the window.
func occupancy(ds []delta, from, to engine.Time, rep *EdgeReport) {
	sort.SliceStable(ds, func(i, j int) bool { return ds[i].at < ds[j].at })
	n, area, over := 0, 0.0, 0.0
	prev := from
	flush := func(t engine.Time) {
		lo, hi := max(prev, from), min(t, to)
		if hi > lo {
			area += float64(n) * hi.Sub(lo).Seconds()
			if n > rep.C {
				over += hi.Sub(lo).Seconds()
			}
		}
		prev = t
	}
	for _, d := range ds {
		flush(d.at)
		n += d.n
		if n > rep.Peak && d.at <= to {
			rep.Peak = n
		}
	}
	flush(to)
	if rep.C > 0 {
		rep.MeanRatio = area / to.Sub(from).Seconds() / float64(rep.C)
	}
	rep.OverS = over
}

func (r *ParkReport) showExits(byScope map[string][]engine.Event) {
	var shows []string
	for src, evs := range byScope {
		if len(evs) > 0 && parseInfo(evs[0].Info)["archetype"] == "show" {
			shows = append(shows, src)
		}
	}
	sort.Strings(shows)
	for _, src := range shows {
		target := parseInfo(byScope[src][0].Info)["exit_to"]
		tev := byScope[target]
		if target == "" || len(tev) == 0 {
			continue
		}
		for _, e := range byScope[src] {
			if e.Type != park.EvShowEnd || !r.in(e.T) {
				continue
			}
			x := ShowExit{Show: src, Target: target, At: e.T, Audience: e.N, PostedBefore: -1}
			for _, te := range tev {
				switch {
				case te.Type == engine.EvQueueJoin && te.T >= e.T.Add(-showExitWindow) && te.T < e.T:
					x.JoinsBefore += te.N
				case te.Type == engine.EvQueueJoin && te.T >= e.T && te.T < e.T.Add(showExitWindow):
					x.JoinsAfter += te.N
				case te.Type == engine.EvPostedWait && te.T < e.T:
					x.PostedBefore = float64(te.N) / 60
				case te.Type == engine.EvPostedWait && te.T >= e.T && te.T < e.T.Add(2*showExitWindow):
					x.PostedAfter = math.Max(x.PostedAfter, float64(te.N)/60)
				}
			}
			r.ShowExits = append(r.ShowExits, x)
		}
	}
}

func (r *ParkReport) targets() {
	for _, t := range strings.Split(r.Run["targets"], ",") {
		parts := strings.Split(t, ":")
		if len(parts) != 3 {
			continue
		}
		avg, _ := strconv.ParseFloat(parts[1], 64)
		peak, _ := strconv.ParseFloat(parts[2], 64)
		tg := Target{Node: parts[0], AvgMin: avg, PeakMin: peak}
		for _, n := range r.Nodes {
			if n.Node == parts[0] {
				tg.SimAvgMin, tg.SimPeakMin = n.Report.PostedMean, n.Report.PostedPeak
			}
		}
		r.Targets = append(r.Targets, tg)
	}
}

// Node returns the report for a node, or nil.
func (r *ParkReport) Node(id string) *NodeReport {
	for i := range r.Nodes {
		if r.Nodes[i].Node == id {
			return &r.Nodes[i]
		}
	}
	return nil
}

// Write renders the park report as text.
func (r *ParkReport) Write(w io.Writer) error {
	var b strings.Builder
	keys := make([]string, 0, len(r.Run))
	for k := range r.Run {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("run:")
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%s", k, r.Run[k])
	}
	fmt.Fprintf(&b, "\nwindow: %v - %v (%.2f h)\n\n", r.WindowStart, r.WindowEnd, r.WindowEnd.Sub(r.WindowStart).Seconds()/3600)
	fmt.Fprintf(&b, "guests            arrived %d, left %d; in park peak %d at %v, mean %.0f\n", r.Arrived, r.Left, r.PeakInPark, r.PeakAt, r.MeanInPark)
	states := make([]string, 0, len(r.StateShare))
	for k := range r.StateShare {
		states = append(states, k)
	}
	sort.Strings(states)
	b.WriteString("time share       ")
	for _, k := range states {
		fmt.Fprintf(&b, " %s %.1f%%", k, r.StateShare[k]*100)
	}
	fmt.Fprintf(&b, "\nlightning lane    booked %d, redeemed %d, lapsed %d\n\n", r.LLBooked, r.LLRedeemed, r.LLLapsed)

	fmt.Fprintf(&b, "%-16s %-20s %8s %9s %8s %8s %8s %8s %6s %6s %6s\n", "node", "kind", "riders", "riders/hr", "wait", "p95", "posted", "peak hr", "LL %", "balks", "reneg")
	for _, n := range r.Nodes {
		m := n.Report
		fmt.Fprintf(&b, "%-16s %-20s %8d %9.0f %7.1fm %7.1fm %7.1fm %7.1fm %6.1f %6d %6d\n", n.Node, n.Kind, m.Riders, m.RidersPerHr,
			m.Wait.Mean/60, m.Wait.P95/60, m.PostedMean, m.PostedPeak, m.LLShare*100, m.Balks, m.Reneges)
	}
	fmt.Fprintf(&b, "\n%-24s %6s %6s %8s %8s %8s\n", "edge", "C", "peak", "mean n/C", "over C", "blocked")
	for _, e := range r.Edges {
		fmt.Fprintf(&b, "%-24s %6d %6d %8.3f %7.0fs %8d\n", e.ID, e.C, e.Peak, e.MeanRatio, e.OverS, e.Blocked)
	}
	if len(r.ShowExits) > 0 {
		fmt.Fprintf(&b, "\n%-10s %-16s %9s %8s %13s %13s\n", "show", "exit to", "at", "audience", "joins -/+15m", "posted -/+30m")
		for _, x := range r.ShowExits {
			fmt.Fprintf(&b, "%-10s %-16s %9v %8d %6d/%-6d %6.1f/%-6.1f\n", x.Show, x.Target, x.At, x.Audience, x.JoinsBefore, x.JoinsAfter, x.PostedBefore, x.PostedAfter)
		}
	}
	if len(r.Targets) > 0 {
		b.WriteString("\ncalibration [fan Queue-Times]  posted avg / peak hour (min)\n")
		for _, t := range r.Targets {
			fmt.Fprintf(&b, "  %-16s target %5.1f / %5.1f   sim %5.1f / %5.1f\n", t.Node, t.AvgMin, t.PeakMin, t.SimAvgMin, t.SimPeakMin)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
