package study

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"gopkg.in/yaml.v3"
)

func TestMeanCI(t *testing.T) {
	mean, half := meanCI([]float64{1, 2, 3, 4, 5})
	// sd = 1.5811, t(4) = 2.776, half = 2.776 x 1.5811 / sqrt(5) = 1.963
	if mean != 3 || math.Abs(half-1.963) > 0.001 {
		t.Fatalf("mean %v half %v, want 3 and 1.963", mean, half)
	}
	if _, h := meanCI([]float64{7}); h != 0 {
		t.Fatal("one sample must have zero half-width")
	}
}

func TestPairedRemovesSharedNoise(t *testing.T) {
	base := []float64{100, 140, 90, 160, 110}
	variant := []float64{102, 142, 92, 162, 112} // +2 on every seed despite large seed-to-seed spread
	d := Paired(base, variant)
	if d.Mean != 2 || d.Low != 2 || d.High != 2 || !d.Significant {
		t.Fatalf("paired diff %+v, want exactly +2 and significant", d)
	}
	same := Paired(base, base)
	if same.Mean != 0 || same.Significant {
		t.Fatalf("identical samples gave %+v", same)
	}
}

func loadData(t *testing.T) *config.Dataset {
	t.Helper()
	ds, err := config.LoadDir("../../data")
	if err != nil {
		t.Fatal(err)
	}
	if config.Validate(ds).HasErrors() {
		t.Fatal("data invalid")
	}
	return ds
}

func node(t *testing.T, src string) yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	return *doc.Content[0]
}

// A variant identical to the base runs on the same seeds, so every paired difference is exactly zero;
// removing a train is a clear, significant loss of throughput.
func TestStudyCommonRandomNumbers(t *testing.T) {
	ds := loadData(t)
	st := &config.Study{ID: "t", Base: "everest_baseline_day", Variants: []config.Variant{
		{ID: "same", Set: node(t, "{}")},
		{ID: "four_trains", Set: node(t, "{ trains: 4 }")},
	}}
	res, err := Run(ds, st, 4, 0)
	if err != nil {
		t.Fatal(err)
	}
	j := res.JSON()
	for _, m := range j.Variants[1].Metrics {
		if m.Diff.Mean != 0 || m.Diff.Significant {
			t.Errorf("identical variant: %s differs by %v", m.Name, m.Diff.Mean)
		}
	}
	riders := j.Variants[2].Metrics[0]
	if riders.Name != "riders_per_hr" || riders.Diff.Mean >= 0 || !riders.Diff.Significant {
		t.Errorf("four trains: riders diff %+v, want a significant loss", riders.Diff)
	}
	var buf bytes.Buffer
	if err := res.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var back ResultJSON
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil || len(back.Variants) != 3 || len(back.Variants[0].Metrics[0].Replication) != 4 {
		t.Fatalf("JSON round trip: %v, %+v", err, back)
	}
}

// Parallel and sequential replication give identical reports (runs are independent and deterministic).
func TestReplicationIndependentOfParallelism(t *testing.T) {
	ds := loadData(t)
	st := &config.Study{ID: "t", Base: "rnrc_crowd_low"}
	a, err := Run(ds, st, 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run(ds, st, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	var ja, jb bytes.Buffer
	_ = a.WriteJSON(&ja)
	_ = b.WriteJSON(&jb)
	if !bytes.Equal(ja.Bytes(), jb.Bytes()) {
		t.Fatal("results depend on parallelism")
	}
}

// Broken variants fail resolution with the failing check named, so validate catches them.
func TestStudyRejectsBadVariants(t *testing.T) {
	ds := loadData(t)
	cases := map[string]config.Variant{
		"too many trains": {ID: "v", Set: node(t, "{ trains: 9 }")},
		"unknown field":   {ID: "v", Set: node(t, "{ trainz: 4 }")},
		"bad ride value":  {ID: "v", RideSet: node(t, "{ station: { min_dispatch_interval_s: 99 } }")},
	}
	for name, v := range cases {
		st := &config.Study{ID: "t", Base: "everest_baseline_day", Variants: []config.Variant{v}}
		if _, err := st.Resolve(ds); err == nil {
			t.Errorf("%s: resolved without error", name)
		}
	}
	if _, err := (&config.Study{ID: "t", Base: "nope"}).Resolve(ds); err == nil {
		t.Error("unknown base resolved")
	}
}

// A park study compares one node across variants; the identical variant shows no difference.
func TestParkStudyReportsTheNamedNode(t *testing.T) {
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	st := &config.Study{ID: "t", Base: "fx_park_day", ReportNode: "coaster", Variants: []config.Variant{
		{ID: "same", Set: node(t, "{}")},
		{ID: "congestion", Set: node(t, "{ walking_mode: congestion }")},
	}}
	res, err := Run(ds, st, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	j := res.JSON()
	if len(j.Variants) != 3 || j.Variants[0].Metrics[0].Mean == 0 {
		t.Fatalf("variants %d, base riders %+v", len(j.Variants), j.Variants[0].Metrics[0])
	}
	for _, m := range j.Variants[1].Metrics {
		if m.Diff.Mean != 0 {
			t.Errorf("identical variant: %s differs by %v", m.Name, m.Diff.Mean)
		}
	}
	for _, bad := range []*config.Study{
		{ID: "t", Base: "fx_park_day"},
		{ID: "t", Base: "fx_park_day", ReportNode: "hub"},
		{ID: "t", Base: "fx_park_day", ReportNode: "coaster", Variants: []config.Variant{{ID: "r", RideSet: node(t, "{ ride: { listed_capacity_per_hr: 1 } }")}}},
	} {
		if _, err := bad.Resolve(ds); err == nil {
			t.Errorf("study %+v resolved, want an error", bad)
		}
	}
}
