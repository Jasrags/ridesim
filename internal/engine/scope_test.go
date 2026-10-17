package engine

import "testing"

func TestScopeSharesClockAndLogAndStampsSource(t *testing.T) {
	s := New(1)
	a, b := s.Scope("a"), s.Scope("b")
	a.After(2*Second, func() { a.Emit(Event{Type: "x"}) })
	b.After(1*Second, func() { b.Emit(Event{Type: "y"}) })
	s.Emit(Event{Type: "root"})
	s.RunUntil(Time(10 * Second))
	ev := s.Log.Events()
	if len(ev) != 3 || ev[1].Src != "b" || ev[2].Src != "a" || ev[0].Src != "" {
		t.Fatalf("events = %+v", ev)
	}
	if a.Now() != s.Now() {
		t.Fatalf("scopes must share the clock")
	}
}

func TestScopeStreamsAreSeparate(t *testing.T) {
	s := New(7)
	x := s.Scope("a").Stream(StreamArrivals).Uint64()
	y := s.Scope("b").Stream(StreamArrivals).Uint64()
	z := New(7).Stream("a/" + StreamArrivals).Uint64()
	if x == y || x != z {
		t.Fatalf("scoped streams: a=%d b=%d expected a == unscoped a/arrivals=%d", x, y, z)
	}
	if s.Scope("a").Scope("c").Src() != "a/c" {
		t.Fatalf("nested scope src = %q", s.Scope("a").Scope("c").Src())
	}
}

// RunBefore runs everything strictly before t and leaves the clock at t, so an action applied then precedes
// every event already scheduled at t.
func TestRunBeforeStopsShortOfEventsAtT(t *testing.T) {
	s := New(1)
	var order []string
	s.At(Time(Second), func() { order = append(order, "a") })
	s.At(Time(2*Second), func() { order = append(order, "b") })
	s.RunBefore(Time(2 * Second))
	if s.Now() != Time(2*Second) || len(order) != 1 {
		t.Fatalf("now %v order %v", s.Now(), order)
	}
	order = append(order, "op")
	s.RunUntil(Time(3 * Second))
	if len(order) != 3 || order[1] != "op" || order[2] != "b" {
		t.Fatalf("order %v, want a, op, b", order)
	}
}
