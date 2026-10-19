// Package queue holds ride queues: FIFO lines with lazy deletion (reneging), the preshow batch
// stage, and the Lightning Lane merge.
package queue

import "github.com/jasrags/ridesim/internal/guest"

// FIFO is a first-in first-out line of parties. Parties marked Gone are skipped and dropped lazily.
type FIFO struct {
	parties []*guest.Party
	head    int
	guests  int
	count   int
}

// Push adds p to the back.
func (q *FIFO) Push(p *guest.Party) {
	q.parties = append(q.parties, p)
	q.guests += p.Size
	q.count++
}

func (q *FIFO) skipGone() {
	for q.head < len(q.parties) && q.parties[q.head].Gone {
		q.parties[q.head] = nil
		q.head++
	}
}

// Peek returns the front party or nil.
func (q *FIFO) Peek() *guest.Party {
	q.skipGone()
	if q.head == len(q.parties) {
		return nil
	}
	return q.parties[q.head]
}

// Pop removes and returns the front party, or nil.
func (q *FIFO) Pop() *guest.Party {
	p := q.Peek()
	if p == nil {
		return nil
	}
	q.parties[q.head] = nil
	q.head++
	q.guests -= p.Size
	q.count--
	if q.head > 1024 && q.head*2 > len(q.parties) {
		q.parties = append([]*guest.Party(nil), q.parties[q.head:]...)
		q.head = 0
	}
	return p
}

// Remove marks p Gone (it reneged) and drops it from the counts. p must be in this queue.
func (q *FIFO) Remove(p *guest.Party) {
	if p.Gone {
		return
	}
	p.Gone = true
	q.guests -= p.Size
	q.count--
}

// TakeFirst removes and returns the first of the front `window` waiting parties for which ok is true,
// or nil. The parties ahead of it keep their places (a cast member's callout for a party that fits).
func (q *FIFO) TakeFirst(window int, ok func(*guest.Party) bool) *guest.Party {
	seen := 0
	for i := q.head; i < len(q.parties) && seen < window; i++ {
		p := q.parties[i]
		if p == nil || p.Gone {
			continue
		}
		seen++
		if !ok(p) {
			continue
		}
		if i == q.head {
			return q.Pop()
		}
		copy(q.parties[i:], q.parties[i+1:])
		q.parties = q.parties[:len(q.parties)-1]
		q.guests -= p.Size
		q.count--
		return p
	}
	return nil
}

// Parties is the number of parties waiting.
func (q *FIFO) Parties() int { return q.count }

// Guests is the number of guests waiting.
func (q *FIFO) Guests() int { return q.guests }
