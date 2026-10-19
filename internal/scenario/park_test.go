package scenario

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/metrics"
)

func fixtureParkRun(t *testing.T) ParkRun {
	t.Helper()
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	sf := ds.Scenario("fx_park_day")
	if sf == nil {
		t.Fatal("no fx_park_day scenario")
	}
	run, err := Load(ds, sf.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	pr, ok := run.(ParkRun)
	if !ok {
		t.Fatalf("Load returned %T, want ParkRun", run)
	}
	return pr
}

func TestParkScenarioRunsAndReports(t *testing.T) {
	run := fixtureParkRun(t)
	if run.Options.Attendance != 200 || run.Options.WalkingMode != "fixed_delay" {
		t.Fatalf("options %+v", run.Options)
	}
	log, err := run.Run()
	if err != nil {
		t.Fatal(err)
	}
	rep, err := metrics.ComputePark(log.Events(), run.WarmupDuration())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Arrived < 150 || rep.Left != rep.Arrived {
		t.Fatalf("arrived %d left %d", rep.Arrived, rep.Left)
	}
	for _, id := range []string{"coaster", "filler", "dining", "elsewhere"} {
		if n := rep.Node(id); n == nil || n.Report.Riders == 0 {
			t.Errorf("node %s missing or unused", id)
		}
	}
	if len(rep.Edges) != 7 || len(rep.Targets) != 2 {
		t.Fatalf("edges %d targets %d", len(rep.Edges), len(rep.Targets))
	}
	var out bytes.Buffer
	if err := rep.Write(&out); err != nil || !strings.Contains(out.String(), "calibration") {
		t.Fatalf("write: %v\n%s", err, out.String())
	}
}

func TestParkRunSameSeedSameLog(t *testing.T) {
	var logs [2]bytes.Buffer
	for i := range logs {
		log, err := WithSeed(fixtureParkRun(t), 9).Run()
		if err != nil {
			t.Fatal(err)
		}
		_ = log.Write(&logs[i])
	}
	if !bytes.Equal(logs[0].Bytes(), logs[1].Bytes()) {
		t.Fatal("park logs differ for the same seed")
	}
}

func TestParkScaleMultipliesAttendance(t *testing.T) {
	base, _ := fixtureParkRun(t).Run()
	big, _ := WithScale(fixtureParkRun(t), 2, 1).Run()
	a, _ := metrics.ComputePark(base.Events(), 0)
	b, _ := metrics.ComputePark(big.Events(), 0)
	if r := float64(b.Arrived) / float64(a.Arrived); r < 1.6 || r > 2.4 {
		t.Fatalf("scale 2 gave %d vs %d arrivals", b.Arrived, a.Arrived)
	}
}

// Scripted park ops run at their times and are logged.
func TestParkScenarioOpsRun(t *testing.T) {
	log, err := fixtureParkRun(t).Run()
	if err != nil {
		t.Fatal(err)
	}
	var at []string
	for _, e := range log.Events() {
		if e.Type == "ops" {
			at = append(at, e.T.String()+" "+e.Info)
		}
	}
	if len(at) != 6 || !strings.HasPrefix(at[2], "02:00:00.000 action=close_ride") || !strings.HasPrefix(at[5], "04:00:00.000 action=ll_merge value=3") {
		t.Fatalf("ops events %v", at)
	}
}
