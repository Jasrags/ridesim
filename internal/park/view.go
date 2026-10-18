package park

import (
	"math"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/ride"
)

// View is a read-only picture of the park now, for front ends. Building it changes nothing in the sim.
type View struct {
	Now    engine.Time
	InPark int
	Crowd  float64
	States map[string]int // guests by state (walking, ride, dining, ...)
	Nodes  []NodeView
	Rides  []RideView // activities with a line, in node order
	Edges  []EdgeView
}

type NodeView struct {
	ID, Kind, Label string
	X, Y            float64
	HasPos          bool
}

type RideView struct {
	Node, Label, Kind string
	Status            string // open | stopped | closed
	Posted            engine.Duration
	Waiting           int
	Trains            int   // coasters only
	SingleRider       *bool // nil = no single-rider line
	LightningLane     bool
}

type EdgeView struct {
	ID, From, To string
	Guests       int
	Comfortable  int // C, guests
	Spill        int // queue guests beyond the adjoining ride's physical capacity
}

type trainCounter interface{ InService() int }
type singleRiderState interface{ SingleRiderOpen() bool }

// View builds the current picture.
func (p *Park) View() View {
	v := View{Now: p.sim.Now(), InPark: p.inPark, Crowd: p.crowd, States: map[string]int{}}
	for _, s := range agentStates {
		v.States[s] = p.states[s]
	}
	for _, n := range p.g.Nodes {
		nv := NodeView{ID: n.ID, Kind: n.Kind, Label: label(n.ID)}
		if n.X != nil && n.Y != nil {
			nv.X, nv.Y, nv.HasPos = *n.X, *n.Y, true
		}
		if ra, ok := p.acts[n.Idx].(*rideAct); ok && ra.info.Name != "" {
			nv.Label = ra.info.Name
		}
		v.Nodes = append(v.Nodes, nv)
		a := p.acts[n.Idx]
		if a == nil || n.Kind == config.NodeElsewhere || n.Kind == config.NodeShop || n.Kind == config.NodeShow {
			continue
		}
		rv := RideView{Node: n.ID, Label: nv.Label, Kind: n.Kind, Status: "open", Posted: a.posted(), Waiting: a.queueGuests()}
		if n.Kind == config.NodeDining {
			rv.Waiting = a.(*dining).line.Guests()
		}
		if ra, ok := a.(*rideAct); ok {
			switch {
			case ra.closed():
				rv.Status = "closed"
			case ra.r.Status() == ride.Down:
				rv.Status = "stopped"
			}
			if tc, ok := ra.r.(trainCounter); ok {
				rv.Trains = tc.InService()
			}
			if s, ok := ra.r.(singleRiderState); ok && ra.single {
				open := s.SingleRiderOpen()
				rv.SingleRider = &open
			}
			rv.LightningLane = ra.ll
		}
		v.Rides = append(v.Rides, rv)
	}
	for _, e := range p.g.Edges {
		ev := EdgeView{ID: e.ID, From: p.g.Nodes[e.from].ID, To: p.g.Nodes[e.to].ID, Guests: e.guests,
			Comfortable: int(math.Round(e.LengthFt * e.WidthFt * p.cfg.Congestion.ComfortableDensityPerSqft))}
		if e.spill != nil {
			ev.Spill = e.spill()
		}
		v.Edges = append(v.Edges, ev)
	}
	return v
}

// label turns a node id into a display name ("boat_ride" -> "Boat ride").
func label(id string) string {
	s := strings.ReplaceAll(id, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
