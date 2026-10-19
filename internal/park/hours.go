package park

import (
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

// Node hours (M17): an activity can open later or close earlier than the park (EPCOT's World Showcase opens at
// 11:00). Before opening it is closed on the board and guests do not choose it; modeled rides stop with cause
// area_hours and simple rides and meets log the same cause, so posted-wait stats skip the closed time as for
// any closure. After closing it takes no new guests, and anyone already in line still rides.

// buildHours parses node hours into offsets from opening (validated by park.node_hours).
func (p *Park) buildHours() error {
	p.hours = make([][2]engine.Time, len(p.g.Nodes))
	openS := float64(p.cfg.Park.OpenHour * 3600)
	for _, n := range p.g.Nodes {
		p.hours[n.Idx] = [2]engine.Time{0, engine.Time(math.MaxInt64)} // no hours: the park's own day decides
		if n.Hours == "" {
			continue
		}
		from, to, err := config.ParseClockWindow(n.Hours)
		if err != nil {
			return err
		}
		p.hours[n.Idx] = [2]engine.Time{engine.Time(engine.Seconds(from - openS)), engine.Time(engine.Seconds(to - openS))}
	}
	return nil
}

// inHours reports whether node i takes guests at t.
func (p *Park) inHours(i int, t engine.Time) bool {
	h := p.hours[i]
	return t >= h[0] && t < h[1]
}

type hoursStopper interface {
	Stop(cause string)
	Resume(cause string)
}

// startHours closes the activities that open after the park, until they open.
func (p *Park) startHours() {
	for i, a := range p.acts {
		from := p.hours[i][0]
		if a == nil || from <= 0 {
			continue
		}
		switch act := a.(type) {
		case *rideAct:
			s, ok := act.r.(hoursStopper)
			if !ok {
				continue
			}
			s.Stop(config.CauseAreaHours)
			p.sim.At(from, func() { s.Resume(config.CauseAreaHours) })
		case *simpleRide, *meet:
			sim := p.scopes[i]
			sim.Emit(engine.Event{Type: engine.EvRideStop, Info: config.CauseAreaHours})
			sim.At(from, func() { sim.Emit(engine.Event{Type: engine.EvRideResume, Info: config.CauseAreaHours}) })
		}
	}
}

// turnAwayOutOfHours sends away a party that reached node c outside its hours; false when the node is open.
func (p *Park) turnAwayOutOfHours(c int, party *guest.Party) bool {
	if p.inHours(c, p.sim.Now()) {
		return false
	}
	p.scopes[c].Emit(engine.Event{Type: engine.EvBalk, Guest: party.ID, N: party.Size, Info: "closed"})
	party.Exited(guest.Balked)
	return true
}
