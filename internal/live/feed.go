package live

import (
	"strconv"

	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/park"
)

// accessibilityHold is the continuous_load hold cause (kept out of the feed).
const accessibilityHold = "accessibility_hold"

// feedLen is how many recent notable events a snapshot carries.
const feedLen = 40

// FeedItem is one notable event for the front end's event feed.
type FeedItem struct {
	T    engine.Time `json:"t"`
	Src  string      `json:"src"`
	Type string      `json:"type"`
	Text string      `json:"text"`
}

// Point is one posted-wait sample (minutes) for a ride's sparkline.
type Point struct {
	T   engine.Time `json:"t"`
	Min float64     `json:"min"`
}

// feed reads the event log incrementally: recent notable events and posted-wait history per ride.
type feed struct {
	cursor  int
	items   []FeedItem
	posted  map[string][]Point // by node id; lookup only, snapshots iterate the park's ride order
	blocked map[string]int     // edge_blocked parties per edge since the last item for that edge
}

func newFeed() *feed { return &feed{posted: map[string][]Point{}, blocked: map[string]int{}} }

var notable = map[string]bool{
	engine.EvRideStop: true, engine.EvRideResume: true, engine.EvCascade: true, engine.EvBarnSwap: true,
	engine.EvOpsRejected: true, engine.EvSingleRider: true, park.EvShowEnd: true, park.EvParkOps: true,
}

func (f *feed) scan(evs []engine.Event) {
	for ; f.cursor < len(evs); f.cursor++ {
		e := evs[f.cursor]
		switch {
		case e.Type == engine.EvPostedWait:
			f.posted[e.Src] = append(f.posted[e.Src], Point{e.T, float64(e.N) / 60})
		case e.Type == park.EvEdgeBlocked:
			if f.blocked[e.Block]++; f.blocked[e.Block] == 1 {
				f.push(FeedItem{e.T, park.Scope, e.Type, "walkway " + e.Block + " at jam density"})
			}
		case (e.Type == engine.EvRideStop || e.Type == engine.EvRideResume) && e.Info == accessibilityHold:
			// several an hour on the Haunted Mansion; they would drown the feed
		case notable[e.Type]:
			f.push(FeedItem{e.T, e.Src, e.Type, describe(e)})
		}
	}
}

func (f *feed) push(it FeedItem) {
	f.items = append(f.items, it)
	if len(f.items) > feedLen {
		f.items = append([]FeedItem(nil), f.items[len(f.items)-feedLen:]...)
	}
}

func describe(e engine.Event) string {
	switch e.Type {
	case engine.EvRideStop:
		return "stopped: " + e.Info
	case engine.EvRideResume:
		return "resumed: " + e.Info
	case engine.EvCascade:
		return "cascade stop at " + e.Block
	case park.EvShowEnd:
		return "show let out " + strconv.Itoa(e.N) + " guests"
	case park.EvParkOps:
		if e.Block != "" {
			return e.Block + ": " + e.Info
		}
		return e.Info
	}
	if e.Info != "" {
		return e.Type + ": " + e.Info
	}
	return e.Type
}
