package coaster

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// The fixture train has 20 seats in 10 rows of 2; one train starts loading at t=0 with no demand.
func loadingHarness(t *testing.T, opts Options) *harness {
	t.Helper()
	opts.Trains, opts.FixedTimes = 1, true
	cfg := fixture(t)
	cfg.Queues.SingleRider.Enabled = true
	return newHarness(t, cfg, opts, 0, engine.Hour)
}

// Without a single-rider line, a single rider queues standby as a party of 1.
func TestSingleRiderFallsBackToStandbyWithoutLine(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true}, 0, engine.Hour)
	p := arriveAt(h, 1, 1, guest.SingleRider, 0)
	h.sim.RunUntil(sec(2))
	if p.Lane != guest.Standby || !boarded(h, p) {
		t.Fatalf("lane %v boarded %v, want standby and boarded during the load window", p.Lane, boarded(h, p))
	}
}

var nextID int64

func arriveAt(h *harness, at float64, size int, lane guest.Lane, patienceMin float64) *guest.Party {
	nextID++
	p := &guest.Party{ID: nextID, Size: size, Patience: engine.Seconds(patienceMin * 60)}
	h.sim.At(sec(at), func() { h.c.Arrive(p, lane) })
	return p
}

func loadTrain(h *harness) *train { return h.c.load.occupant }

func boarded(h *harness, p *guest.Party) bool {
	for _, e := range h.sim.Log.Events() {
		if e.Type == engine.EvBoard && e.Guest == p.ID {
			return true
		}
	}
	return false
}

func TestPartyOfThreeLeavesGapForSingleRider(t *testing.T) {
	h := loadingHarness(t, Options{})
	arriveAt(h, 1, 3, guest.Standby, 0)
	sr := arriveAt(h, 2, 1, guest.SingleRider, 0)
	h.sim.RunUntil(sec(3))
	tr := loadTrain(h)
	if !boarded(h, sr) || tr.riders != 4 || tr.rowsFree != 8 || tr.gaps != 0 {
		t.Fatalf("riders %d rowsFree %d gaps %d, single rider boarded %v; want 4/8/0/true", tr.riders, tr.rowsFree, tr.gaps, boarded(h, sr))
	}
}

func TestSoloStandbyFillsGapButNobodySkips(t *testing.T) {
	h := loadingHarness(t, Options{})
	arriveAt(h, 1, 3, guest.Standby, 0) // 2 rows, 1 gap
	solo := arriveAt(h, 2, 1, guest.Standby, 0)
	h.sim.RunUntil(sec(3))
	if tr := loadTrain(h); !boarded(h, solo) || tr.gaps != 0 || tr.rowsFree != 8 {
		t.Fatalf("solo boarded %v, gaps %d rowsFree %d; want the solo in the gap", boarded(h, solo), tr.gaps, tr.rowsFree)
	}
	for i := 0; i < 7; i++ { // 7 more rows: 1 row left
		arriveAt(h, 4, 2, guest.Standby, 0)
	}
	arriveAt(h, 5, 3, guest.Standby, 0) // needs 2 rows: blocks the line
	behind := arriveAt(h, 6, 1, guest.Standby, 0)
	h.sim.RunUntil(sec(10))
	if boarded(h, behind) {
		t.Fatal("solo behind a blocked party of 3 skipped the line")
	}
}

func TestSingleRidersTakeEmptyRowsOnlyAfterLoadDone(t *testing.T) {
	h := loadingHarness(t, Options{})
	sr := arriveAt(h, 1, 1, guest.SingleRider, 0)
	h.sim.RunUntil(sec(24))
	if boarded(h, sr) {
		t.Fatal("single rider took an empty row during the load window")
	}
	h.sim.RunUntil(sec(26)) // load_s = 25
	if !boarded(h, sr) {
		t.Fatal("single rider not seated at load_done")
	}
}

// hold_for_full would wait forever for a party that cannot fit: it dispatches instead.
func TestHoldForFullDoesNotStallOnUnfittableParty(t *testing.T) {
	h := loadingHarness(t, Options{DispatchRule: config.DispatchHoldForFull})
	for i := 0; i < 9; i++ {
		arriveAt(h, 1, 2, guest.Standby, 0)
	}
	arriveAt(h, 2, 3, guest.Standby, 0)
	h.sim.RunUntil(sec(600))
	// load_done at 25 s finds the head party unfittable: check 5 + clearance 2
	if got := firstAt(h.sim.Log.Events(), engine.EvDispatch, "", 0); got != sec(32) {
		t.Fatalf("dispatch at %v, want 32 s", got)
	}
}

func TestBalkWhenPostedWaitExceedsPatience(t *testing.T) {
	h := loadingHarness(t, Options{})
	h.sim.At(0, func() { h.c.Stop(CauseWeatherHold) })
	for i := 0; i < 100; i++ { // posted = 100 / (20 seats / 50 s) = 250 s
		arriveAt(h, 1, 1, guest.Standby, 0)
	}
	impatient := arriveAt(h, 2, 2, guest.Standby, 3) // 180 s < 250 s
	patient := arriveAt(h, 2, 2, guest.Standby, 5)   // 300 s > 250 s
	h.sim.RunUntil(sec(3))
	if n := countEvents(h.sim.Log.Events(), engine.EvBalk, "standby"); n != 1 {
		t.Fatalf("%d balks, want 1", n)
	}
	for _, e := range h.sim.Log.Events() {
		if e.Type == engine.EvBalk && e.Guest != impatient.ID {
			t.Fatalf("wrong party balked: %d (patient party %d)", e.Guest, patient.ID)
		}
	}
}

func TestRenegeLeavesOutdoorLineAfterPatience(t *testing.T) {
	h := loadingHarness(t, Options{})
	h.sim.At(0, func() { h.c.Stop(CauseWeatherHold) })
	p := arriveAt(h, 10, 4, guest.Standby, 2)
	h.sim.RunUntil(sec(11))
	if h.c.WaitingGuests() != 4 {
		t.Fatalf("waiting %d, want 4", h.c.WaitingGuests())
	}
	h.sim.RunUntil(sec(200))
	if got := firstAt(h.sim.Log.Events(), engine.EvRenege, "", 0); got != sec(130) {
		t.Fatalf("renege at %v, want 130 s (joined 10 s + 2 min)", got)
	}
	if h.c.WaitingGuests() != 0 || p.Committed {
		t.Fatalf("waiting %d after renege", h.c.WaitingGuests())
	}
	h.sim.At(sec(201), func() { h.c.Resume(CauseWeatherHold) })
	h.sim.RunUntil(sec(400))
	if boarded(h, p) {
		t.Fatal("reneged party boarded")
	}
}
