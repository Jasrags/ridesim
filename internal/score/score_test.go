package score

import (
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/park"
	"github.com/jasrags/ridesim/internal/scenario"
)

var weights = config.Scoring{ID: "w", Experience: config.ExperiencePoints{Ride: 10, Show: 6, Meet: 4}, WaitComfortMin: 20,
	WaitPenaltyPerMin: 0.5, BalkPenalty: 2, RenegePenalty: 5, WalkPenaltyPerMin: 0.1, JamPenaltyPerMin: 1, LLLapsePenalty: 3}

func mins(m int64) engine.Time { return engine.Time(m * int64(engine.Minute)) }

// One party of 2 and one of 1: the pair rides the coaster after 30 min (10 past comfort), sees a show,
// walks 10 min after 5 min at a jam, lets a pass lapse and balks at dining (free); the single balks at the meet and reneges at the
// simple ride after 25 min. Dining does not score.
func TestCardFromEvents(t *testing.T) {
	ps := park.Scope
	evs := []engine.Event{
		{Type: engine.EvRunStart, Src: "coaster", Info: "scenario=park node=coaster archetype=multi_train_coaster"},
		{Type: engine.EvRunStart, Src: "simple", Info: "archetype=ride_simple"},
		{Type: engine.EvRunStart, Src: "theater", Info: "archetype=show"},
		{Type: engine.EvRunStart, Src: "meet", Info: "archetype=meet"},
		{Type: engine.EvRunStart, Src: "diner", Info: "archetype=dining"},
		{Src: ps, Type: park.EvParkArrive, Guest: 1, N: 2},
		{Src: ps, Type: park.EvParkArrive, Guest: 2, N: 1},
		{T: mins(1), Src: ps, Type: park.EvEdgeBlocked, Guest: 1, N: 2},
		{T: mins(6), Src: ps, Type: park.EvWalk, Guest: 1, N: 2, D: 10 * engine.Minute},
		{T: mins(16), Src: "coaster", Type: engine.EvQueueJoin, Guest: 1<<8 | 1, N: 2},
		{T: mins(46), Src: "coaster", Type: engine.EvBoard, Guest: 1<<8 | 1, N: 2},
		{T: mins(50), Src: ps, Type: park.EvVisitEnd, Guest: 1, Block: "coaster", Info: "rode"},
		{T: mins(60), Src: "theater", Type: engine.EvQueueJoin, Guest: 1<<8 | 2, N: 2},
		{T: mins(90), Src: "theater", Type: engine.EvBoard, Guest: 1<<8 | 2, N: 2},
		{T: mins(100), Src: ps, Type: park.EvVisitEnd, Guest: 1, Block: "theater", Info: "rode"},
		{T: mins(110), Src: "diner", Type: engine.EvQueueJoin, Guest: 1<<8 | 3, N: 2},
		{T: mins(150), Src: "diner", Type: engine.EvBoard, Guest: 1<<8 | 3, N: 2},
		{T: mins(160), Src: ps, Type: park.EvVisitEnd, Guest: 1, Block: "diner", Info: "rode"},
		{T: mins(170), Src: ps, Type: park.EvLLLapse, Guest: 1, Block: "coaster"},
		{T: mins(171), Src: ps, Type: park.EvLLRedeem, Guest: 1, Block: "coaster"},
		{T: mins(175), Src: "diner", Type: engine.EvBalk, Guest: 1<<8 | 4, N: 2}, // dining balks cost nothing
		{T: mins(10), Src: "meet", Type: engine.EvBalk, Guest: 2<<8 | 1, N: 1},
		{T: mins(20), Src: "simple", Type: engine.EvQueueJoin, Guest: 2<<8 | 2, N: 1},
		{T: mins(45), Src: "simple", Type: engine.EvRenege, Guest: 2<<8 | 2, N: 1},
	}
	c := Compute(evs, weights)
	want := Card{Guests: 3, Experiences: 4, Balks: 1, Reneges: 1, LLLapsed: 1, LLRedeemed: 1,
		WaitMin: 2*30 + 25, WaitOverMin: 2*10 + 5, WalkMin: 20, JamMin: 10,
		Experience: 2*10 + 2*6, WaitPenalty: 0.5 * 25, BalkPenalty: 2, RenegePenalty: 5, WalkPenalty: 2, JamPenalty: 10, LapsePenalty: 6}
	if c != want {
		t.Fatalf("card\n got %+v\nwant %+v", c, want)
	}
	if got := c.Score(); math.Abs(got-(32-37.5)/3) > 1e-9 {
		t.Errorf("score %.4f", got)
	}
}

// A party split across two vehicles: each part's wait counts once.
func TestSplitPartyWaits(t *testing.T) {
	evs := []engine.Event{
		{Type: engine.EvRunStart, Src: "tot", Info: "archetype=elevator_drop"},
		{T: 0, Src: "tot", Type: engine.EvQueueJoin, Guest: 9, N: 4},
		{T: mins(10), Src: "tot", Type: engine.EvBoard, Guest: 9, N: 3},
		{T: mins(12), Src: "tot", Type: engine.EvBoard, Guest: 9, N: 1},
		{T: mins(30), Src: "tot", Type: engine.EvBoard, Guest: 9, N: 4}, // a later visit would have a new id
	}
	if got := Compute(evs, weights).WaitMin; got != 3*10+12 {
		t.Errorf("wait %.1f guest-min, want 42", got)
	}
}

// A real park day scores the same twice, earns points and charges each penalty.
func TestFixtureDayScore(t *testing.T) {
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	var cards [2]Card
	for i := range cards {
		run, err := scenario.Load(ds, ds.Scenario("fx_park_day").Scenario)
		if err != nil {
			t.Fatal(err)
		}
		log, err := run.Run()
		if err != nil {
			t.Fatal(err)
		}
		cards[i] = Compute(log.Events(), weights)
	}
	c := cards[0]
	if c != cards[1] {
		t.Fatal("two runs of one seed score differently")
	}
	if c.Guests == 0 || c.Experiences == 0 || c.WalkMin == 0 || c.WaitMin == 0 {
		t.Fatalf("empty card %+v", c)
	}
	t.Logf("\n%s", c)
}
