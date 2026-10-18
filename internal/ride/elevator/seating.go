package elevator

import "github.com/jasrags/ridesim/internal/guest"

// Seating follows the coaster rule: rows of seats_per_row, a party takes whole rows and is never split,
// spare seats in a party's last row go to a solo party at the head of the line or to single riders.
// Boarding stops at the first party that does not fit (no skipping).

type seatings struct {
	rowsFree int
	gaps     int
	riders   int
	parties  []*guest.Party
	fillWait bool // load_s is over; the cab waits up to max_wait_after_ready_s for more guests
}

func (r *Ride) newSeating() seatings {
	v := r.cfg.Vehicles
	return seatings{rowsFree: v.SeatsPerVehicle / max(1, v.SeatsPerRow)}
}

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
	stuck := head != nil && !fits && r.lines.PeekSingle() == nil
	return s.full() || stuck
}
