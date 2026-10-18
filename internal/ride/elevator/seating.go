package elevator

import "github.com/jasrags/ridesim/internal/guest"

// Seating follows the coaster rule: rows of seats_per_row, a party takes whole rows and is never split,
// spare seats in a party's last row go to a solo party at the head of the line or to single riders.
// Boarding stops at the first party that does not fit (no skipping).

type seatings struct {
	groupsFree []int // free seats per boarding group (vehicles.boarding_groups only); gaps is their sum
	rowsFree   int
	gaps       int
	riders     int
	parties    []*guest.Party
	fillWait   bool // load_s is over; the cab waits up to max_wait_after_ready_s for more guests
}

func (r *Ride) newSeating() seatings {
	v := r.cfg.Vehicles
	if len(v.BoardingGroups) > 0 {
		return seatings{groupsFree: append([]int(nil), v.BoardingGroups...), gaps: v.SeatsPerVehicle}
	}
	return seatings{rowsFree: v.SeatsPerVehicle / max(1, v.SeatsPerRow)}
}

func (r *Ride) groupMode() bool { return len(r.cfg.Vehicles.BoardingGroups) > 0 }

func (s *seatings) full() bool { return s.rowsFree == 0 && s.gaps == 0 }

func (r *Ride) rowsFor(size int) int {
	spr := max(1, r.cfg.Vehicles.SeatsPerRow)
	return (size + spr - 1) / spr
}

// Board implements lines.Ride: the load line changed, so seat whoever fits in the cabs at load, oldest
// cab first.
func (r *Ride) Board() {
	for _, c := range append([]*cab(nil), r.loading...) {
		if r.paused(c) || !r.atLoad(c) { // a nested Board may have sent it on already
			continue
		}
		r.boardCab(c)
		if !r.atLoad(c) { // boarding re-entered Board, which already started the check
			continue
		}
		if c.seat.fillWait && r.fillWaitOver(c) {
			r.cancelWork(c)
			r.startCheck(c)
		}
	}
}

func (r *Ride) boardCab(c *cab) {
	if r.paused(c) {
		return
	}
	if r.groupMode() {
		r.boardGroups(c)
		return
	}
	s := &c.seat
	spr := max(1, r.cfg.Vehicles.SeatsPerRow)
	boarded := false
	for {
		p, line := r.lines.Head()
		if p == nil {
			break
		}
		if p.Size == 1 && s.gaps > 0 {
			r.lines.Pop(line)
			s.gaps--
			r.seatParty(c, p)
			boarded = true
			continue
		}
		need := r.rowsFor(p.Size)
		if need > s.rowsFree {
			break
		}
		r.lines.Pop(line)
		s.rowsFree -= need
		s.gaps += need*spr - p.Size
		r.seatParty(c, p)
		boarded = true
	}
	for s.gaps > 0 || (s.fillWait && s.rowsFree > 0) {
		p := r.lines.PopSingle()
		if p == nil {
			break
		}
		if s.gaps > 0 {
			s.gaps--
		} else {
			s.rowsFree--
			s.gaps += spr - 1
		}
		r.seatParty(c, p)
		boarded = true
	}
	if boarded {
		r.lines.Boarded()
	}
}

func (r *Ride) seatParty(c *cab, p *guest.Party) {
	r.lines.Seated(p)
	c.seat.riders += p.Size
	c.seat.parties = append(c.seat.parties, p)
	r.sim.Emit(engineBoard(c, p))
}

// fillWaitOver: the cab is full, or nobody in line can take a free seat (the next party needs more rows
// than remain and no single rider waits). An empty line keeps waiting: a newcomer could still board.
func (r *Ride) fillWaitOver(c *cab) bool {
	s := &c.seat
	head, _ := r.lines.Head()
	fits := head != nil && (r.rowsFor(head.Size) <= s.rowsFree || (head.Size == 1 && s.gaps > 0))
	if r.groupMode() {
		fits = head != nil && head.Size <= s.gaps
	}
	stuck := head != nil && !fits && r.lines.PeekSingle() == nil
	return s.full() || stuck
}

// Boarding groups and callouts ([user] Tower of Terror): cast keep a party together in one group when one
// has room for it, otherwise split it; spare seats go to the next party in line, to single riders, then
// to a party called forward from up to callout_parties back that fits. Cabs leave nearly full.
func (r *Ride) boardGroups(c *cab) {
	s := &c.seat
	boarded := false
	take := func(p *guest.Party) {
		placeInGroups(s.groupsFree, p.Size)
		s.gaps -= p.Size
		r.seatParty(c, p)
		boarded = true
	}
	for {
		p, line := r.lines.Head()
		if p == nil || p.Size > s.gaps {
			break
		}
		r.lines.Pop(line)
		take(p)
	}
	for s.gaps > 0 && r.lines.PeekSingle() != nil {
		take(r.lines.PopSingle())
	}
	for w := r.cfg.Cycle.CalloutParties; w > 0 && s.gaps > 0; {
		p := r.lines.CallOut(s.gaps, w)
		if p == nil {
			break
		}
		take(p)
	}
	if boarded {
		r.lines.Boarded()
	}
}

// placeInGroups takes size seats from free: the smallest group that holds them all (front first on a
// tie), else from the groups with the most free seats (front first on a tie). The caller checked that
// the cab has size free seats.
func placeInGroups(free []int, size int) {
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
