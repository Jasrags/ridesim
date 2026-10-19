package live

import (
	"bytes"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/scenario"
)

func fixture(t *testing.T) (*config.Dataset, *config.ScenarioFile) {
	t.Helper()
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	f := ds.Scenario("fx_park_day")
	if f == nil {
		t.Fatal("no fx_park_day")
	}
	return ds, f.Scenario
}

func logBytes(t *testing.T, l *engine.Log) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := l.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func v(x float64) *float64 { return &x }

// A played session (uneven chunks, live levers, a jump back) exports as a scenario whose rerun with the
// ordinary runner gives a byte-identical log.
func TestPlayedSessionExportRerunsIdentically(t *testing.T) {
	ds, sf := fixture(t)
	s, err := New(ds, sf)
	if err != nil {
		t.Fatal(err)
	}
	step := func(d engine.Duration) {
		if err := s.Advance(d); err != nil {
			t.Fatal(err)
		}
	}
	apply := func(op config.OpsEvent) {
		if _, err := s.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	step(37*engine.Minute + 123)
	apply(config.OpsEvent{Action: config.OpCrowdScale, Value: v(1.4)})
	step(7 * engine.Second)
	apply(config.OpsEvent{Action: config.OpCloseRide, Node: "coaster", Minutes: 12})
	step(2*engine.Hour + 999)
	apply(config.OpsEvent{Action: config.OpRemoveTrain, Node: "coaster"}) // discarded by the jump below
	step(20 * engine.Minute)
	if err := s.JumpTo(engine.Time(2 * engine.Hour)); err != nil {
		t.Fatal(err)
	}
	apply(config.OpsEvent{Action: config.OpCrowdScale, Value: v(0.7)})
	for !s.Done() {
		step(53 * engine.Minute)
	}
	live := 0
	for _, o := range s.ops {
		if o.live {
			live++
		}
	}
	if live != 3 {
		t.Fatalf("%d live ops kept, want 3 (one discarded by the jump)", live)
	}

	exported := s.Export("fx_played")
	f, err := config.LoadBytes("fx-played.yaml", exported)
	if err != nil {
		t.Fatalf("export does not load: %v\n%s", err, exported)
	}
	files := append([]*config.File{f}, ds.Files...)
	for _, r := range config.Validate(&config.Dataset{Files: files}).FileReport("fx-played.yaml").Results {
		if r.Status == config.StatusFail {
			t.Fatalf("export fails %s: %s\n%s", r.CheckID, r.Detail, exported)
		}
	}
	run, err := scenario.Load(ds, f.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	rerun, err := run.Run()
	if err != nil {
		t.Fatal(err)
	}
	// The logs match byte for byte after run_start, whose header names the scenario (played vs exported id).
	a, b := logBytes(t, s.Log()), logBytes(t, rerun)
	ha, ra, _ := bytes.Cut(a, []byte("\n"))
	hb, rb, _ := bytes.Cut(b, []byte("\n"))
	if !bytes.Equal(bytes.Replace(ha, []byte("scenario=fx_park_day"), []byte("scenario=fx_played"), 1), hb) {
		t.Fatalf("run_start differs beyond the scenario id:\n%s\n%s", ha, hb)
	}
	if !bytes.Equal(ra, rb) {
		la, lb := bytes.Split(ra, []byte("\n")), bytes.Split(rb, []byte("\n"))
		for i := 0; i < len(la) && i < len(lb); i++ {
			if !bytes.Equal(la[i], lb[i]) {
				t.Fatalf("rerun differs at event %d:\n played %s\n rerun  %s", i+1, la[i], lb[i])
			}
		}
		t.Fatalf("rerun differs in length: %d vs %d events", len(la), len(lb))
	}
}

// Jumping back to t and playing forward with no new levers reproduces the original timeline.
func TestJumpBackReplaysTheSameDay(t *testing.T) {
	ds, sf := fixture(t)
	s, _ := New(ds, sf)
	_ = s.Advance(3 * engine.Hour)
	before := logBytes(t, s.Log())
	_ = s.JumpTo(engine.Time(engine.Hour))
	if s.Now() != engine.Time(engine.Hour) {
		t.Fatalf("now %v after jump", s.Now())
	}
	_ = s.Advance(2 * engine.Hour)
	if !bytes.Equal(before, logBytes(t, s.Log())) {
		t.Fatal("replay after a jump differs")
	}
}

func TestSnapshotHasMapRidesAndFeed(t *testing.T) {
	ds, sf := fixture(t)
	s, _ := New(ds, sf)
	_ = s.Advance(2*engine.Hour + 30*engine.Minute) // past the scripted 02:00 closure
	snap := s.Snapshot()
	if len(snap.Nodes) == 0 || len(snap.Rides) == 0 || len(snap.Edges) == 0 || len(snap.Ops) != 6 {
		t.Fatalf("snapshot %d nodes %d rides %d edges %d ops", len(snap.Nodes), len(snap.Rides), len(snap.Edges), len(snap.Ops))
	}
	found := false
	for _, it := range snap.Feed {
		if it.Type == "ops" {
			found = true
		}
	}
	if !found {
		t.Fatalf("feed has no ops items: %+v", snap.Feed)
	}
}

func TestApplyRejectsBadLeverWithoutRecording(t *testing.T) {
	ds, sf := fixture(t)
	s, _ := New(ds, sf)
	_ = s.Advance(engine.Hour)
	n := len(s.ops)
	if _, err := s.Apply(config.OpsEvent{Action: config.OpCloseRide, Node: "nowhere", Minutes: 5}); err == nil {
		t.Fatal("bad lever accepted")
	}
	if len(s.ops) != n {
		t.Fatal("bad lever was recorded")
	}
}
