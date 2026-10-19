package park

import (
	"fmt"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
)

// EvParkOps records an operations lever applied to the park (scripted or live). Block = node,
// Info = "action=... minutes=... value=...".
const EvParkOps = "ops"

type fleet interface {
	AddTrain()
	RemoveTrain()
}

type singleRiderLine interface {
	SetSingleRiderOpen(open bool)
}

type merger interface {
	SetMergeRatio(llPerStandby int) bool
}

type stopper interface {
	Stop(cause string)
	Resume(cause string)
}

// Apply runs an operations lever now: add/remove a train, open/close a single-rider line, close a ride for
// some minutes, or set the crowd scale. The same actions run from scenario ops and from live play, so a
// played session exported as ops reruns identically.
func (p *Park) Apply(op config.OpsEvent) error {
	if bad := opShapeErr(op); bad != nil {
		return bad
	}
	if op.Action == config.OpLLInventory || op.Action == config.OpLLMerge {
		return p.applyLL(op)
	}
	var ra *rideAct
	if op.Action != config.OpCrowdScale {
		n := p.g.Node(op.Node)
		if n == nil {
			return fmt.Errorf("ops %s: no node %q", op.Action, op.Node)
		}
		ra, _ = p.acts[n.Idx].(*rideAct)
		if ra == nil {
			return fmt.Errorf("ops %s: %s is not a modeled ride", op.Action, op.Node)
		}
	}
	switch op.Action {
	case config.OpAddTrain, config.OpRemoveTrain:
		f, ok := ra.r.(fleet)
		if !ok {
			return fmt.Errorf("ops %s: %s has no trains to add or remove", op.Action, op.Node)
		}
		p.logOp(op)
		if op.Action == config.OpAddTrain {
			f.AddTrain()
		} else {
			f.RemoveTrain()
		}
	case config.OpSingleRiderOpen, config.OpSingleRiderClose:
		l, ok := ra.r.(singleRiderLine)
		if !ok || !ra.single {
			return fmt.Errorf("ops %s: %s has no single-rider line", op.Action, op.Node)
		}
		p.logOp(op)
		l.SetSingleRiderOpen(op.Action == config.OpSingleRiderOpen)
	case config.OpCloseRide:
		s, ok := ra.r.(stopper)
		if !ok {
			return fmt.Errorf("ops close_ride: %s cannot be stopped", op.Node)
		}
		p.logOp(op)
		s.Stop(config.CauseOpsClosure)
		p.sim.After(engine.Seconds(op.Minutes*60), func() { s.Resume(config.CauseOpsClosure) })
	case config.OpCrowdScale:
		p.logOp(op)
		p.crowd = *op.Value
	}
	return nil
}

// applyLL runs the Lightning Lane levers on a node that sells LL in this run.
func (p *Park) applyLL(op config.OpsEvent) error {
	n := p.g.Node(op.Node)
	if n == nil {
		return fmt.Errorf("ops %s: no node %q", op.Action, op.Node)
	}
	var r *llRide
	if p.ll != nil {
		r = p.ll.ride(n.Idx)
	}
	if r == nil {
		return fmt.Errorf("ops %s: %s sells no Lightning Lane", op.Action, op.Node)
	}
	if op.Action == config.OpLLInventory {
		if r.tier == config.LLSinglePass {
			return fmt.Errorf("ops ll_inventory: %s is Single Pass, sold only before the day", op.Node)
		}
		p.logOp(op)
		p.ll.setShare(r, *op.Value, p.sim.Now())
		return nil
	}
	v := int(*op.Value)
	switch act := p.acts[n.Idx].(type) {
	case *simpleRide:
		p.logOp(op)
		act.setMergeRatio(v)
	case *rideAct:
		m, ok := act.r.(merger)
		if !ok {
			return fmt.Errorf("ops ll_merge: %s has no merge to set", op.Node)
		}
		p.logOp(op)
		if !m.SetMergeRatio(v) {
			return fmt.Errorf("ops ll_merge: %s has no Lightning Lane line", op.Node)
		}
	default:
		return fmt.Errorf("ops ll_merge: %s has no merge to set", op.Node)
	}
	return nil
}

func opShapeErr(op config.OpsEvent) error {
	f := &config.ScenarioFile{Scenario: config.Scenario{ParkID: "x", Ops: []config.OpsEvent{op}}}
	if bad := config.OpShape(op, f.Scenario.IsPark()); bad != "" {
		return fmt.Errorf("ops: %s", bad)
	}
	return nil
}

func (p *Park) logOp(op config.OpsEvent) {
	info := "action=" + op.Action
	if op.Minutes != 0 {
		info += fmt.Sprintf(" minutes=%g", op.Minutes)
	}
	if op.Value != nil {
		info += fmt.Sprintf(" value=%g", *op.Value)
	}
	p.sim.Emit(engine.Event{Type: EvParkOps, Block: op.Node, Info: info})
}

// CrowdScale is the current crowd scale (1 = the scenario's attendance).
func (p *Park) CrowdScale() float64 { return p.crowd }
