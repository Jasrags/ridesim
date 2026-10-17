// Package guest holds parties and arrival generation. M1 uses single-guest parties and a fixed
// piecewise-constant Poisson arrival curve; the park-level crowd model (M7) builds on this.
package guest

import "github.com/jasrags/ridesim/internal/engine"

// Lane is the queue a party joins at a ride.
type Lane int

const (
	Standby Lane = iota + 1
	SingleRider
	LightningLane
)

func (l Lane) String() string {
	switch l {
	case Standby:
		return "standby"
	case SingleRider:
		return "single_rider"
	case LightningLane:
		return "lightning_lane"
	}
	return "unknown"
}

// Party is a group that queues and rides together (never split, spec §8).
type Party struct {
	ID       int64
	Size     int
	JoinedAt engine.Time
}
