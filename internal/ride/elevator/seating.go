package elevator

import (
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/seats"
)

// Seating rules live in package seats (rows, or [user] boarding groups with callouts).

func (r *Ride) seatRules() seats.Rules {
	v := r.cfg.Vehicles
	return seats.Rules{Seats: v.SeatsPerVehicle, SeatsPerRow: v.SeatsPerRow, Groups: v.BoardingGroups, CalloutParties: r.cfg.Cycle.CalloutParties}
}

func (r *Ride) newSeating() seats.Load { return r.seatRules().New() }

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
		if c.seat.FillWait && r.fillWaitOver(c) {
			r.cancelWork(c)
			r.startCheck(c)
		}
	}
}

func (r *Ride) boardCab(c *cab) {
	if r.paused(c) {
		return
	}
	r.seatRules().Board(&c.seat, r.lines, func(p *guest.Party) { r.sim.Emit(engineBoard(c, p)) })
}

func (r *Ride) fillWaitOver(c *cab) bool { return r.seatRules().FillWaitOver(&c.seat, r.lines) }
