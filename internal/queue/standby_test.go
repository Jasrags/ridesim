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
