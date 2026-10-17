package park

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

func parkFixture(t *testing.T) *config.ParkFile {
	t.Helper()
	f, err := config.LoadFile("../config/testdata/test-fx-park.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return f.Park
}

func TestRoutesFollowShortestFreeFlowPath(t *testing.T) {
	g, err := NewGraph(parkFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Route(g.Node("gate").Idx, g.Node("theater").Idx)
	if len(r) != 2 || r[0].ID != "gate_hub" || r[1].ID != "theater_hub" {
		t.Fatalf("route %v", r)
	}
	if d := g.Distance(g.Node("gate").Idx, g.Node("theater").Idx); d != 200 {
		t.Fatalf("distance %g, want 200", d)
	}
}

func TestFixedDelayIsFreeFlow(t *testing.T) {
	p := parkFixture(t)
	p.Park.WalkingMode = config.WalkingFixedDelay
	g, _ := NewGraph(p)
	tt := NewTravelTime(p)
	e := g.Edge("hub_coaster")
	for i := 0; i < 50; i++ {
		d, ok := tt.Enter(e, 4)
		if !ok || d != engine.Seconds(25) { // 100 ft at 4 ft/s
			t.Fatalf("enter %d: %v %v", i, d, ok)
		}
	}
}

func TestCongestionIsBPRWithJamCap(t *testing.T) {
	p := parkFixture(t) // 100 x 10 ft: C = 100 guests, jam = 300
	g, _ := NewGraph(p)
	tt := NewTravelTime(p)
	e := g.Edge("hub_dining")
	d, _ := tt.Enter(e, 1)
	if d != engine.Seconds(25*(1+1.5*1e-6)) {
		t.Fatalf("one guest: %v", d)
	}
	d, _ = tt.Enter(e, 99) // n = C: t = t0 * (1 + 1.5)
	if d != engine.Seconds(62.5) {
		t.Fatalf("at capacity: %v, want 62.5 s", d)
	}
	if _, ok := tt.Enter(e, 201); ok {
		t.Fatal("entered past jam density")
	}
	tt.Leave(e, 100)
	if _, ok := tt.Enter(e, 301); !ok {
		t.Fatal("an empty edge must admit one party")
	}
}

func TestSpillbackNarrowsTheWalkway(t *testing.T) {
	p := parkFixture(t)
	g, _ := NewGraph(p)
	c := congestion{p.Congestion}
	e := g.Edge("hub_coaster")
	e.spill = func() int { return 400 } // 400 x 1.5 / 100 = 6 ft taken of 10
	if w := c.Width(e); w != 4 {
		t.Fatalf("width %g, want 4", w)
	}
	e.spill = func() int { return 10000 }
	if w := c.Width(e); w != 1 {
		t.Fatalf("width %g, want the 10%% floor", w)
	}
}
