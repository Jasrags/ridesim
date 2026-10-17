package coaster

import (
	"testing"

	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// A seated party is told it rode once its train finishes unloading, after the board time.
func TestPartyIsToldItRodeAfterUnload(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true}, 0, engine.Hour)
	var exits []guest.Exit
	var at engine.Time
	p := arriveAt(h, 1, 2, guest.Standby, 0)
	p.OnExit = func(e guest.Exit) { exits, at = append(exits, e), h.sim.Now() }
	h.sim.RunUntil(engine.Time(engine.Hour))
	var boardAt, unloadAt engine.Time = -1, -1
	for _, e := range h.sim.Log.Events() {
		if e.Type == engine.EvBoard && e.Guest == p.ID {
			boardAt = e.T
		}
		if e.Type == engine.EvUnload && boardAt >= 0 && unloadAt < 0 {
			unloadAt = e.T
		}
	}
	if len(exits) != 1 || exits[0] != guest.Rode || unloadAt < 0 || at < unloadAt {
		t.Fatalf("exits %v at %v (board %v unload %v), want one rode at or after unload", exits, at, boardAt, unloadAt)
	}
}

// A balking party is told it balked; it never boards.
func TestBalkingPartyIsToldItBalked(t *testing.T) {
	h := newHarness(t, fixture(t), Options{Trains: 1, FixedTimes: true}, 0, engine.Hour)
	for i := 0; i < 400; i++ {
		arriveAt(h, 1, 2, guest.Standby, 0)
	}
	var exits []guest.Exit
	p := arriveAt(h, 2, 2, guest.Standby, 1) // 1 min patience against a long line
	p.OnExit = func(e guest.Exit) { exits = append(exits, e) }
	h.sim.RunUntil(sec(3))
	if len(exits) != 1 || exits[0] != guest.Balked {
		t.Fatalf("exits %v, want [balked]", exits)
	}
}
