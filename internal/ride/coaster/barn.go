package coaster

import (
	"fmt"

	"github.com/jasrags/ridesim/internal/engine"
)

// Barn swaps are scripted ops events (decision 2026-10-17). A swap stops the ride for barn.swap_time_s:
// removal takes the next train that finishes unloading; insertion places a train in the unload station
// once it is empty. Trains-in-service changes immediately after the swap, which also switches the
// shadow-room hold_until_clear on or off around 5 trains.

// RemoveTrain requests that the next empty train at unload go to the barn.
func (c *Coaster) RemoveTrain() {
	if err := c.checkFleet(len(c.trains) - c.pendingRemove + c.pendingAdd - 1); err != nil {
		c.sim.Emit(engine.Event{Type: engine.EvOpsRejected, Info: "remove_train: " + err.Error()})
		return
	}
	c.pendingRemove++
}

// AddTrain requests a train from the barn.
func (c *Coaster) AddTrain() {
	if err := c.checkFleet(len(c.trains) - c.pendingRemove + c.pendingAdd + 1); err != nil {
		c.sim.Emit(engine.Event{Type: engine.EvOpsRejected, Info: "add_train: " + err.Error()})
		return
	}
	c.pendingAdd++
	c.tryAdd()
}

// InService is the number of trains currently on the track.
func (c *Coaster) InService() int { return len(c.trains) }

func (c *Coaster) checkFleet(n int) error {
	t := c.cfg.Trains
	switch {
	case c.cfg.Barn == nil:
		return fmt.Errorf("no barn")
	case n < 1 || n > t.MaxInService || n > len(c.blocks)-1:
		return fmt.Errorf("%d trains outside 1..%d", n, t.MaxInService)
	case n > t.FleetSize:
		return fmt.Errorf("%d trains exceed fleet %d", n, t.FleetSize)
	case c.cfg.Barn.StorageBays != nil && t.FleetSize-n > *c.cfg.Barn.StorageBays:
		return fmt.Errorf("%d trains in barn exceed %d bays", t.FleetSize-n, *c.cfg.Barn.StorageBays)
	}
	return nil
}

func (c *Coaster) unloadBlock() *block {
	for _, b := range c.blocks {
		if b.role == roleUnload || b.role == roleBoth {
			return b
		}
	}
	return nil
}

// unloadDone is called when t finishes unloading; it diverts t to the barn if a removal is pending.
func (c *Coaster) unloadDone(t *train, next func()) {
	if c.pendingRemove == 0 || c.swapping {
		next()
		return
	}
	c.pendingRemove--
	c.swap(t, "remove_train", func() {
		t.at.occupant = nil
		kept := make([]*train, 0, len(c.trains)-1)
		for _, u := range c.trains {
			if u != t {
				kept = append(kept, u)
			}
		}
		c.trains = kept
		c.sim.Emit(engine.Event{Type: engine.EvTrainToBarn, Train: t.id, Block: t.at.cfg.ID, N: len(c.trains)})
	})
}

// tryAdd starts a pending insertion if the unload station is empty and no swap is running.
func (c *Coaster) tryAdd() {
	ub := c.unloadBlock()
	if c.pendingAdd == 0 || c.swapping || ub == nil || ub.occupant != nil {
		return
	}
	c.pendingAdd--
	t := c.newTrain()
	ub.occupant = t // reserve the station so nothing else enters during the swap
	t.at = ub
	c.swap(t, "add_train", func() {
		c.trains = append(append([]*train(nil), c.trains...), t)
		c.sim.Emit(engine.Event{Type: engine.EvTrainFromBarn, Train: t.id, Block: ub.cfg.ID, N: len(c.trains)})
		c.enter(t, ub)
	})
}

// swap stops the ride for swap_time_s, applies done, then resumes.
func (c *Coaster) swap(t *train, action string, done func()) {
	c.swapping = true
	c.Stop(CauseBarnSwap)
	d := c.sample(c.cfg.Barn.SwapTimeS, engine.StreamOps)
	c.sim.Emit(engine.Event{Type: engine.EvBarnSwap, Train: t.id, Info: action, N: int(d)})
	c.sim.After(d, func() {
		done()
		c.swapping = false
		c.Resume(CauseBarnSwap)
		c.tryAdd()
	})
}
