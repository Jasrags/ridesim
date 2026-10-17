package metrics

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/jasrags/ridesim/internal/engine"
)

// CauseDowntime is stopped time attributed to one cause inside the window. Overlapping causes each
// count their own time; Ops.StoppedS is the union.
type CauseDowntime struct {
	Cause  string
	Count  int // stops starting in the window
	TotalS float64
}

// PostedError compares the posted wait a guest saw at join with the wait they got.
type PostedError struct {
	N         int
	MeanAbsS  float64
	BiasS     float64 // mean(posted - true); positive = posted too high
	Algorithm string
}

// Ops covers M2: downtime, cascades, barn swaps, seat use, stacking, posted-wait accuracy.
type Ops struct {
	Downtime      []CauseDowntime // sorted by cause
	StoppedS      float64
	Availability  float64 // 1 - StoppedS / window
	Cascades      int
	CascadeBlocks map[string]int
	Swaps         int
	EmptySeats    Stats // per dispatch
	StackingS     float64
	Posted        PostedError
}

func computeOps(events []engine.Event, r *Report) {
	o := &r.Ops
	o.CascadeBlocks = map[string]int{}
	seats, _ := strconv.Atoi(r.Run["seats"])
	ws, we := r.WindowStart, r.WindowEnd
	in := func(t engine.Time) bool { return t >= ws && t <= we }
	clip := func(from, to engine.Time) float64 {
		a, b := max(from, ws), min(to, we)
		if b <= a {
			return 0
		}
		return b.Sub(a).Seconds()
	}

	depth := map[string]int{}
	since := map[string]engine.Time{}
	totals := map[string]*CauseDowntime{}
	active, stoppedFrom := 0, engine.Time(0)
	var empty, abs, bias []float64
	posted, postedAt := -1.0, map[int64]float64{}
	openHold := map[int]engine.Event{}

	for _, e := range events {
		switch e.Type {
		case engine.EvRideStop:
			cd := totals[e.Info]
			if cd == nil {
				cd = &CauseDowntime{Cause: e.Info}
				totals[e.Info] = cd
			}
			if in(e.T) {
				cd.Count++
			}
			if depth[e.Info] == 0 {
				since[e.Info] = e.T
			}
			depth[e.Info]++
			if active == 0 {
				stoppedFrom = e.T
			}
			active++
		case engine.EvRideResume:
			depth[e.Info]--
			if depth[e.Info] == 0 {
				totals[e.Info].TotalS += clip(since[e.Info], e.T)
			}
			active--
			if active == 0 {
				o.StoppedS += clip(stoppedFrom, e.T)
			}
		case engine.EvCascade:
			if in(e.T) {
				o.Cascades++
				o.CascadeBlocks[e.Block]++
			}
		case engine.EvBarnSwap:
			if in(e.T) {
				o.Swaps++
			}
		case engine.EvDispatch:
			if in(e.T) && seats > 0 {
				empty = append(empty, float64(seats-e.N))
			}
		case engine.EvPostedWait:
			posted = float64(e.N)
			o.Posted.Algorithm = e.Info
		case engine.EvQueueJoin:
			if posted >= 0 && in(e.T) {
				postedAt[e.Guest] = posted
			}
		case engine.EvBoard:
			if p, ok := postedAt[e.Guest]; ok {
				joined := r.joinTimes[e.Guest]
				d := p - e.T.Sub(joined).Seconds()
				abs, bias = append(abs, math.Abs(d)), append(bias, d)
				delete(postedAt, e.Guest)
			}
		case engine.EvHoldStart:
			openHold[e.Train] = e
		case engine.EvHoldEnd:
			if st, ok := openHold[e.Train]; ok {
				delete(openHold, e.Train)
				if r.brakes[st.Block] && st.Info != "ride_stopped" {
					o.StackingS += clip(st.T, e.T)
				}
			}
		}
	}
	for cause, d := range depth { // still stopped at run end
		if d > 0 {
			totals[cause].TotalS += clip(since[cause], we)
		}
	}
	if active > 0 {
		o.StoppedS += clip(stoppedFrom, we)
	}
	for _, cd := range totals {
		o.Downtime = append(o.Downtime, *cd)
	}
	sort.Slice(o.Downtime, func(i, j int) bool { return o.Downtime[i].Cause < o.Downtime[j].Cause })
	o.Availability = 1 - o.StoppedS/we.Sub(ws).Seconds()
	o.EmptySeats = summarize(empty)
	if len(abs) > 0 {
		o.Posted.N = len(abs)
		o.Posted.MeanAbsS = summarize(abs).Mean
		o.Posted.BiasS = summarize(bias).Mean
	}
}

func (o *Ops) write(b *strings.Builder, ridersPerHr float64) {
	fmt.Fprintf(b, "\navailability      %.1f%% (stopped %.0f s)", o.Availability*100, o.StoppedS)
	if o.Availability > 0 && o.Availability < 1 {
		fmt.Fprintf(b, "; running throughput %.0f/hr, lost ~%.0f riders", ridersPerHr/o.Availability, ridersPerHr/o.Availability*o.StoppedS/3600)
	}
	b.WriteString("\n")
	for _, d := range o.Downtime {
		fmt.Fprintf(b, "  %-14s %4d stop(s) %8.0f s\n", d.Cause, d.Count, d.TotalS)
	}
	blocks := make([]string, 0, len(o.CascadeBlocks))
	for k, v := range o.CascadeBlocks {
		blocks = append(blocks, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(blocks)
	fmt.Fprintf(b, "cascade stops     %d %s\n", o.Cascades, strings.Join(blocks, " "))
	fmt.Fprintf(b, "barn swaps        %d\n", o.Swaps)
	fmt.Fprintf(b, "empty seats       mean %.2f per dispatch (max %.0f)\n", o.EmptySeats.Mean, o.EmptySeats.Max)
	fmt.Fprintf(b, "stacking          %.0f s held at brakes\n", o.StackingS)
	if o.Posted.N > 0 {
		fmt.Fprintf(b, "posted vs true    %s: mean |error| %.1f min, bias %+.1f min (n=%d)\n", o.Posted.Algorithm, o.Posted.MeanAbsS/60, o.Posted.BiasS/60, o.Posted.N)
	}
}
