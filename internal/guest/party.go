// Package guest holds parties and arrival generation: a piecewise-constant Poisson arrival curve, party
// sizes, single-rider choice and patience. The park-level crowd model (M7) builds on this.
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
	Lane     Lane
	JoinedAt engine.Time
	Patience engine.Duration // longest wait tolerated; 0 = unlimited (no balk or renege)

	// Committed parties are past the point of reneging (inside a preshow room or the load area).
	Committed bool
	// Gone parties reneged; queues skip them (lazy deletion).
	Gone bool
}
