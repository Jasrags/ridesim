// Package live plays a park day interactively (M8): advance in chunks, apply levers between chunks, jump
// back in time, and export what was played as a park scenario that reruns byte-identically. The sim stays
// single-threaded; a Session is not safe for concurrent use (the server serializes access).
package live

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/park"
	"github.com/jasrags/ridesim/internal/scenario"
)

// Session is one played park day.
type Session struct {
	base *config.Scenario
	run  scenario.ParkRun

	sim  *engine.Sim
	park *park.Park
	ops  []played // every op in time order: the scenario's own and live ones
	next int      // first op not yet applied
	done bool     // run_end logged

	feed *feed
}

type played struct {
	scenario.ParkOp
	live bool
}

// New opens a session at park opening for a validated park scenario.
func New(ds *config.Dataset, f *config.ScenarioFile) (*Session, error) {
	r, err := scenario.Load(ds, f)
	if err != nil {
		return nil, err
	}
	run, ok := r.(scenario.ParkRun)
	if !ok {
		return nil, fmt.Errorf("scenario %s is not a park scenario", f.Scenario.ID)
	}
	s := &Session{base: &f.Scenario, run: run}
	var ops []played
	for _, o := range run.Ops {
		ops = append(ops, played{ParkOp: o})
	}
	return s, s.rebuild(ops, 0)
}

// rebuild starts the day again from opening with ops, and runs to t.
func (s *Session) rebuild(ops []played, t engine.Time) error {
	s.sim = engine.New(s.run.Seed)
	s.run.EmitStart(s.sim)
	p, err := s.run.Start(s.sim)
	if err != nil {
		return err
	}
	s.park, s.ops, s.next, s.done, s.feed = p, ops, 0, false, newFeed()
	return s.runTo(t)
}

// runTo advances to t (clamped to the end), applying due ops exactly as ParkRun.Run does.
func (s *Session) runTo(t engine.Time) error {
	end := s.End()
	if t > end {
		t = end
	}
	for s.next < len(s.ops) && engine.Time(s.ops[s.next].At) <= t {
		o := s.ops[s.next]
		s.sim.RunBefore(engine.Time(o.At))
		if err := s.park.Apply(o.Op); err != nil {
			return fmt.Errorf("ops at %v: %w", o.At, err)
		}
		s.next++
	}
	s.sim.RunUntil(t)
	if t == end && !s.done {
		s.sim.Emit(engine.Event{Type: engine.EvRunEnd, N: s.park.InPark()})
		s.done = true
	}
	s.feed.scan(s.sim.Log.Events())
	return nil
}

// Now is the current sim time (offset from opening); End is the end of the day.
func (s *Session) Now() engine.Time { return s.sim.Now() }
func (s *Session) End() engine.Time { return engine.Time(s.run.Length) }

// Done reports whether the day has ended.
func (s *Session) Done() bool { return s.done }

// Advance runs d more of the day.
func (s *Session) Advance(d engine.Duration) error {
	if s.done || d <= 0 {
		return nil
	}
	return s.runTo(s.sim.Now().Add(d))
}

// Apply runs a lever at the next whole second (ahead of every event then, as in a rerun) and records it.
// It returns the time the lever took effect.
func (s *Session) Apply(op config.OpsEvent) (engine.Time, error) {
	if s.done {
		return 0, fmt.Errorf("the day has ended")
	}
	at := (s.sim.Now()/engine.Time(engine.Second) + 1) * engine.Time(engine.Second)
	if at >= s.End() {
		return 0, fmt.Errorf("too late in the day for %s", op.Action)
	}
	if err := s.runTo(at - 1); err != nil { // scripted ops due before the lever
		return 0, err
	}
	s.sim.RunBefore(at)
	for s.next < len(s.ops) && engine.Time(s.ops[s.next].At) == at { // scripted ops in the same second go first
		if err := s.park.Apply(s.ops[s.next].Op); err != nil {
			return 0, fmt.Errorf("ops at %v: %w", s.ops[s.next].At, err)
		}
		s.next++
	}
	if err := s.park.Apply(op); err != nil {
		return 0, err
	}
	// Insert after every op at or before at (scripted ops at the same second ran first in a rerun too).
	i := sort.Search(len(s.ops), func(i int) bool { return s.ops[i].At > engine.Duration(at) })
	s.ops = append(s.ops[:i], append([]played{{ParkOp: scenario.ParkOp{At: engine.Duration(at), Op: op}, live: true}}, s.ops[i:]...)...)
	s.next = i + 1
	s.feed.scan(s.sim.Log.Events())
	return at, nil
}

