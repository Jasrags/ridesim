package engine

import (
	"bytes"
	"reflect"
	"testing"
)

func TestSchedulerOrdersByTimeThenSeq(t *testing.T) {
	s := New(1)
	var got []string
	rec := func(name string) func() { return func() { got = append(got, name) } }
	s.At(200, rec("c"))
	s.At(100, rec("a"))
	s.At(200, rec("d")) // same time as c, scheduled later -> runs after c
	s.At(100, rec("b"))
	s.RunUntil(1000)
	if want := []string{"a", "b", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	if s.Now() != 1000 {
		t.Fatalf("now = %d, want 1000", s.Now())
	}
}

func TestSchedulerSameTimeChildRunsAfterQueuedPeers(t *testing.T) {
	s := New(1)
	var got []string
	s.At(10, func() {
		got = append(got, "a")
		s.After(0, func() { got = append(got, "a-child") })
	})
	s.At(10, func() { got = append(got, "b") })
	s.RunUntil(10)
	if want := []string{"a", "b", "a-child"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
}

func TestCancel(t *testing.T) {
	s := New(1)
	ran := false
	h := s.At(5, func() { ran = true })
	h.Cancel()
	s.RunUntil(10)
	if ran {
		t.Fatal("cancelled action ran")
	}
}

func TestRunUntilLeavesLaterEvents(t *testing.T) {
	s := New(1)
	n := 0
	s.At(5, func() { n++ })
	s.At(15, func() { n++ })
	s.RunUntil(10)
	if n != 1 {
		t.Fatalf("ran %d actions by t=10, want 1", n)
	}
	s.RunUntil(20)
	if n != 2 {
		t.Fatalf("ran %d actions by t=20, want 2", n)
	}
}

func TestSchedulingInThePastPanics(t *testing.T) {
	s := New(1)
	s.RunUntil(100)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	s.At(50, func() {})
}

func draw(s *Sim, name string, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = s.Stream(name).Uint64()
	}
	return out
}

func TestStreamsAreIndependent(t *testing.T) {
	a := New(42)
	b := New(42)
	draw(b, StreamFailures, 1000) // heavy use of one stream must not shift another
	if !reflect.DeepEqual(draw(a, StreamArrivals, 10), draw(b, StreamArrivals, 10)) {
		t.Fatal("arrivals stream shifted by use of failures stream")
	}
	if reflect.DeepEqual(draw(New(42), StreamArrivals, 10), draw(New(42), StreamStationTimes, 10)) {
		t.Fatal("different streams produced identical draws")
	}
	if reflect.DeepEqual(draw(New(1), StreamArrivals, 10), draw(New(2), StreamArrivals, 10)) {
		t.Fatal("different seeds produced identical draws")
	}
}

func TestEventLogRoundTrip(t *testing.T) {
	s := New(1)
	s.At(1500, func() { s.Emit(Event{Type: EvTrainEnter, Train: 2, Block: "LIFT"}) })
	s.At(2500, func() { s.Emit(Event{Type: EvBoard, Guest: 7, Train: 2, N: 1}) })
	s.RunUntil(3000)

	var buf bytes.Buffer
	if err := s.Log.Write(&buf); err != nil {
		t.Fatal(err)
	}
	want := "{\"t\":1500,\"type\":\"train_enter\",\"train\":2,\"block\":\"LIFT\"}\n{\"t\":2500,\"type\":\"board\",\"train\":2,\"guest\":7,\"n\":1}\n"
	if buf.String() != want {
		t.Fatalf("encoded log:\n%s\nwant:\n%s", buf.String(), want)
	}
	back, err := ReadLog(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, s.Log.Events()) {
		t.Fatalf("replayed %v, want %v", back, s.Log.Events())
	}
}

func TestTimeConversions(t *testing.T) {
	if got := Seconds(54.0); got != 54000 {
		t.Fatalf("Seconds(54) = %d", got)
	}
	if got := Seconds(0.5); got != 500 {
		t.Fatalf("Seconds(0.5) = %d", got)
	}
	if got := Duration(1500).Seconds(); got != 1.5 {
		t.Fatalf("Seconds() = %v", got)
	}
}
