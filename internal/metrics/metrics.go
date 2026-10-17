// Package metrics derives run metrics from an event log only (spec §3: everything observable is an
// event; metrics come from the log). Feeding it a log read back from disk replays the run.
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/jasrags/ridesim/internal/engine"
)

// Stats summarizes a sample in seconds.
type Stats struct {
	N                        int
	Mean, P50, P95, Min, Max float64
}

func summarize(xs []float64) Stats {
	if len(xs) == 0 {
		return Stats{}
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	sum := 0.0
	for _, x := range s {
		sum += x
	}
	return Stats{N: len(s), Mean: sum / float64(len(s)), P50: pct(s, 50), P95: pct(s, 95), Min: s[0], Max: s[len(s)-1]}
}

// pct is the nearest-rank percentile of sorted s.
func pct(s []float64, p float64) float64 {
	rank := int(math.Ceil(p / 100 * float64(len(s))))
	return s[max(0, min(len(s)-1, rank-1))]
}

// BlockHolds is hold time at one block during the window.
type BlockHolds struct {
	Block   string
	Brake   bool
	Passes  int // trains entering the block in the window
	Holds   int // trains held at least once on that pass (hold segments started in the window)
	TotalS  float64
	MeanS   float64 // per hold
	MaxS    float64
	Reasons map[string]int
	Yields  int // hold_until_clear released early to avoid a circular wait
}

// Little compares time-average queue length L with arrival rate times mean wait.
type Little struct {
	L       float64 // guests, time average over the window
	Lambda  float64 // guests per second
	W       float64 // seconds, guests who joined in the window and boarded
	LambdaW float64
	ErrPct  float64 // |L - lambda W| / L
}

// Report is everything M1 measures.
type Report struct {
	Run           map[string]string // run_start parameters
	WindowStart   engine.Time
	WindowEnd     engine.Time
	Dispatches    int
	Interval      Stats // realized dispatch interval
	Riders        int
	RidersPerHr   float64
	Wait          Stats // standby wait of guests who joined in the window and boarded
	Unserved      int   // joined in the window, still waiting at the end
	QueueMax      int
	Balks         int                // guests who left on arrival
	Reneges       int                // guests who left the outdoor line
	Lanes         map[string]int     // guests boarded in the window, by lane
	LaneWait      map[string]Stats   // wait by lane, guests who joined in the window and boarded
	LLShare       float64            // realized Lightning Lane share of riders (spec §8: an output)
	PhaseS        map[string]float64 // time in each merge phase during the window
	Holds         []BlockHolds       // every block with a pass in the window, in first-seen order
	Little        Little
	LoadIdleS     float64 // time the load station sat empty during the window
	PostedMean    float64 // min, time-weighted posted wait over the window (0 when nothing is posted)
	PostedPeak    float64 // min, highest clock-hour mean posted wait
	StationBlocks []string
	Ops           Ops

	joinTimes map[int64]engine.Time // every guest's join time, for computeOps
	brakes    map[string]bool
}

// Compute builds the report for the window [run start + warmup, run end].
func Compute(events []engine.Event, warmup engine.Duration) (*Report, error) {
	if len(events) == 0 || events[0].Type != engine.EvRunStart {
		return nil, fmt.Errorf("metrics: log must begin with %s", engine.EvRunStart)
	}
	last := events[len(events)-1]
	if last.Type != engine.EvRunEnd {
		return nil, fmt.Errorf("metrics: log must end with %s", engine.EvRunEnd)
	}
	r := &Report{Run: parseInfo(events[0].Info), WindowStart: events[0].T.Add(warmup), WindowEnd: last.T}
	if r.WindowEnd <= r.WindowStart {
		return nil, fmt.Errorf("metrics: warmup %v leaves no window before run end %v", warmup, last.T)
	}
	brakes := map[string]bool{}
	for _, b := range strings.Split(r.Run["brakes"], ",") {
		brakes[b] = true
	}
	r.brakes, r.joinTimes, r.Lanes = brakes, map[int64]engine.Time{}, map[string]int{}
	r.LaneWait, r.PhaseS = map[string]Stats{}, map[string]float64{}
	laneWaits := map[string][]float64{}
	phase, phaseSince := r.Run["ll_phase"], r.WindowStart
	var littleSum float64
	var littleN int
	loadBlock := r.Run["load"]
	in := func(t engine.Time) bool { return t >= r.WindowStart && t <= r.WindowEnd }

	var (
		dispatchT []engine.Time
		waits     []float64
		joinedAt  = map[int64]engine.Time{}
		holds     = map[string]*BlockHolds{}
		order     []string
		openHold  = map[int]engine.Event{}
		q, joins  int
		area      float64
		lastT     = r.WindowStart
		loadBusy  bool
		loadIdle  engine.Duration
		idleFrom  = r.WindowStart
	)
	blockStats := func(id string) *BlockHolds {
		h, ok := holds[id]
		if !ok {
			h = &BlockHolds{Block: id, Brake: brakes[id], Reasons: map[string]int{}}
			holds[id] = h
			order = append(order, id)
		}
		return h
	}
	advance := func(t engine.Time) { // integrate queue length over the window up to t
		if t > r.WindowStart {
			from := max(lastT, r.WindowStart)
			if t > from {
				area += float64(q) * t.Sub(from).Seconds()
			}
			lastT = t
		}
	}

	for _, e := range events {
		advance(e.T)
		switch e.Type {
		case engine.EvQueueJoin:
			q += e.N
			r.QueueMax = max(r.QueueMax, q)
			joinedAt[e.Guest] = e.T
			r.joinTimes[e.Guest] = e.T
			if in(e.T) {
				joins += e.N
			}
		case engine.EvBoard, engine.EvRenege:
			q -= e.N
			if e.Type == engine.EvBoard && in(e.T) {
				r.Lanes[e.Info] += e.N // riders by lane: what was dispatched in the window
			}
			if j, ok := joinedAt[e.Guest]; ok {
				if in(j) {
					w := e.T.Sub(j).Seconds()
					littleSum += w * float64(e.N)
					littleN += e.N
					if e.Type == engine.EvBoard {
						for range e.N { // guest-weighted
							waits = append(waits, w)
							laneWaits[e.Info] = append(laneWaits[e.Info], w)
						}
					} else {
						r.Reneges += e.N
					}
				}
				delete(joinedAt, e.Guest)
			}
		case "merge_phase":
			if phase != "" && e.T > r.WindowStart {
				r.PhaseS[phase] += e.T.Sub(max(phaseSince, r.WindowStart)).Seconds()
			}
			phase, phaseSince = e.Info, e.T
		case engine.EvBalk:
			if in(e.T) {
				r.Balks += e.N
			}
		case engine.EvDispatch:
			if in(e.T) {
				dispatchT = append(dispatchT, e.T)
				r.Riders += e.N
			}
		case engine.EvTrainEnter:
			if in(e.T) {
				blockStats(e.Block).Passes++
			}
			if e.Block == loadBlock {
				if !loadBusy && e.T > idleFrom { // idleFrom is never before the window start
					loadIdle += e.T.Sub(idleFrom)
				}
				loadBusy = true
			}
		case engine.EvHoldStart:
			openHold[e.Train] = e
		case engine.EvHoldYield:
			if in(e.T) {
				blockStats(e.Block).Yields++
			}
		case engine.EvHoldEnd:
			st, ok := openHold[e.Train]
			if !ok {
				return nil, fmt.Errorf("metrics: hold_end without hold_start for train %d at %v", e.Train, e.T)
			}
			delete(openHold, e.Train)
			if d := e.T.Sub(st.T).Seconds(); in(st.T) && d > 0 { // zero-length holds are not holds
				h := blockStats(st.Block)
				h.Holds++
				h.TotalS += d
				h.MaxS = math.Max(h.MaxS, d)
				h.Reasons[st.Info]++
			}
		}
		if e.Type == engine.EvDispatch && e.Block == loadBlock {
			loadBusy = false
			idleFrom = max(e.T, r.WindowStart)
		}
	}
	if !loadBusy && r.WindowEnd > idleFrom {
		loadIdle += r.WindowEnd.Sub(idleFrom)
	}

	for _, id := range order {
		h := holds[id]
		if h.Holds > 0 {
			h.MeanS = h.TotalS / float64(h.Holds)
		}
		r.Holds = append(r.Holds, *h)
	}
	gaps := make([]float64, 0, len(dispatchT))
	for i := 1; i < len(dispatchT); i++ {
		gaps = append(gaps, dispatchT[i].Sub(dispatchT[i-1]).Seconds())
	}
	r.Interval = summarize(gaps)
	r.Dispatches = len(dispatchT)
	window := r.WindowEnd.Sub(r.WindowStart).Seconds()
	r.RidersPerHr = float64(r.Riders) / window * 3600
	r.PostedMean, r.PostedPeak = postedStats(events, r.WindowStart, r.WindowEnd)
	r.Wait = summarize(waits)
	for lane, ws := range laneWaits {
		r.LaneWait[lane] = summarize(ws)
	}
	if phase != "" {
		r.PhaseS[phase] += r.WindowEnd.Sub(max(phaseSince, r.WindowStart)).Seconds()
	}
	if r.Riders > 0 {
		r.LLShare = float64(r.Lanes["lightning_lane"]) / float64(r.Riders)
	}
	for _, j := range joinedAt {
		if in(j) {
			r.Unserved++
		}
	}
	r.LoadIdleS = loadIdle.Seconds()

	r.Little.L = area / window
	r.Little.Lambda = float64(joins) / window
	if littleN > 0 {
		r.Little.W = littleSum / float64(littleN) // boarded and reneged guests
	}
	r.Little.LambdaW = r.Little.Lambda * r.Little.W
	if r.Little.L > 0 {
		r.Little.ErrPct = math.Abs(r.Little.L-r.Little.LambdaW) / r.Little.L * 100
	}
	computeOps(events, r)
	return r, nil
}

// parseInfo splits "k=v k=v" into a map.
func parseInfo(s string) map[string]string {
	m := map[string]string{}
	for _, f := range strings.Fields(s) {
		if k, v, ok := strings.Cut(f, "="); ok {
			m[k] = v
		}
	}
	return m
}

// Write renders the report as text.
func (r *Report) Write(w io.Writer) error {
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

	fmt.Fprintf(&b, "dispatches        %d\n", r.Dispatches)
	fmt.Fprintf(&b, "dispatch interval mean %.2f s  p50 %.2f  p95 %.2f  min %.2f  max %.2f\n", r.Interval.Mean, r.Interval.P50, r.Interval.P95, r.Interval.Min, r.Interval.Max)
	fmt.Fprintf(&b, "throughput        %.0f riders/hr (%d riders)\n", r.RidersPerHr, r.Riders)
	fmt.Fprintf(&b, "load station idle %.0f s (%.1f%% of window)\n", r.LoadIdleS, r.LoadIdleS/r.WindowEnd.Sub(r.WindowStart).Seconds()*100)
	fmt.Fprintf(&b, "standby wait      mean %.1f min  p50 %.1f  p95 %.1f  (n=%d, unserved at end %d, max queue %d)\n",
		r.Wait.Mean/60, r.Wait.P50/60, r.Wait.P95/60, r.Wait.N, r.Unserved, r.QueueMax)
	fmt.Fprintf(&b, "little's law      L %.1f guests vs lambda*W %.4f/s x %.1f s = %.1f  (%.2f%% apart)\n\n",
		r.Little.L, r.Little.Lambda, r.Little.W, r.Little.LambdaW, r.Little.ErrPct)

	fmt.Fprintf(&b, "%-18s %-5s %7s %6s %9s %7s %7s %6s  %s\n", "block", "brake", "passes", "holds", "total s", "mean s", "max s", "yields", "reasons")
	for _, h := range r.Holds {
		brake := ""
		if h.Brake {
			brake = "yes"
		}
		reasons := make([]string, 0, len(h.Reasons))
		for k, v := range h.Reasons {
			reasons = append(reasons, fmt.Sprintf("%s=%d", k, v))
		}
		sort.Strings(reasons)
		fmt.Fprintf(&b, "%-18s %-5s %7d %6d %9.0f %7.1f %7.1f %6d  %s\n", h.Block, brake, h.Passes, h.Holds, h.TotalS, h.MeanS, h.MaxS, h.Yields, strings.Join(reasons, " "))
	}
	lanes := make([]string, 0, len(r.Lanes))
	for k, v := range r.Lanes {
		lanes = append(lanes, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(lanes)
	fmt.Fprintf(&b, "\nriders by lane    %s\nabandonment       %d balked, %d reneged\n", strings.Join(lanes, " "), r.Balks, r.Reneges)
	if r.Lanes["lightning_lane"] > 0 || len(r.PhaseS) > 1 {
		ll, sb := r.LaneWait["lightning_lane"], r.LaneWait["standby"]
		fmt.Fprintf(&b, "lightning lane    share %.1f%% of riders; wait LL %.1f min (p95 %.1f), standby %.1f min (p95 %.1f)\n",
			r.LLShare*100, ll.Mean/60, ll.P95/60, sb.Mean/60, sb.P95/60)
		phases := make([]string, 0, len(r.PhaseS))
		for k, v := range r.PhaseS {
			phases = append(phases, fmt.Sprintf("%s=%.0f%%", k, v/r.WindowEnd.Sub(r.WindowStart).Seconds()*100))
		}
		sort.Strings(phases)
		fmt.Fprintf(&b, "merge phases      %s\n", strings.Join(phases, " "))
	}
	r.Ops.write(&b, r.RidersPerHr)
	_, err := io.WriteString(w, b.String())
	return err
}
