package park

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// A simple ride boards Lightning Lane first, up to 2/3 of each vehicle, so standby always moves; with
// no standby waiting, Lightning Lane may fill the vehicle.
func TestSimpleRideBoardsLightningLaneUpToTwoThirds(t *testing.T) {
	sim := engine.New(1)
	s := newSimpleRide(sim, &Node{ParkNode: config.ParkNode{ID: "r", Kind: config.NodeRideSimple, CapacityPerHr: 540, DurationS: 60}})
	if s.seats != 9 {
		t.Fatalf("seats = %d, want 9", s.seats)
	}
	id := int64(0)
	join := func(n int, lane guest.Lane) {
		for i := 0; i < n; i++ {
			id++
			s.visit(&guest.Party{ID: id, Size: 1}, lane)
		}
	}
	join(10, guest.LightningLane)
	join(10, guest.Standby)
	s.dispatch()
	if ll, sb := s.llLine.Guests(), s.line.Guests(); ll != 4 || sb != 7 {
		t.Errorf("after one vehicle: LL waiting %d, standby waiting %d; want 4 and 7 (6 LL + 3 standby boarded)", ll, sb)
	}
	for s.line.Guests() > 0 {
		s.dispatch()
	}
	join(9, guest.LightningLane)
	before := s.llLine.Guests()
	s.dispatch()
	if got := before - s.llLine.Guests(); got != 9 {
		t.Errorf("with no standby waiting, LL boarded %d, want the whole vehicle (9)", got)
	}
}

// The posted standby wait counts the seats Lightning Lane takes.
func TestSimpleRidePostedWaitAllowsForLightningLane(t *testing.T) {
	sim := engine.New(1)
	s := newSimpleRide(sim, &Node{ParkNode: config.ParkNode{ID: "r", Kind: config.NodeRideSimple, CapacityPerHr: 540, DurationS: 60}})
	for i := int64(1); i <= 90; i++ {
		s.visit(&guest.Party{ID: i, Size: 1}, guest.Standby)
	}
	plain := s.posted()
	for i := int64(100); i < 400; i++ {
		s.visit(&guest.Party{ID: i, Size: 1}, guest.LightningLane)
	}
	for i := 0; i < 15; i++ {
		s.dispatch()
		for j := 0; j < 3; j++ { // keep standby topped up: each vehicle took 3
			s.visit(&guest.Party{ID: int64(1000 + 3*i + j), Size: 1}, guest.Standby)
		}
	}
	if got := s.posted(); got < 2*plain {
		t.Errorf("posted with LL taking 2/3 of seats = %v, want about 3x the no-LL %v", got, plain)
	}
}

func tieredPark(t *testing.T) (*config.Dataset, *config.ParkFile) {
	t.Helper()
	ds, pf := fixtureDataset(t)
	for i := range pf.Nodes {
		switch pf.Nodes[i].ID {
		case "coaster":
			pf.Nodes[i].LightningLane = config.LLTier1
		case "filler":
			pf.Nodes[i].LightningLane = config.LLSinglePass
			pf.Nodes[i].CapacityPerHr = 600
		}
	}
	pf.LightningLane.AdvanceByTier = map[string]int{config.LLTier1: 1, config.LLTier2: 2}
	pf.LightningLane.SinglePassShare = 0.3
	pf.LightningLane.ElsewhereShare = 0 // tier 2 has no rides here, so tier 2 advance bookings find nothing
	return ds, pf
}

// Multi Pass buyers book at most the tier counts before the day; the Single Pass ride is booked only
// as a Single Pass (at most once per party, never rebooked) and redeemed through the simple ride's
// Lightning Lane line.
func TestTieredLightningLane(t *testing.T) {
	ds, pf := tieredPark(t)
	sim := engine.New(4)
	p, err := New(sim, ds, pf, Options{Attendance: 1500})
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	sim.RunUntil(p.CloseAt())
	booked := map[int64]map[string]int{}
	var single, tier1 int
	for _, e := range sim.Log.Events() {
		if e.Type != EvLLBook {
			continue
		}
		if booked[e.Guest] == nil {
			booked[e.Guest] = map[string]int{}
		}
		booked[e.Guest][e.Block]++
		switch e.Block {
		case "filler":
			single++
		case "coaster":
			tier1++
		}
	}
	for g, m := range booked {
		if m["filler"] > 1 || m["coaster"] > 1 {
			t.Fatalf("guest %d booked %v: each ride at most once a day", g, m)
		}
	}
	if single == 0 || tier1 == 0 {
		t.Fatalf("single pass bookings %d, tier 1 bookings %d; want both", single, tier1)
	}
	if n, _ := count(sim, EvLLRedeem, ""); n == 0 {
		t.Fatal("no redemptions")
	}
	var llBoards int
	for _, e := range sim.Log.Events() {
		if e.Src == "filler" && e.Type == engine.EvBoard && e.Info == guest.LightningLane.String() {
			llBoards++
		}
	}
	if llBoards == 0 {
		t.Error("no Lightning Lane boardings on the simple ride")
	}
}

// A park without tiers keeps the M7 booking model: the simple ride sells nothing.
func TestUntieredParkSellsOnlyHostedRides(t *testing.T) {
	_, sim := runPark(t, 2, Options{Attendance: 1500}, 0)
	for _, e := range sim.Log.Events() {
		if e.Type == EvLLBook && e.Block == "filler" {
			t.Fatal("untiered park sold a simple ride")
		}
	}
}

// The ll_merge lever on a simple ride sets LL guests per standby guest: 1 boards LL up to half a vehicle,
// 0 seats LL only where standby leaves room.
func TestSimpleRideMergeRatio(t *testing.T) {
	for _, c := range []struct{ ratio, wantLL int }{{1, 4}, {0, 0}} {
		sim := engine.New(1)
		s := newSimpleRide(sim, &Node{ParkNode: config.ParkNode{ID: "r", Kind: config.NodeRideSimple, CapacityPerHr: 540, DurationS: 60}})
		s.setMergeRatio(c.ratio)
		for i := int64(1); i <= 20; i++ {
			lane := guest.Standby
			if i <= 10 {
				lane = guest.LightningLane
			}
			s.visit(&guest.Party{ID: i, Size: 1}, lane)
		}
		s.dispatch()
		if got := 10 - s.llLine.Guests(); got != c.wantLL {
			t.Errorf("ratio %d: %d LL boarded of 9 seats, want %d", c.ratio, got, c.wantLL)
		}
	}
}
