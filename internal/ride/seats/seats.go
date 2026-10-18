// Package seats seats parties from a ride's load line into one vehicle (a cab, a simulator bay) that
// loads over a boarding window. Two rules:
//
//   - Rows (the coaster rule): rows of SeatsPerRow; a party takes whole rows and is never split; spare
//     seats in a party's last row go to a solo party at the head of the line or to single riders, who may
//     also take whole empty rows once the window is over. Boarding stops at the first party that does not
//     fit (no skipping).
//   - Boarding groups ([user] Tower of Terror): a party sits in the smallest group with room for it, else
//     it is split over the groups with the most free seats; spare seats go to the next party in line, to
//     single riders, then to a party called forward from up to CalloutParties back.
package seats

import (
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/lines"
)

// Rules describe one vehicle's seats.
type Rules struct {
	Seats          int
	SeatsPerRow    int
	Groups         []int // boarding groups, front to back; empty = the row rule
	CalloutParties int   // boarding groups only
}

// Load is one vehicle's seating during a load.
type Load struct {
	groupsFree []int
	rowsFree   int
	gaps       int // spare seats in partly filled rows; with groups, every free seat
	Riders     int
	Parties    []*guest.Party
	FillWait   bool // the boarding window is over; the vehicle waits a little longer for more guests
}

// New starts an empty load.
func (r Rules) New() Load {
	if len(r.Groups) > 0 {
		return Load{groupsFree: append([]int(nil), r.Groups...), gaps: r.Seats}
	}
	return Load{rowsFree: r.Seats / max(1, r.SeatsPerRow)}
}

// Full reports whether every seat is taken.
func (l *Load) Full() bool { return l.rowsFree == 0 && l.gaps == 0 }

func (r Rules) rowsFor(size int) int {
	spr := max(1, r.SeatsPerRow)
	return (size + spr - 1) / spr
}

// Board seats whoever fits from ln into l. Each seated party is marked Seated, added to l, then passed
// to onSeat (which logs it). If anyone boarded, ln.Boarded is called last.
func (r Rules) Board(l *Load, ln *lines.Lines, onSeat func(*guest.Party)) {
	boarded := false
	seat := func(p *guest.Party) {
		ln.Seated(p)
		l.Riders += p.Size
		l.Parties = append(l.Parties, p)
		onSeat(p)
		boarded = true
	}
	if len(r.Groups) > 0 {
		r.boardGroups(l, ln, seat)
	} else {
		r.boardRows(l, ln, seat)
	}
	if boarded {
		ln.Boarded()
	}
}

func (r Rules) boardRows(l *Load, ln *lines.Lines, seat func(*guest.Party)) {
	spr := max(1, r.SeatsPerRow)
	for {
		p, line := ln.Head()
		if p == nil {
			break
		}
		if p.Size == 1 && l.gaps > 0 {
			ln.Pop(line)
			l.gaps--
			seat(p)
			continue
		}
		need := r.rowsFor(p.Size)
		if need > l.rowsFree {
			break
		}
		ln.Pop(line)
		l.rowsFree -= need
		l.gaps += need*spr - p.Size
		seat(p)
	}
	for l.gaps > 0 || (l.FillWait && l.rowsFree > 0) {
		p := ln.PopSingle()
		if p == nil {
			break
		}
		if l.gaps > 0 {
			l.gaps--
		} else {
			l.rowsFree--
			l.gaps += spr - 1
		}
		seat(p)
	}
}

func (r Rules) boardGroups(l *Load, ln *lines.Lines, seat func(*guest.Party)) {
	take := func(p *guest.Party) {
		PlaceInGroups(l.groupsFree, p.Size)
		l.gaps -= p.Size
		seat(p)
	}
	for {
		p, line := ln.Head()
		if p == nil || p.Size > l.gaps {
			break
		}
		ln.Pop(line)
		take(p)
	}
	for l.gaps > 0 && ln.PeekSingle() != nil {
		take(ln.PopSingle())
	}
	for w := r.CalloutParties; w > 0 && l.gaps > 0; {
		p := ln.CallOut(l.gaps, w)
		if p == nil {
			break
		}
		take(p)
	}
}

// FillWaitOver: the vehicle is full, or nobody in line can take a free seat (the next party does not
// fit and no single rider waits). An empty line keeps waiting: a newcomer could still board.
func (r Rules) FillWaitOver(l *Load, ln *lines.Lines) bool {
	head, _ := ln.Head()
	fits := head != nil && (r.rowsFor(head.Size) <= l.rowsFree || (head.Size == 1 && l.gaps > 0))
	if len(r.Groups) > 0 {
		fits = head != nil && head.Size <= l.gaps
	}
	stuck := head != nil && !fits && ln.PeekSingle() == nil
	return l.Full() || stuck
}

// PlaceInGroups takes size seats from free: the smallest group that holds them all (front first on a
// tie), else from the groups with the most free seats (front first on a tie). The caller checked that
// the vehicle has size free seats.
func PlaceInGroups(free []int, size int) {
	best := -1
	for i, f := range free {
		if f >= size && (best < 0 || f < free[best]) {
			best = i
		}
	}
	if best >= 0 {
		free[best] -= size
		return
	}
	for size > 0 {
		most := 0
		for i, f := range free {
			if f > free[most] {
				most = i
			}
		}
		take := min(size, free[most])
		free[most] -= take
		size -= take
	}
}
