package queue

import (
	"testing"

	"github.com/jasrags/ridesim/internal/guest"
)

// TakeFirst pulls the first party that fits from within the window and leaves the ones ahead in place.
func TestTakeFirstPullsFromWithinTheWindow(t *testing.T) {
	q := &FIFO{}
	sizes := []int{4, 4, 2, 1}
	ps := make([]*guest.Party, len(sizes))
	for i, s := range sizes {
		ps[i] = &guest.Party{ID: int64(i + 1), Size: s}
		q.Push(ps[i])
	}
	q.Remove(ps[1]) // reneged: skipped, not counted in the window
	fits2 := func(p *guest.Party) bool { return p.Size <= 2 }
	if got := q.TakeFirst(1, fits2); got != nil {
		t.Fatalf("window 1 took party %d; only the head (size 4) is in reach", got.ID)
	}
	if got := q.TakeFirst(2, fits2); got != ps[2] {
		t.Fatalf("window 2 took %v, want party 3 (size 2)", got)
	}
	if q.Guests() != 5 || q.Parties() != 2 || q.Peek() != ps[0] {
		t.Fatalf("after the callout: %d guests in %d parties, head %v; want 5 in 2, head party 1", q.Guests(), q.Parties(), q.Peek())
	}
	if q.Pop() != ps[0] || q.Pop() != ps[3] || q.Pop() != nil {
		t.Fatal("order after the callout changed")
	}
}
