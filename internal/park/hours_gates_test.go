package park

import (
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// A node with hours takes nobody before it opens: the coaster (10:00) and the simple ride (11:00) on a
// 09:00 park. Both log the area_hours closure for posted-wait stats, and the coaster reads closed until then.
func TestNodeHoursKeepAreasShutUntilOpening(t *testing.T) {
	ds, pf := fixtureDataset(t)
	for i := range pf.Nodes {
		switch pf.Nodes[i].ID {
		case "coaster":
			pf.Nodes[i].Hours = "10:00-18:00"
		case "filler":
			pf.Nodes[i].Hours = "11:00-17:00"
		}
	}
	sim := engine.New(1)
	p, err := New(sim, ds, pf, Options{Attendance: 300})
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	sim.RunUntil(engine.Time(30 * engine.Minute))
	if ra := p.acts[p.g.Node("coaster").Idx].(*rideAct); !ra.closed() || !p.closed[ra.n.Idx] {
		t.Error("coaster should read closed before 10:00")
	}
	sim.RunUntil(engine.Time(8 * engine.Hour))
	for _, c := range []struct {
		src  string
		open engine.Time
	}{{"coaster", engine.Time(engine.Hour)}, {"filler", engine.Time(2 * engine.Hour)}} {
		joins, resumed := 0, false
		for _, e := range sim.Log.Events() {
			if e.Src != c.src {
				continue
			}
			if e.Type == engine.EvQueueJoin {
				if e.T < c.open {
					t.Fatalf("%s: queue join at %v, before it opens at %v", c.src, e.T, c.open)
				}
				joins++
			}
			if e.Type == engine.EvRideResume && e.Info == config.CauseAreaHours && e.T == c.open {
				resumed = true
			}
		}
		if joins == 0 || !resumed {
			t.Errorf("%s: %d joins after opening, area_hours resume logged at opening: %v", c.src, joins, resumed)
		}
	}
}

// With two gates, parties enter by shares and leave by their own gate's exit.
func TestTwoGates(t *testing.T) {
	ds, pf := fixtureDataset(t)
	x, y := 380.0, 300.0
	for i := range pf.Nodes {
		if pf.Nodes[i].Kind == config.NodeEntrance {
			pf.Nodes[i].ArrivalShare, pf.Nodes[i].Exit = 0.7, "exit"
		}
	}
	pf.Nodes = append(pf.Nodes,
		config.ParkNode{ID: "gate2", Kind: config.NodeEntrance, ArrivalShare: 0.3, Exit: "exit2", X: &x, Y: &y},
		config.ParkNode{ID: "exit2", Kind: config.NodeExit, X: &x, Y: &y})
	pf.Edges = append(pf.Edges, config.ParkEdge{ID: "hub_gate2", From: "hub", To: "gate2", LengthFt: 100, WidthFt: 10},
		config.ParkEdge{ID: "hub_exit2", From: "hub", To: "exit2", LengthFt: 100, WidthFt: 10})
	sim := engine.New(1)
	p, err := New(sim, ds, pf, Options{Attendance: 2000})
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	sim.RunUntil(engine.Time(10 * engine.Hour))
	first, last := map[int64]string{}, map[int64]string{}
	for _, e := range sim.Log.Events() {
		if e.Type == EvWalk {
			if _, ok := first[e.Guest]; !ok {
				first[e.Guest] = e.Block
			}
			last[e.Guest] = e.Block
		}
	}
	via2 := 0
	for g, b := range first {
		if b == "hub_gate2" {
			via2++
			if l := last[g]; l != "hub_exit2" && p.agents[g-1].gone {
				t.Fatalf("party %d entered by gate2 and left by %s", g, l)
			}
		}
	}
	if share := float64(via2) / float64(len(first)); math.Abs(share-0.3) > 0.05 {
		t.Errorf("gate2 share %.2f, want ~0.3", share)
	}
}