// JumpTo replays the day from opening to t with the scenario's ops and the live ops before t; live ops at
// or after t are discarded.
func (s *Session) JumpTo(t engine.Time) error {
	if t < 0 {
		t = 0
	}
	if t > s.sim.Now() {
		return s.runTo(t)
	}
	var keep []played
	for _, o := range s.ops {
		if !o.live || engine.Time(o.At) < t {
			o := o
			keep = append(keep, o)
		}
	}
	return s.rebuild(keep, t)
}

// Ops is every lever in time order (scripted and live).
func (s *Session) Ops() []scenario.ParkOp {
	out := make([]scenario.ParkOp, len(s.ops))
	for i, o := range s.ops {
		out[i] = o.ParkOp
	}
	return out
}

// Log is the event log so far.
func (s *Session) Log() *engine.Log { return s.sim.Log }

// Export writes the played session as a park scenario: the base scenario with every lever as a timed op.
func (s *Session) Export(id string) []byte {
	b := s.base
	var w bytes.Buffer
	fmt.Fprintf(&w, "# Played in ridesim serve from scenario %s; reruns byte-identically with ridesim run.\n", b.ID)
	w.WriteString("schema_version: \"1.0\"\nscenario:\n")
	fmt.Fprintf(&w, "  id: %s\n", id)
	fmt.Fprintf(&w, "  description: %q\n", "Played session based on "+b.ID)
	fmt.Fprintf(&w, "  park_id: %s\n  seed: %d\n  duration_h: %g\n  warmup_min: %g\n", b.ParkID, s.run.Seed, b.DurationH, b.WarmupMin)
	if b.Attendance > 0 {
		fmt.Fprintf(&w, "  attendance: %d\n", b.Attendance)
	} else {
		fmt.Fprintf(&w, "  crowd_level: %s\n", b.CrowdLevel)
	}
	if b.WalkingMode != "" {
		fmt.Fprintf(&w, "  walking_mode: %s\n", b.WalkingMode)
	}
	if b.NoLightningLane {
		w.WriteString("  no_lightning_lane: true\n")
	}
	if b.FixedTimes {
		w.WriteString("  fixed_times: true\n")
	}
	if len(b.Failures) > 0 {
		fmt.Fprintf(&w, "  failures: [%s]\n", joinComma(b.Failures))
	}
	if len(s.ops) > 0 {
		w.WriteString("  ops:\n")
	}
	for _, o := range s.ops {
		fmt.Fprintf(&w, "    - { at: %q, action: %s", clock(o.At), o.Op.Action)
		if o.Op.Node != "" {
			fmt.Fprintf(&w, ", node: %s", o.Op.Node)
		}
		if o.Op.Minutes != 0 {
			fmt.Fprintf(&w, ", minutes: %g", o.Op.Minutes)
		}
		if o.Op.Value != nil {
			fmt.Fprintf(&w, ", value: %g", *o.Op.Value)
		}
		w.WriteString(" }\n")
	}
	return w.Bytes()
}

func clock(d engine.Duration) string {
	sec := int64(d / engine.Second)
	return fmt.Sprintf("%02d:%02d:%02d", sec/3600, sec/60%60, sec%60)
}

func joinComma(xs []string) string {
	var b bytes.Buffer
	for i, x := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(x)
	}
	return b.String()
}
