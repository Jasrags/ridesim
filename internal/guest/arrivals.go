package guest

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/jasrags/ridesim/internal/engine"
)

// Segment is a constant arrival rate starting at Start (offset from t=0).
type Segment struct {
	Start     engine.Duration
	RatePerHr float64
}

// Curve is a piecewise-constant arrival rate, sorted by Start, first segment at 0.
type Curve []Segment

// ParseCurve parses "rate" or "min:rate,min:rate,..." (minutes from start, guests per hour).
func ParseCurve(s string) (Curve, error) {
	var c Curve
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		startMin, rateStr := "0", part
		if i := strings.IndexByte(part, ':'); i >= 0 {
			startMin, rateStr = part[:i], part[i+1:]
		}
		m, err := strconv.ParseFloat(startMin, 64)
		if err != nil {
			return nil, fmt.Errorf("arrival curve %q: bad start %q", s, startMin)
		}
		r, err := strconv.ParseFloat(rateStr, 64)
		if err != nil || r < 0 {
			return nil, fmt.Errorf("arrival curve %q: bad rate %q", s, rateStr)
		}
		c = append(c, Segment{Start: engine.Seconds(m * 60), RatePerHr: r})
	}
	sort.SliceStable(c, func(i, j int) bool { return c[i].Start < c[j].Start })
	if len(c) == 0 || c[0].Start != 0 {
		return nil, fmt.Errorf("arrival curve %q must start at minute 0", s)
	}
	return c, nil
}

// Arriver receives arriving parties (implemented by ride.Ride).
type Arriver interface {
	Arrive(p *Party, lane Lane)
}

// Arrivals generates single-guest standby parties as a Poisson process following Curve, using the
// arrivals RNG stream only.
type Arrivals struct {
	sim    *engine.Sim
	curve  Curve
	to     Arriver
	end    engine.Time
	nextID int64
}

// StartArrivals schedules arrivals into to until end.
func StartArrivals(sim *engine.Sim, curve Curve, to Arriver, end engine.Time) *Arrivals {
	a := &Arrivals{sim: sim, curve: curve, to: to, end: end}
	a.scheduleFrom(sim.Now())
	return a
}

func (a *Arrivals) segmentAt(t engine.Time) int {
	i := sort.Search(len(a.curve), func(i int) bool { return engine.Time(a.curve[i].Start) > t })
	return i - 1
}

// scheduleFrom draws the next arrival after t. Crossing a segment boundary restarts the draw at the
// boundary with the new rate, which is exact for a Poisson process (memoryless).
func (a *Arrivals) scheduleFrom(t engine.Time) {
	for t < a.end {
		i := a.segmentAt(t)
		boundary := a.end
		if i+1 < len(a.curve) && engine.Time(a.curve[i+1].Start) < boundary {
			boundary = engine.Time(a.curve[i+1].Start)
		}
		rate := a.curve[i].RatePerHr
		if rate <= 0 {
			t = boundary
			continue
		}
		gap := a.sim.Stream(engine.StreamArrivals).ExpFloat64() * 3600 / rate
		next := t.Add(engine.Duration(math.Max(1, math.Round(gap*1000))))
		if next >= boundary {
			t = boundary
			continue
		}
		a.sim.At(next, func() { a.arrive(next) })
		return
	}
}

func (a *Arrivals) arrive(t engine.Time) {
	a.nextID++
	a.to.Arrive(&Party{ID: a.nextID, Size: 1}, Standby)
	a.scheduleFrom(t)
}
