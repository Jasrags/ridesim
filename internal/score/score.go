// Package score turns a park day's event log into the guest-experience score (M15, decision 2026-10-19):
// points per guest for rides, shows and meets completed, less penalties for waits past a comfort
// threshold, balks and reneges, walking, jammed walkways and lapsed Lightning Lane passes. It reads the
// log only, one event at a time, so a live session can keep a running score.
package score

import (
	"fmt"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/park"
)

// Card is the day so far. Points are totals over all guests; penalties are positive and subtracted.
type Card struct {
	Guests      int // guests who entered the park
	Experiences int // guest visits completed at rides, shows and meets
	Balks       int // guests who turned away on arrival at a ride, show or meet
	Reneges     int // guests who left a ride, show or meet line
	LLLapsed    int // passes that lapsed unused

	WaitMin     float64 // guest-minutes waited at rides and meets
	WaitOverMin float64 // guest-minutes past the comfort threshold
	WalkMin     float64 // guest-minutes walking
	JamMin      float64 // guest-minutes waiting to enter a jammed walkway

	Experience, WaitPenalty, BalkPenalty, RenegePenalty, WalkPenalty, JamPenalty, LapsePenalty float64
}

// Total is the day's points over all guests.
func (c Card) Total() float64 {
	return c.Experience - c.WaitPenalty - c.BalkPenalty - c.RenegePenalty - c.WalkPenalty - c.JamPenalty - c.LapsePenalty
}

// Score is points per guest (0 before anyone arrives).
func (c Card) Score() float64 { return c.per(c.Total()) }

// ExperiencesPerGuest is completed visits per guest.
func (c Card) ExperiencesPerGuest() float64 { return c.per(float64(c.Experiences)) }

func (c Card) per(v float64) float64 {
	if c.Guests == 0 {
		return 0
	}
	return v / float64(c.Guests)
}

// node classes that earn points
const (
	classNone = iota
	classRide
	classShow
	classMeet
)

// waiting is a queued visit: when it joined and how many of the party have not yet boarded (a party can
// be split across vehicles).
type waiting struct {
	since engine.Time
	left  int
}

type visitKey struct {
	src   string
	guest int64
}

// Scorer accumulates a Card from events in log order.
type Scorer struct {
	w    config.Scoring
	card Card

	class   map[string]int        // scope (node id) -> class, from each scope's run_start
	size    map[int64]int         // park party id -> size
	joined  map[visitKey]*waiting // ride and meet queue joins not yet boarded or reneged
	blocked map[int64]engine.Time // parties waiting at a jammed walkway since
}

// New starts an empty score with weights w.
func New(w config.Scoring) *Scorer {
	return &Scorer{w: w, class: map[string]int{}, size: map[int64]int{}, joined: map[visitKey]*waiting{}, blocked: map[int64]engine.Time{}}
}

// Compute scores a whole log.
func Compute(events []engine.Event, w config.Scoring) Card {
	s := New(w)
	for _, e := range events {
		s.Add(e)
	}
	return s.Card()
}

// Card is the score so far.
func (s *Scorer) Card() Card { return s.card }

// Add reads the next event of the log.
func (s *Scorer) Add(e engine.Event) {
	if e.Src == park.Scope {
		s.parkEvent(e)
		return
	}
	if e.Src == "" {
		return
	}
	switch e.Type {
	case engine.EvRunStart:
		s.class[e.Src] = classOf(info(e.Info, "archetype"))
	case engine.EvQueueJoin:
		if c := s.class[e.Src]; c == classRide || c == classMeet {
			s.joined[visitKey{e.Src, e.Guest}] = &waiting{e.T, e.N}
		}
	case engine.EvBoard:
		s.endWait(e)
	case engine.EvRenege:
		s.endWait(e)
		if s.class[e.Src] == classNone {
			return
		}
		s.card.Reneges += e.N
		s.card.RenegePenalty += s.w.RenegePenalty * float64(e.N)
	case engine.EvBalk:
		if s.class[e.Src] == classNone {
			return // dining and shops earn nothing, so turning away there costs nothing either
		}
		s.card.Balks += e.N
		s.card.BalkPenalty += s.w.BalkPenalty * float64(e.N)
	}
}

