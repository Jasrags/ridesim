package coaster

import (
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride/lines"
)

// Guest flow up to the load line lives in package lines. This file seats parties on trains.
// Seats are rows of trains.seats_per_row: a party takes whole rows and is never split. The spare seats
// parties leave (gaps) go to a solo party at the head of the line (the grouper pairs odd parties; no
// skipping) or to single riders; from load_done on, single riders may also take whole empty rows
// (decision 2026-10-17).

func (c *Coaster) initQueues() {
	c.lines = lines.New(c.sim, c, &c.cfg.Queues, c.cfg.Preshow)
}

func (c *Coaster) newTrain() *train {
	c.nextTrainID++
	spr := max(1, c.cfg.Trains.SeatsPerRow)
	return &train{id: c.nextTrainID, seats: c.cfg.Trains.SeatsPerTrain, rows: c.cfg.Trains.SeatsPerTrain / spr}
}

func (t *train) full() bool { return t.rowsFree == 0 && t.gaps == 0 }

func (c *Coaster) rowsFor(size int) int {
	spr := max(1, c.cfg.Trains.SeatsPerRow)
	return (size + spr - 1) / spr
}

// Arrive implements ride.Ride.
func (c *Coaster) Arrive(p *guest.Party, lane guest.Lane) { c.lines.Arrive(p, lane) }

// SingleRiderOpen reports whether the single-rider line takes new guests.
func (c *Coaster) SingleRiderOpen() bool { return c.lines.SingleRiderOpen() }

// SetSingleRiderOpen opens or closes the single-rider line (an operations lever).
func (c *Coaster) SetSingleRiderOpen(open bool) { c.lines.SetSingleRiderOpen(open) }

// Sample implements lines.Ride (preshow timings use the station stream).
func (c *Coaster) Sample(d config.Dist) engine.Duration {
	return c.sample(d, engine.StreamStationTimes)
}

// Board implements lines.Ride: seat whoever can be seated on the train in the load station.
func (c *Coaster) Board() {
	t := c.load.occupant
	if t == nil || c.Stopped() || (t.phase != phaseBoarding && t.phase != phaseFillWait) {
		return
	}
	c.board(t, t.phase == phaseFillWait)
	if t.phase == phaseFillWait {
		c.checkFillWait(t)
	}
}

func (c *Coaster) flow() { c.lines.Flow() }

// board seats line parties in whole rows (stopping at the first that does not fit), then single riders
// in the gaps, and in whole empty rows when fullRows is set (after load_done).
func (c *Coaster) board(t *train, fullRows bool) {
	spr := max(1, c.cfg.Trains.SeatsPerRow)
	boarded := false
	for {
		p, line := c.lines.Head()
		if p == nil {
			break
		}
		if p.Size == 1 && t.gaps > 0 {
			c.lines.Pop(line)
			t.gaps--
			c.seat(t, p)
			boarded = true
			continue
		}
		need := c.rowsFor(p.Size)
		if need > t.rowsFree {
			break
		}
		c.lines.Pop(line)
		t.rowsFree -= need
		t.gaps += need*spr - p.Size
		c.seat(t, p)
		boarded = true
	}
	for t.gaps > 0 || (fullRows && t.rowsFree > 0) {
		p := c.lines.PopSingle()
		if p == nil {
			break
		}
		if t.gaps > 0 {
			t.gaps--
		} else {
			t.rowsFree--
			t.gaps += spr - 1
		}
		c.seat(t, p)
		boarded = true
	}
	if boarded {
		c.lines.Boarded()
	}
}

func (c *Coaster) seat(t *train, p *guest.Party) {
	c.lines.Seated(p)
	c.recordWait(c.sim.Now().Sub(p.JoinedAt))
	t.riders += p.Size
	t.parties = append(t.parties, p)
	c.sim.Emit(engine.Event{Type: engine.EvBoard, Train: t.id, Block: t.at.cfg.ID, Guest: p.ID, N: p.Size, Info: p.Lane.String()})
}

// checkFillWait ends the fill wait once the train is full or once nobody in line can fill a free seat:
// the next party needs more rows than remain and no single rider is waiting. Both hold rules stop
// waiting then (decision 2026-10-17: cast do not hold a train for guests who cannot board it). An empty
// line keeps waiting: a newcomer could still fill the train.
func (c *Coaster) checkFillWait(t *train) {
	if t.phase != phaseFillWait {
		return
	}
	head, _ := c.lines.Head()
	fits := head != nil && (c.rowsFor(head.Size) <= t.rowsFree || (head.Size == 1 && t.gaps > 0))
	stuck := head != nil && !fits && c.lines.PeekSingle() == nil
	if t.full() || stuck {
		c.cancelWork(t)
		c.startCheck(t)
	}
}

// WaitingGuests is every guest who has joined and not yet boarded or reneged.
func (c *Coaster) WaitingGuests() int { return c.lines.Waiting() }

// QueueGuests is WaitingGuests (kept for callers of the M1 API).
func (c *Coaster) QueueGuests() int { return c.WaitingGuests() }
