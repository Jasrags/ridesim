package park

import (
	"github.com/jasrags/ridesim/internal/config"
)

// streamGate is the park-scope stream that picks each party's gate. It is drawn only in parks with several
// entrances, so single-gate parks keep their logs.
const streamGate = "gate"

// gate is an entrance and the exit its parties leave by (M17, EPCOT's International Gateway).
type gate struct {
	entrance, exit int
	share          float64
}

// buildGates pairs entrances with exits: a single-gate park pairs its one entrance and exit; otherwise each
// entrance names its exit and arrival share (validated by park.gates).
func (p *Park) buildGates() {
	var entrances []*Node
	exit := -1
	for _, n := range p.g.Nodes {
		switch n.Kind {
		case config.NodeEntrance:
			entrances = append(entrances, n)
		case config.NodeExit:
			exit = n.Idx
		}
	}
	if len(entrances) == 1 {
		p.gates = []gate{{entrance: entrances[0].Idx, exit: exit, share: 1}}
		return
	}
	for _, e := range entrances {
		p.gates = append(p.gates, gate{entrance: e.Idx, exit: p.g.Node(e.Exit).Idx, share: e.ArrivalShare})
	}
}

// pickGate draws a party's gate by the arrival shares (always gate 0 in a single-gate park).
func (p *Park) pickGate() int {
	if len(p.gates) == 1 {
		return 0
	}
	x := p.sim.Stream(streamGate).Float64()
	for i, g := range p.gates {
		if x -= g.share; x < 0 {
			return i
		}
	}
	return len(p.gates) - 1
}

// exitOf is the exit a party leaves by.
func (p *Park) exitOf(a *agent) int { return p.gates[a.gate].exit }

// entranceOf is the gate a party enters by.
func (p *Park) entranceOf(a *agent) int { return p.gates[a.gate].entrance }
