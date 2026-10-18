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

// A short edge's capacity uses min_capacity_length_ft, so it carries a crowd like a longer walkway would.
func TestShortEdgesUseTheCapacityFloor(t *testing.T) {
	p := parkFixture(t)
	p.Edges = append(p.Edges, config.ParkEdge{ID: "stub", From: "hub", To: "filler", LengthFt: 10, WidthFt: 10})
	g, err := NewGraph(p)
	if err != nil {
		t.Fatal(err)
	}
	admitted := func(floor float64) int {
		p.Congestion.MinCapacityLengthFt = floor
		tt := NewTravelTime(p)
		e := g.Edge("stub")
		e.guests = 0
		n := 0
		for {
			if _, ok := tt.Enter(e, 1); !ok {
				return n
			}
			n++
		}
	}
	if without, with := admitted(0), admitted(100); without != 30 || with != 300 {
		t.Errorf("guests admitted to a 10 ft x 10 ft edge: %d without the floor, %d with 100 ft; want 30 and 300", without, with)
	}
}

// Rerouting weighs edges by their current walking time, so a jammed shortcut loses its traffic.
func TestRerouteAvoidsJammedEdges(t *testing.T) {
	p := parkFixture(t)
	// A second, longer and wide path from hub to dining.
	p.Nodes = append(p.Nodes, config.ParkNode{ID: "bend", Kind: config.NodeJunction})
	p.Edges = append(p.Edges,
		config.ParkEdge{ID: "hub_bend", From: "hub", To: "bend", LengthFt: 100, WidthFt: 40},
		config.ParkEdge{ID: "bend_dining", From: "bend", To: "dining", LengthFt: 100, WidthFt: 40})
	g, err := NewGraph(p)
	if err != nil {
		t.Fatal(err)
	}
	hub, dining := g.Node("hub").Idx, g.Node("dining").Idx
	if r := g.Route(hub, dining); len(r) != 1 || r[0].ID != "hub_dining" {
		t.Fatalf("free-flow route %v, want the 100 ft direct edge", r)
	}
	tt := NewTravelTime(p)
	g.Edge("hub_dining").guests = 250 // n/C = 2.5 on the 100 ft x 10 ft edge
	g.Reroute(func(e *Edge) float64 { return tt.Estimate(e).Seconds() * p.Congestion.FreeSpeedFps })
	if r := g.Route(hub, dining); len(r) != 2 || r[0].ID != "hub_bend" {
		t.Errorf("rerouted %v, want the wide 200 ft detour", r)
	}
	if d := g.Distance(hub, dining); d < 199 || d > 201 {
		t.Errorf("rerouted distance %.1f, want the detour's 200 ft", d)
	}
}
