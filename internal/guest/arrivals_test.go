package guest

import (
	"math"
	"testing"

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
