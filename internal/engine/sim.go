package engine

import (
	"container/heap"
	"fmt"
	"math/rand/v2"
)

// Sim owns the clock, scheduler, RNG streams and event log for one run. It is single-threaded.
type Sim struct {
	now   Time
	seq   uint64
	queue queue
	rng   rngSet
	Log   *Log
}

// New returns a simulation at t=0 with RNG streams derived from seed.
func New(seed uint64) *Sim {
	return &Sim{rng: rngSet{seed: seed, streams: map[string]*rand.Rand{}}, Log: &Log{}}
}

// Now is the current simulated time.
func (s *Sim) Now() Time { return s.now }

// At schedules fn at absolute time t, which must not be in the past.
func (s *Sim) At(t Time, fn func()) Handle {
	if t < s.now {
		panic(fmt.Sprintf("engine: schedule at %v before now %v", t, s.now))
	}
	s.seq++
	it := &item{at: t, seq: s.seq, fn: fn}
	heap.Push(&s.queue, it)
	return Handle{it}
}

// After schedules fn d from now.
func (s *Sim) After(d Duration, fn func()) Handle { return s.At(s.now.Add(d), fn) }

// RunUntil runs every action scheduled at or before end, then sets the clock to end.
func (s *Sim) RunUntil(end Time) {
	for s.queue.Len() > 0 && s.queue[0].at <= end {
		it := heap.Pop(&s.queue).(*item)
		if it.cancelled {
			continue
		}
		s.now = it.at
		it.fn()
	}
	if end > s.now {
		s.now = end
	}
}

// Stream returns the named RNG stream.
func (s *Sim) Stream(name string) *rand.Rand { return s.rng.stream(name) }

// Emit appends e to the log, stamped with the current time.
func (s *Sim) Emit(e Event) {
	e.T = s.now
	s.Log.append(e)
}
