package guest

import (
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

func TestParseCurve(t *testing.T) {
	c, err := ParseCurve("0:2600, 120:1000")
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c[1].Start != 2*engine.Hour || c[1].RatePerHr != 1000 {
		t.Fatalf("got %+v", c)
	}
	for _, bad := range []string{"60:100", "x", "0:-5"} {
		if _, err := ParseCurve(bad); err == nil {
			t.Errorf("ParseCurve(%q) accepted", bad)
		}
	}
}

func TestArrivalsFollowPiecewiseRates(t *testing.T) {
	sim := engine.New(3)
	var hours [3]int
	to := arriverFunc(func(*Party, Lane) { hours[int(sim.Now())/int(engine.Hour)]++ })
	StartArrivals(sim, Curve{{0, 3000}, {engine.Hour, 0}, {2 * engine.Hour, 1000}}, to, engine.Time(3*engine.Hour))
	sim.RunUntil(engine.Time(3 * engine.Hour))
	want := [3]float64{3000, 0, 1000}
	for h, n := range hours {
		if math.Abs(float64(n)-want[h]) > 4*math.Sqrt(want[h])+0.5 { // 4 sigma
			t.Errorf("hour %d: %d arrivals, want ~%.0f", h, n, want[h])
		}
	}
}

type arriverFunc func(*Party, Lane)

func (f arriverFunc) Arrive(p *Party, l Lane) { f(p, l) }

func TestProfileDrawsSizesLanesPatience(t *testing.T) {
	sim := engine.New(9)
	var n, singles, guests int
	var patience float64
	sizes := map[int]int{}
	to := arriverFunc(func(p *Party, l Lane) {
		n++
		guests += p.Size
		sizes[p.Size]++
		patience += p.Patience.Seconds() / 60
		if l == SingleRider {
			singles++
			if p.Size != 1 {
				t.Errorf("single rider party of %d", p.Size)
			}
		}
	})
	prof := Profile{
		PartySizes:       PartySizesFrom(map[int]float64{1: 0.2, 2: 0.5, 4: 0.3}),
		SingleRiderShare: 0.1,
		PatienceMin:      &config.Dist{Mean: 40, SD: 10},
	}
	StartArrivalsWith(sim, Curve{{0, 20000}}, prof, to, engine.Time(engine.Hour))
	sim.RunUntil(engine.Time(engine.Hour))
	frac := func(k int) float64 { return float64(k) / float64(n) }
	if math.Abs(frac(singles)-0.1) > 0.01 {
		t.Errorf("single-rider share %.3f, want 0.10", frac(singles))
	}
	if want := 0.9 * 0.5; math.Abs(frac(sizes[2])-want) > 0.015 {
		t.Errorf("size-2 share %.3f, want %.3f", frac(sizes[2]), want)
	}
	if sizes[3] != 0 {
		t.Errorf("drew size 3, not in the distribution")
	}
	if mean := patience / float64(n); math.Abs(mean-40) > 0.5 {
		t.Errorf("mean patience %.1f min, want 40", mean)
	}
}

// Arrival times come from the arrivals stream only: profiles with the same expected party size (and so
// the same party rate) give identical arrival times, however sizes, lanes and patience are drawn.
func TestProfileDoesNotShiftArrivalTimes(t *testing.T) {
	times := func(p Profile) []engine.Time {
		sim := engine.New(4)
		var out []engine.Time
		StartArrivalsWith(sim, Curve{{0, 3000}}, p, arriverFunc(func(*Party, Lane) { out = append(out, sim.Now()) }), engine.Time(engine.Hour))
		sim.RunUntil(engine.Time(engine.Hour))
		return out
	}
	a := times(Profile{PartySizes: PartySizesFrom(map[int]float64{2: 1})})
	b := times(Profile{PartySizes: PartySizesFrom(map[int]float64{1: 0.5, 3: 0.5}), PatienceMin: &config.Dist{Mean: 30}})
	if len(a) != len(b) {
		t.Fatalf("%d vs %d arrivals", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("arrival %d at %v vs %v", i, a[i], b[i])
		}
	}
}

// Curve rates are guests per hour whatever the party mix.
func TestCurveRateIsGuestsPerHour(t *testing.T) {
	sim := engine.New(5)
	guests := 0
	prof := Profile{PartySizes: PartySizesFrom(map[int]float64{2: 0.5, 4: 0.5}), SingleRiderShare: 0.1}
	StartArrivalsWith(sim, Curve{{0, 3000}}, prof, arriverFunc(func(p *Party, _ Lane) { guests += p.Size }), engine.Time(20*engine.Hour))
	sim.RunUntil(engine.Time(20 * engine.Hour))
	if perHr := float64(guests) / 20; math.Abs(perHr-3000) > 60 {
		t.Fatalf("%.0f guests/hr, want ~3,000", perHr)
	}
}
