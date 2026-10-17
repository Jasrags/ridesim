// Package queue holds ride queues. M1 has the standby FIFO; single rider, preshow batch stages and
// the Lightning Lane merge arrive in M3/M4.
package queue

import "github.com/jasrags/ridesim/internal/guest"

// FIFO is a first-in first-out line of parties.
type FIFO struct {
	parties []*guest.Party
	head    int
	guests  int
}

// Push adds p to the back.
func (q *FIFO) Push(p *guest.Party) {
	q.parties = append(q.parties, p)
	q.guests += p.Size
}

// Peek returns the front party or nil.
func (q *FIFO) Peek() *guest.Party {
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
	if q.head > 1024 && q.head*2 > len(q.parties) {
		q.parties = append([]*guest.Party(nil), q.parties[q.head:]...)
		q.head = 0
	}
	return p
}

// Parties is the number of parties waiting.
func (q *FIFO) Parties() int { return len(q.parties) - q.head }

// Guests is the number of guests waiting.
func (q *FIFO) Guests() int { return q.guests }
