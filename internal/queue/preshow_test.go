package queue

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

type fakeHost struct {
	sim      *engine.Sim
	reenter  *Preshow // when set, Release calls back into Drain and Pull like the coaster does
	outdoor  FIFO
	loadArea int
	capacity int
	released []*guest.Party
}

func (h *fakeHost) NextParty() *guest.Party              { return h.outdoor.Peek() }
func (h *fakeHost) TakeParty() *guest.Party              { return h.outdoor.Pop() }
func (h *fakeHost) LoadAreaFree() int                    { return h.capacity - h.loadArea }
func (h *fakeHost) Sample(d config.Dist) engine.Duration { return engine.Seconds(d.Mean) }
func (h *fakeHost) Release(p *guest.Party) {
	h.loadArea += p.Size
	h.released = append(h.released, p)
	if h.reenter != nil {
		h.reenter.Pull()
		h.reenter.Drain()
	}
}

// 1 room x 10 guests, show 100 s, turnover 20 s, max fill wait 30 s.
func newPreshow(capacity int) (*engine.Sim, *fakeHost, *Preshow) {
	sim := engine.New(1)
	h := &fakeHost{sim: sim, capacity: capacity}
	cfg := &config.Preshow{Enabled: true, Rooms: 1, BatchSize: 10, DurationS: config.Dist{Mean: 100}, TurnoverS: config.Dist{Mean: 20}, MaxFillWaitS: 30}
	return sim, h, NewPreshow(sim, cfg, h)
}

func arrive(sim *engine.Sim, h *fakeHost, p *Preshow, at float64, id int64, size int) {
	sim.At(engine.Time(engine.Seconds(at)), func() {
		h.outdoor.Push(&guest.Party{ID: id, Size: size})
		p.Pull()
	})
}

func eventsOf(sim *engine.Sim, typ string) []engine.Event {
	var out []engine.Event
	for _, e := range sim.Log.Events() {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func TestPreshowStartsWhenFull(t *testing.T) {
	sim, h, p := newPreshow(1000)
	p.Start()
	for i := 0; i < 5; i++ {
		arrive(sim, h, p, float64(i), int64(i+1), 2) // full (10) at t=4
	}
	sim.RunUntil(engine.Time(10 * engine.Minute))
	shows := eventsOf(sim, EvPreshowShow)
	if len(shows) != 1 || shows[0].T != engine.Time(4*engine.Second) || shows[0].N != 10 {
		t.Fatalf("shows %+v, want one full show at 4 s", shows)
	}
	if len(h.released) != 5 || h.released[0].ID != 1 {
		t.Fatalf("released %d parties", len(h.released))
	}
	if got := eventsOf(sim, EvPreshowEmpty); len(got) != 1 || got[0].T != engine.Time(104*engine.Second) {
		t.Fatalf("empty at %+v, want 104 s", got)
	}
}

// A party that would overfill the room closes the batch; it waits for the next room.
func TestPreshowNeverSplitsParties(t *testing.T) {
	sim, h, p := newPreshow(1000)
	p.Start()
	arrive(sim, h, p, 0, 1, 6)
	arrive(sim, h, p, 1, 2, 6)
	sim.RunUntil(engine.Time(10 * engine.Minute))
	shows := eventsOf(sim, EvPreshowShow)
	if len(shows) != 2 || shows[0].N != 6 || shows[0].T != engine.Time(engine.Second) {
		t.Fatalf("shows %+v: want a 6-guest show at 1 s (party 2 does not fit), then another", shows)
	}
	// second room cycle: refill after turnover at 1 + 100 + 20 = 121 s, party 2 already waiting,
	// then the 30 s max fill wait starts a partial show at 151 s
	if shows[1].T != engine.Time(151*engine.Second) || shows[1].N != 6 {
		t.Fatalf("second show %+v, want 6 guests at 151 s", shows[1])
	}
}

func TestPreshowPartialBatchAfterMaxFillWait(t *testing.T) {
	sim, h, p := newPreshow(1000)
	p.Start()
	arrive(sim, h, p, 5, 1, 3)
	sim.RunUntil(engine.Time(10 * engine.Minute))
	shows := eventsOf(sim, EvPreshowShow)
	if len(shows) != 1 || shows[0].T != engine.Time(30*engine.Second) || shows[0].N != 3 {
		t.Fatalf("shows %+v, want a 3-guest partial show at 30 s", shows)
	}
}

// Load area holds 4: the release stalls after two parties of 2 until space frees.
func TestPreshowReleaseBlocksOnFullLoadArea(t *testing.T) {
	sim, h, p := newPreshow(4)
	p.Start()
	for i := 0; i < 5; i++ {
		arrive(sim, h, p, 0, int64(i+1), 2)
	}
	sim.At(engine.Time(200*engine.Second), func() { h.loadArea = 0; p.Drain() }) // a train boards everyone
	sim.RunUntil(engine.Time(300 * engine.Second))
	if b := eventsOf(sim, EvPreshowBlocked); len(b) != 1 || b[0].T != engine.Time(100*engine.Second) || b[0].N != 6 {
		t.Fatalf("blocked %+v, want 6 guests stuck at 100 s", b)
	}
	if e := eventsOf(sim, EvPreshowEmpty); len(e) != 0 {
		t.Fatalf("room emptied at %v with only 4 more seats", e[0].T)
	}
	if len(h.released) != 4 {
		t.Fatalf("released %d parties, want 4 after one drain", len(h.released))
	}
}

// Regression: the coaster's Release boards guests and calls Drain again. A room must still empty once,
// giving one show per 100 s show + 20 s turnover cycle with saturated demand.
func TestPreshowReentrantHostEmptiesRoomOnce(t *testing.T) {
	sim, h, p := newPreshow(1 << 30)
	h.reenter = p
	p.Start()
	for i := 0; i < 2000; i++ {
		arrive(sim, h, p, 0, int64(i+1), 1)
	}
	sim.RunUntil(engine.Time(1200 * engine.Second))
	// shows at 0, 120, ..., 1200 s: 11
	if n := len(eventsOf(sim, EvPreshowShow)); n != 11 {
		t.Fatalf("%d shows in 1200 s, want 11", n)
	}
	if n := len(eventsOf(sim, EvPreshowEmpty)); n != 10 {
		t.Fatalf("%d room empties, want 10", n)
	}
}
