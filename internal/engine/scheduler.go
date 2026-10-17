package engine

// item is one scheduled action. seq is assigned at schedule time, so actions at the same time run
// in the order they were scheduled.
type item struct {
	at        Time
	seq       uint64
	fn        func()
	cancelled bool
}

type queue []*item

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	return q[i].seq < q[j].seq
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(*item)) }
func (q *queue) Pop() any {
	old := *q
	it := old[len(old)-1]
	old[len(old)-1] = nil
	*q = old[:len(old)-1]
	return it
}

// Handle cancels a scheduled action. The zero Handle is valid and cancels nothing.
type Handle struct{ it *item }

// Cancel prevents the action from running. Cancelling twice or after it ran is a no-op.
func (h Handle) Cancel() {
	if h.it != nil {
		h.it.cancelled = true
	}
}
