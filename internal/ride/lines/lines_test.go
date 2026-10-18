package lines

import (
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

type stubRide struct{}

func (stubRide) ID() string                           { return "test.fx.ride" }
func (stubRide) PostedWait() engine.Duration          { return 0 }
func (stubRide) Board()                               {}
func (stubRide) Sample(d config.Dist) engine.Duration { return engine.Seconds(d.Mean) }

// A single rider who skipped the preshow ([fan] Smugglers Run) waits in the load line ahead of parties
// released later. Head (the boarding order) returns them first; HeadParty, used for the grouper's party
// seats ([user]), returns the party. The bypassing single does not use load-area space the stage needs.
func TestBypassingSingleRiderAndHeadParty(t *testing.T) {
	sim := engine.New(1)
	q := &config.Queues{SingleRider: config.SingleRider{Enabled: true}}
	ps := &config.Preshow{Enabled: true, Rooms: 1, BatchSize: 10, DurationS: config.Dist{Mean: 60}, TurnoverS: config.Dist{Mean: 10},
		MaxFillWaitS: 30, LoadAreaCapacity: 4, BypassForSingleRider: true}
	l := New(sim, stubRide{}, q, ps, nil)
	single := &guest.Party{ID: 1, Size: 1}
	l.Arrive(single, guest.SingleRider)
	if l.loadSingle.Peek() != single || l.single.Peek() != nil {
		t.Fatal("the bypassing single rider is not in the load-area line")
	}
	if free := l.loadLineFree(ps); free != 4 {
		t.Fatalf("load area free %d with only a bypassing single waiting, want 4", free)
	}
	party := &guest.Party{ID: 2, Size: 3}
	l.releaseToLoad(party) // released by the preshow after the single arrived
	if p, _ := l.Head(); p != single {
		t.Fatalf("Head = party %d, want the earlier single rider", p.ID)
	}
	if p, line := l.HeadParty(); p != party || line != l.loadStandby {
		t.Fatalf("HeadParty = %v, want the released party", p)
	}
}
