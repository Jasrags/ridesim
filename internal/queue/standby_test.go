package queue

import (
	"testing"

	"github.com/jasrags/ridesim/internal/guest"
)

func TestFIFOOrderAndCounts(t *testing.T) {
	var q FIFO
	for i := int64(1); i <= 3000; i++ {
		q.Push(&guest.Party{ID: i, Size: 2})
	}
	for i := int64(1); i <= 2500; i++ {
		if p := q.Pop(); p.ID != i {
			t.Fatalf("pop %d got id %d", i, p.ID)
		}
	}
	if q.Parties() != 500 || q.Guests() != 1000 || q.Peek().ID != 2501 {
		t.Fatalf("parties %d guests %d peek %d", q.Parties(), q.Guests(), q.Peek().ID)
	}
	for q.Pop() != nil {
	}
	if q.Peek() != nil || q.Guests() != 0 {
		t.Fatal("queue not empty")
	}
}

func TestFIFORemoveIsLazy(t *testing.T) {
	var q FIFO
	ps := make([]*guest.Party, 5)
	for i := range ps {
		ps[i] = &guest.Party{ID: int64(i + 1), Size: 3}
		q.Push(ps[i])
	}
	q.Remove(ps[0])
	q.Remove(ps[2])
	q.Remove(ps[2]) // idempotent
	if q.Parties() != 3 || q.Guests() != 9 {
		t.Fatalf("parties %d guests %d, want 3 and 9", q.Parties(), q.Guests())
	}
	for _, want := range []int64{2, 4, 5} {
		if p := q.Pop(); p == nil || p.ID != want {
			t.Fatalf("pop got %v, want %d", p, want)
		}
	}
	if q.Peek() != nil || q.Guests() != 0 || q.Parties() != 0 {
		t.Fatal("queue not empty")
	}
}
