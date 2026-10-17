package park

import (
	"bytes"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func fixtureDataset(t *testing.T) (*config.Dataset, *config.ParkFile) {
	t.Helper()
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ds.Files {
		if f.Kind == config.KindPark {
			return ds, f.Park
		}
	}
	t.Fatal("no park fixture")
	return nil, nil
}

func runPark(t *testing.T, seed uint64, opts Options, extra engine.Duration) (*Park, *engine.Sim) {
	t.Helper()
	ds, pf := fixtureDataset(t)
	sim := engine.New(seed)
	p, err := New(sim, ds, pf, opts)
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	sim.RunUntil(p.CloseAt().Add(extra))
	return p, sim
}

func count(sim *engine.Sim, typ, src string) (n, guests int) {
	for _, e := range sim.Log.Events() {
		if e.Type == typ && (src == "" || e.Src == src) {
			n++
			guests += e.N
		}
	}
	return
}

func TestParkRunIsDeterministic(t *testing.T) {
	var logs [2]bytes.Buffer
	for i := range logs {
		_, sim := runPark(t, 3, Options{Attendance: 600}, engine.Hour)
		if err := sim.Log.Write(&logs[i]); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(logs[0].Bytes(), logs[1].Bytes()) {
		t.Fatal("same seed gave different park logs")
	}
}

// Every guest who enters leaves once the park has closed and activities have finished.
func TestGuestsArriveAndAllLeaveAfterClose(t *testing.T) {
	p, sim := runPark(t, 1, Options{Attendance: 600}, 3*engine.Hour)
	_, in := count(sim, EvParkArrive, Scope)
	_, out := count(sim, EvParkLeave, Scope)
	if in < 450 || in > 750 {
		t.Fatalf("arrived %d guests, want ~600", in)
	}
	if out != in || p.InPark() != 0 {
		t.Fatalf("arrived %d, left %d, still in park %d", in, out, p.InPark())
	}
	for _, src := range []string{"coaster", "filler", "dining", "elsewhere", "theater"} {
		if n, _ := count(sim, engine.EvBoard, src); n == 0 {
			t.Errorf("nobody used %s", src)
		}
	}
}

// When the show ends the audience leaves by the exit edge.
func TestShowAudienceLeavesByExitEdge(t *testing.T) {
	_, sim := runPark(t, 1, Options{Attendance: 600}, 0)
	var end engine.Time = -1
	audience := 0
	for _, e := range sim.Log.Events() {
		if e.Type == EvShowEnd {
			end, audience = e.T, e.N
		}
	}
	if end < 0 || audience == 0 {
		t.Fatalf("show end %v audience %d", end, audience)
	}
	walked := 0
	for _, e := range sim.Log.Events() {
		if e.Type == EvWalk && e.Block == "theater_hub" && e.T == end {
			walked += e.N
		}
	}
	if walked != audience {
		t.Fatalf("%d of %d left by theater_hub at show end", walked, audience)
	}
}

// Lightning Lane: bookings never oversell a window, and redemptions join the ride's LL line.
func TestLightningLaneRespectsInventory(t *testing.T) {
	p, sim := runPark(t, 2, Options{Attendance: 1500}, 0)
	perWindow := map[int]int{}
	sizes := map[int64]int{}
	for _, a := range p.agents {
		sizes[a.id] = a.size
	}
	books := 0
	for _, e := range sim.Log.Events() {
		if e.Type == EvLLBook && e.Block == "coaster" {
			perWindow[e.N] += sizes[e.Guest]
			books++
		}
	}
	limit := int(0.3 * 1000) // inventory_share x listed capacity x 60-min window
	for w, g := range perWindow {
		if g > limit {
			t.Fatalf("window %d s sold %d guests, limit %d", w, g, limit)
		}
	}
	redeem, _ := count(sim, EvLLRedeem, Scope)
	llJoins := 0
	for _, e := range sim.Log.Events() {
		if e.Src == "coaster" && e.Type == engine.EvQueueJoin && e.Info == guest.LightningLane.String() {
			llJoins++
		}
	}
	if books == 0 || redeem == 0 || llJoins == 0 {
		t.Fatalf("books %d redeems %d LL joins %d", books, redeem, llJoins)
	}
}

func TestNoLightningLaneMeansNoBookings(t *testing.T) {
	_, sim := runPark(t, 2, Options{Attendance: 600, NoLightningLane: true}, 0)
	if n, _ := count(sim, EvLLBook, Scope); n != 0 {
		t.Fatalf("%d bookings with LL off", n)
	}
}

// Choice uses the posted board: a long posted wait at the coaster steers guests away from it.
func TestChoiceAvoidsLongPostedWait(t *testing.T) {
	ds, pf := fixtureDataset(t)
	share := func(postedMin float64) float64 {
		sim := engine.New(5)
		p, err := New(sim, ds, pf, Options{Attendance: 1, NoLightningLane: true})
		if err != nil {
			t.Fatal(err)
		}
		c := p.g.Node("coaster").Idx
		p.board[c] = engine.Seconds(postedMin * 60)
		a := &agent{id: 1, size: 2, thrill: 0.5, at: p.g.Node("hub").Idx, count: make([]int, len(p.g.Nodes)),
			meals: make([]bool, len(p.meals)), llBooked: make([]bool, len(p.g.Nodes))}
		hits := 0
		for i := 0; i < 2000; i++ {
			if p.choose(a) == c {
				hits++
			}
		}
		return float64(hits) / 2000
	}
	short, long := share(0), share(60)
	if !(short > 0.2 && long < 0.05) {
		t.Fatalf("coaster share: %.2f at 0 min posted, %.2f at 60 min", short, long)
	}
}