// endWait closes a queued visit's wait: boarded or reneged.
func (s *Scorer) endWait(e engine.Event) {
	k := visitKey{e.Src, e.Guest}
	w, ok := s.joined[k]
	if !ok {
		return
	}
	if w.left -= e.N; w.left <= 0 {
		delete(s.joined, k)
	}
	n := float64(e.N)
	wait := e.T.Sub(w.since).Seconds() / 60
	over := max(0, wait-s.w.WaitComfortMin)
	s.card.WaitMin += wait * n
	s.card.WaitOverMin += over * n
	s.card.WaitPenalty += s.w.WaitPenaltyPerMin * over * n
}

func (s *Scorer) parkEvent(e engine.Event) {
	switch e.Type {
	case park.EvParkArrive:
		s.size[e.Guest] = e.N
		s.card.Guests += e.N
	case park.EvVisitEnd:
		if e.Info != "rode" {
			return
		}
		n := s.size[e.Guest]
		var pts float64
		switch s.class[e.Block] {
		case classRide:
			pts = s.w.Experience.Ride
		case classShow:
			pts = s.w.Experience.Show
		case classMeet:
			pts = s.w.Experience.Meet
		default:
			return
		}
		s.card.Experiences += n
		s.card.Experience += pts * float64(n)
	case park.EvEdgeBlocked:
		if _, ok := s.blocked[e.Guest]; !ok {
			s.blocked[e.Guest] = e.T
		}
	case park.EvWalk:
		n := float64(e.N)
		if since, ok := s.blocked[e.Guest]; ok {
			delete(s.blocked, e.Guest)
			jam := e.T.Sub(since).Seconds() / 60
			s.card.JamMin += jam * n
			s.card.JamPenalty += s.w.JamPenaltyPerMin * jam * n
		}
		walk := e.D.Seconds() / 60
		s.card.WalkMin += walk * n
		s.card.WalkPenalty += s.w.WalkPenaltyPerMin * walk * n
	case park.EvLLLapse:
		n := s.size[e.Guest]
		s.card.LLLapsed++
		s.card.LapsePenalty += s.w.LLLapsePenalty * float64(n)
	}
}

func classOf(archetype string) int {
	switch archetype {
	case config.ArchetypeMultiTrainCoaster, config.ArchetypeContinuousLoad, config.ArchetypeElevatorDrop, config.ArchetypeSimulatorBays, config.NodeRideSimple:
		return classRide
	case config.NodeShow:
		return classShow
	case config.NodeMeet:
		return classMeet
	}
	return classNone
}

// info returns key's value from a "k=v k=v" run_start info string.
func info(s, key string) string {
	for _, f := range strings.Fields(s) {
		if k, v, ok := strings.Cut(f, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// String renders the card as text.
func (c Card) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "score %.2f points per guest (%d guests)\n", c.Score(), c.Guests)
	fmt.Fprintf(&b, "  + experiences   %8.2f   %d visits, %.2f per guest\n", c.per(c.Experience), c.Experiences, c.ExperiencesPerGuest())
	fmt.Fprintf(&b, "  - waits         %8.2f   %.0f min per guest waited, %.0f past comfort\n", c.per(c.WaitPenalty), c.per(c.WaitMin), c.per(c.WaitOverMin))
	fmt.Fprintf(&b, "  - balks         %8.2f   %d guests\n", c.per(c.BalkPenalty), c.Balks)
	fmt.Fprintf(&b, "  - reneges       %8.2f   %d guests\n", c.per(c.RenegePenalty), c.Reneges)
	fmt.Fprintf(&b, "  - walking       %8.2f   %.0f min per guest\n", c.per(c.WalkPenalty), c.per(c.WalkMin))
	fmt.Fprintf(&b, "  - jams          %8.2f   %.1f min per guest\n", c.per(c.JamPenalty), c.per(c.JamMin))
	fmt.Fprintf(&b, "  - lapsed LL     %8.2f   %d passes\n", c.per(c.LapsePenalty), c.LLLapsed)
	return b.String()
}
