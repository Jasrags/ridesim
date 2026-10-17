package engine

import (
	"container/heap"
	"fmt"
	"math/rand/v2"
)

// Sim owns the clock, scheduler, RNG streams and event log for one run. It is single-threaded.
// A Sim may be a scope of another (Scope): scopes share everything but stamp their events with a source
// and draw from their own streams, so several rides can share one run.
type Sim struct {
	*core
	src string
}

type core struct {
	now   Time
	seq   uint64
	queue queue
	rng   rngSet
	Log   *Log
}

// New returns a simulation at t=0 with RNG streams derived from seed.
func New(seed uint64) *Sim {
	return &Sim{core: &core{rng: rngSet{seed: seed, streams: map[string]*rand.Rand{}}, Log: &Log{}}}
}

// Scope returns a view of s whose events carry Src = src (nested as "parent/src") and whose streams are
// named "src/name".
func (s *Sim) Scope(src string) *Sim {
	if s.src != "" {
		src = s.src + "/" + src
	}
	return &Sim{core: s.core, src: src}
}

// Src is the scope's source name ("" for the root).
func (s *Sim) Src() string { return s.src }

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

// RunBefore runs every action scheduled strictly before t, then sets the clock to t. Something done next
// happens at t ahead of every action already scheduled at t (live levers and scripted ops both use it, so a
// played session and its exported rerun interleave identically).
func (s *Sim) RunBefore(t Time) {
	for s.queue.Len() > 0 && s.queue[0].at < t {
		it := heap.Pop(&s.queue).(*item)
		if it.cancelled {
			continue
		}
		s.now = it.at
		it.fn()
	}
	if t > s.now {
		s.now = t
	}
}

// Stream returns the named RNG stream.
func (s *Sim) Stream(name string) *rand.Rand {
	if s.src != "" {
		name = s.src + "/" + name
	}
	return s.rng.stream(name)
}

// Emit appends e to the log, stamped with the current time.
func (s *Sim) Emit(e Event) {
	e.T = s.now
	if e.Src == "" {
		e.Src = s.src
	}
	s.Log.append(e)
}
