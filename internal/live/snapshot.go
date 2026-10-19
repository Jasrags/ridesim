package live

import (
	"github.com/jasrags/ridesim/internal/engine"
)

// Snapshot is what the front end draws. JSON field names are the page's contract.
type Snapshot struct {
	T      engine.Time    `json:"t"`
	End    engine.Time    `json:"end"`
	Open   int            `json:"open_hour"`
	Done   bool           `json:"done"`
	InPark int            `json:"in_park"`
	Crowd  float64        `json:"crowd"`
	States map[string]int `json:"states"`
	Nodes  []NodeJSON     `json:"nodes"`
	Rides  []RideJSON     `json:"rides"`
	Edges  []EdgeJSON     `json:"edges"`
	Feed   []FeedItem     `json:"feed"`
	Ops    []OpJSON       `json:"ops"`
}

type NodeJSON struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

type RideJSON struct {
	Node        string   `json:"node"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	PostedMin   float64  `json:"posted_min"`
	Waiting     int      `json:"waiting"`
	Trains      int      `json:"trains,omitempty"`
	SingleRider *bool    `json:"single_rider,omitempty"`
	LL          bool     `json:"lightning_lane"`
	LLLevers    bool     `json:"ll_levers,omitempty"`
	LLShare     *float64 `json:"ll_share,omitempty"` // inventory share for windows not yet started (M15)
	LLMerge     *int     `json:"ll_merge,omitempty"` // LL guests per standby guest set by ll_merge; absent = authored merge
	History     []Point  `json:"history"`
}

type EdgeJSON struct {
	ID          string `json:"id"`
	From        string `json:"from"`
	To          string `json:"to"`
	Guests      int    `json:"guests"`
	Comfortable int    `json:"comfortable"`
	Spill       int    `json:"spill"`
}

type OpJSON struct {
	At     engine.Time `json:"at"`
	Action string      `json:"action"`
	Node   string      `json:"node,omitempty"`
	Live   bool        `json:"live"`
}

// Snapshot pictures the session now. It reads state only.
func (s *Session) Snapshot() Snapshot {
	v := s.park.View()
	out := Snapshot{T: v.Now, End: s.End(), Open: s.run.Park.Park.OpenHour, Done: s.done, InPark: v.InPark,
		Crowd: v.Crowd, States: v.States, Feed: append([]FeedItem{}, s.feed.items...), Ops: []OpJSON{}}
	for _, n := range v.Nodes {
		out.Nodes = append(out.Nodes, NodeJSON{n.ID, n.Kind, n.Label, n.X, n.Y})
	}
	for _, r := range v.Rides {
		out.Rides = append(out.Rides, RideJSON{Node: r.Node, Label: r.Label, Kind: r.Kind, Status: r.Status,
			PostedMin: r.Posted.Seconds() / 60, Waiting: r.Waiting, Trains: r.Trains, SingleRider: r.SingleRider,
			LL: r.LightningLane, LLLevers: r.LLLevers, LLShare: r.LLShare, LLMerge: r.LLMerge, History: append([]Point{}, s.feed.posted[r.Node]...)})
	}
	for _, e := range v.Edges {
		out.Edges = append(out.Edges, EdgeJSON{e.ID, e.From, e.To, e.Guests, e.Comfortable, e.Spill})
	}
	for _, o := range s.ops {
		out.Ops = append(out.Ops, OpJSON{engine.Time(o.At), o.Op.Action, o.Op.Node, o.live})
	}
	return out
}
