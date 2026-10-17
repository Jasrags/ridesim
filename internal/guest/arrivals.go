package guest

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// Segment is a constant arrival rate in guests per hour starting at Start (offset from t=0). Parties
// arrive at RatePerHr / (expected guests per arrival), so the guest rate holds for any party mix.
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

// SizeWeight is one entry of a party-size distribution.
type SizeWeight struct {
	Size   int
	Weight float64
}

// Profile describes who arrives. The zero Profile is single-guest standby parties with unlimited
// patience (the M1 behavior).
type Profile struct {
	// Lane forces every arrival into one lane (LightningLane for LL return arrivals); 0 = standby with
	// SingleRiderShare single riders.
	Lane Lane
	// Streams name the RNG streams for timing and for who arrives; empty = arrivals / guest_choice.
	TimeStream, ChoiceStream string

	IDBase int64 // party ids start after this, so several generators never collide

	PartySizes       []SizeWeight // sorted by size; empty = always 1
	SingleRiderShare float64      // probability an arrival is a single rider (size 1, single-rider lane)
	PatienceMin      *config.Dist // nil = unlimited (no balking or reneging)
}

// PartySizesFrom converts a size->share map (as in the park file) into sorted weights.
func PartySizesFrom(m map[int]float64) []SizeWeight {
	out := make([]SizeWeight, 0, len(m))
	for s, w := range m {
		out = append(out, SizeWeight{s, w})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size < out[j].Size })
	return out
}

// Arrivals generates parties as a Poisson process following Curve. Arrival times use the arrivals
// stream; who arrives (size, lane, patience) uses guest_choice, so changing the profile never shifts
// arrival times.
type Arrivals struct {
	sim     *engine.Sim
	curve   Curve
	profile Profile
	to      Arriver
	end     engine.Time
	nextID  int64
}

// StartArrivals schedules arrivals into to until end.
func StartArrivals(sim *engine.Sim, curve Curve, to Arriver, end engine.Time) *Arrivals {
	return StartArrivalsWith(sim, curve, Profile{}, to, end)
}

// StartArrivalsWith is StartArrivals with a guest profile.
func StartArrivalsWith(sim *engine.Sim, curve Curve, profile Profile, to Arriver, end engine.Time) *Arrivals {
	a := &Arrivals{sim: sim, curve: curve, profile: profile, to: to, end: end, nextID: profile.IDBase}
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
		rate := a.curve[i].RatePerHr / a.profile.meanGuests()
		if rate <= 0 {
			t = boundary
			continue
		}
		gap := a.sim.Stream(a.timeStream()).ExpFloat64() * 3600 / rate
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
	p := a.draw()
	p.ID = a.nextID
	a.to.Arrive(p, p.Lane)
	a.scheduleFrom(t)
}

// meanGuests is the expected party size of one arrival.
func (pr Profile) meanGuests() float64 {
	sizes, total := 0.0, 0.0
	for _, sw := range pr.PartySizes {
		sizes += float64(sw.Size) * sw.Weight
		total += sw.Weight
	}
	mean := 1.0
	if total > 0 {
		mean = sizes / total
	}
	return pr.SingleRiderShare + (1-pr.SingleRiderShare)*mean
}

func (a *Arrivals) timeStream() string {
	if a.profile.TimeStream != "" {
		return a.profile.TimeStream
	}
	return engine.StreamArrivals
}

func (a *Arrivals) draw() *Party {
	pr := a.profile
	p := &Party{Size: 1, Lane: Standby}
	if pr.Lane != 0 {
		p.Lane = pr.Lane
	}
	if len(pr.PartySizes) == 0 && pr.SingleRiderShare == 0 && pr.PatienceMin == nil {
		return p // M1 behavior; consumes no guest_choice draws
	}
	choice := pr.ChoiceStream
	if choice == "" {
		choice = engine.StreamGuestChoice
	}
	r := a.sim.Stream(choice)
	if pr.Lane == 0 && pr.SingleRiderShare > 0 && r.Float64() < pr.SingleRiderShare {
		p.Lane = SingleRider
	} else if len(pr.PartySizes) > 0 {
		total := 0.0
		for _, sw := range pr.PartySizes {
			total += sw.Weight
		}
		x := r.Float64() * total
		for _, sw := range pr.PartySizes {
			if x < sw.Weight {
				p.Size = sw.Size
				break
			}
			x -= sw.Weight
		}
	}
	if d := pr.PatienceMin; d != nil {
		m := math.Max(1, d.Mean+d.SD*r.NormFloat64()) // at least a minute of patience
		p.Patience = engine.Seconds(m * 60)
	}
	return p
}
