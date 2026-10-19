package park

import (
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

func newFixturePark(t *testing.T, seed uint64, opts Options) (*Park, *engine.Sim) {
	t.Helper()
	ds, pf := fixtureDataset(t)
	sim := engine.New(seed)
	p, err := New(sim, ds, pf, opts)
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	return p, sim
}

func scale(v float64) *float64 { return &v }

// close_ride shows the ride closed for the given minutes, then reopens it; it is logged as an ops event.
func TestCloseRideForMinutes(t *testing.T) {
	p, sim := newFixturePark(t, 1, Options{Attendance: 300})
	sim.RunUntil(engine.Time(engine.Hour))
	if err := p.Apply(config.OpsEvent{Action: config.OpCloseRide, Node: "coaster", Minutes: 20}); err != nil {
		t.Fatal(err)
	}
	ra := p.acts[p.g.Node("coaster").Idx].(*rideAct)
	sim.RunUntil(engine.Time(engine.Hour + 10*engine.Minute))
	if !ra.closed() {
		t.Fatal("coaster should be closed 10 min into a 20 min closure")
	}
	sim.RunUntil(engine.Time(engine.Hour + 21*engine.Minute))
	if ra.closed() {
		t.Fatal("coaster should reopen after 20 min")
	}
	if n, _ := count(sim, EvParkOps, Scope); n != 1 {
		t.Fatalf("%d ops events, want 1", n)
	}
}

func TestApplyRejectsBadOps(t *testing.T) {
	p, _ := newFixturePark(t, 1, Options{Attendance: 10})
	for _, op := range []config.OpsEvent{
		{Action: config.OpCloseRide, Node: "nowhere", Minutes: 5},
		{Action: config.OpAddTrain, Node: "filler"},
		{Action: config.OpCrowdScale, Value: scale(3)},
		{Action: "teleport"},
	} {
		if err := p.Apply(op); err == nil {
			t.Errorf("%+v accepted", op)
		}
	}
}

// Crowd scale thins a fixed day: half the scale admits about half the guests, and a change later in the day
// leaves every event before it unchanged.
func TestCrowdScaleThinsWithoutShiftingEarlierEvents(t *testing.T) {
	arrived := func(p *Park, sim *engine.Sim) int { _, g := count(sim, EvParkArrive, Scope); return g }
	full, fsim := newFixturePark(t, 4, Options{Attendance: 1000})
	fsim.RunUntil(full.CloseAt())
	half, hsim := newFixturePark(t, 4, Options{Attendance: 1000})
	if err := half.Apply(config.OpsEvent{Action: config.OpCrowdScale, Value: scale(0.5)}); err != nil {
		t.Fatal(err)
	}
	hsim.RunUntil(half.CloseAt())
	if r := float64(arrived(half, hsim)) / float64(arrived(full, fsim)); r < 0.4 || r > 0.6 {
		t.Fatalf("scale 0.5 admitted %.2f of the guests", r)
	}

	late, lsim := newFixturePark(t, 4, Options{Attendance: 1000})
	lsim.RunUntil(engine.Time(engine.Hour)) // the fixture admits guests in hours 0-2
	cut := len(lsim.Log.Events())
	if err := late.Apply(config.OpsEvent{Action: config.OpCrowdScale, Value: scale(1.8)}); err != nil {
		t.Fatal(err)
	}
	lsim.RunUntil(late.CloseAt())
	a, b := fsim.Log.Events(), lsim.Log.Events()
	for i := 0; i < cut; i++ {
		if a[i] != b[i] {
			t.Fatalf("event %d differs before the scale change: %+v vs %+v", i, a[i], b[i])
		}
	}
	if arrived(late, lsim) <= arrived(full, fsim) {
		t.Fatal("raising the scale at 1 h admitted no extra guests")
	}
}

// The view reports what levers did, and building it leaves the run unchanged.
func TestViewReflectsLeversAndChangesNothing(t *testing.T) {
	p, sim := newFixturePark(t, 3, Options{Attendance: 400})
	ref, rsim := newFixturePark(t, 3, Options{Attendance: 400})
	sim.RunUntil(engine.Time(engine.Hour))
	rsim.RunUntil(engine.Time(engine.Hour))
	for i := 0; i < 20; i++ {
		_ = p.View()
	}
	if err := p.Apply(config.OpsEvent{Action: config.OpCloseRide, Node: "coaster", Minutes: 10}); err != nil {
		t.Fatal(err)
	}
	if err := ref.Apply(config.OpsEvent{Action: config.OpCloseRide, Node: "coaster", Minutes: 10}); err != nil {
		t.Fatal(err)
	}
	v := p.View()
	var coaster *RideView
	for i := range v.Rides {
		if v.Rides[i].Node == "coaster" {
			coaster = &v.Rides[i]
		}
	}
	if coaster == nil || coaster.Status != "closed" || coaster.Trains < 1 || len(v.Edges) != len(p.g.Edges) || !v.Nodes[0].HasPos {
		t.Fatalf("view %+v", coaster)
	}
	sim.RunUntil(p.CloseAt())
	rsim.RunUntil(ref.CloseAt())
	a, b := sim.Log.Events(), rsim.Log.Events()
	if len(a) != len(b) {
		t.Fatalf("viewing changed the run: %d vs %d events", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("event %d differs", i)
		}
	}
}

// ll_inventory resets what windows not yet started can sell to the new share, less what they already sold;
// started windows keep theirs. It is logged as an ops event.
func TestLLInventoryLeverResetsFutureWindows(t *testing.T) {
	p, sim := newFixturePark(t, 1, Options{Attendance: 300})
	r := p.ll.ride(p.g.Node("coaster").Idx)
	at := engine.Time(90 * engine.Minute) // inside window 1; window 2 is the first not yet started
	sim.RunUntil(at)
	before := append([]int(nil), r.inv...)
	sold := make([]int, len(r.inv))
	for i, v := range r.inv {
		sold[i] = r.per - v
	}
	if err := p.Apply(config.OpsEvent{Action: config.OpLLInventory, Node: "coaster", Value: scale(0.5)}); err != nil {
		t.Fatal(err)
	}
	per := int(math.Round(0.5 * r.capacity)) // 60-min windows
	for w := range r.inv {
		want := before[w]
		if w >= 2 {
			want = max(0, per-sold[w])
		}
		if r.inv[w] != want {
			t.Errorf("window %d: %d sellable, want %d", w, r.inv[w], want)
		}
	}
	if n, _ := count(sim, EvParkOps, Scope); n != 1 {
		t.Fatalf("%d ops events, want 1", n)
	}
}

func TestLLLeversNeedALightningLaneRide(t *testing.T) {
	p, _ := newFixturePark(t, 1, Options{Attendance: 10})
	for _, op := range []config.OpsEvent{
		{Action: config.OpLLInventory, Node: "filler", Value: scale(0.5)}, // untiered park: simple rides sell no LL
		{Action: config.OpLLMerge, Node: "hub", Value: scale(1)},
		{Action: config.OpLLMerge, Node: "coaster", Value: scale(1.5)}, // whole numbers only
		{Action: config.OpLLMerge, Node: "coaster", Value: scale(11)},
		{Action: config.OpLLInventory, Node: "coaster", Value: scale(1.2)},
		{Action: config.OpLLInventory, Node: "coaster"},
	} {
		if err := p.Apply(op); err == nil {
			t.Errorf("%+v accepted", op)
		}
	}
	if err := p.Apply(config.OpsEvent{Action: config.OpLLMerge, Node: "coaster", Value: scale(0)}); err != nil {
		t.Errorf("ll_merge 0 on the coaster: %v", err)
	}
}

