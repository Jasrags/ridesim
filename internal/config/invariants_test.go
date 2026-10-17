package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fxCoaster    = "testdata/test-fx-coaster.yaml"
	fxOmnimover  = "testdata/test-fx-omnimover.yaml"
	fxPark       = "testdata/test-fx-park.yaml"
	realDataRoot = "../../data"
)

// patched loads fixture after replacing old with new exactly once. name overrides the path used for
// the filename invariant ("" keeps the fixture path).
func patched(t *testing.T, fixture, name, old, new string) *File {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	if old != "" {
		if n := strings.Count(src, old); n != 1 {
			t.Fatalf("patch target %q occurs %d times in %s, want 1", old, n, fixture)
		}
		src = strings.Replace(src, old, new, 1)
	}
	if name == "" {
		name = fixture
	}
	f, err := LoadBytes(name, []byte(src))
	if err != nil {
		t.Fatalf("load patched %s: %v", fixture, err)
	}
	return f
}

func resultFor(t *testing.T, results []Result, id string) Result {
	t.Helper()
	for _, r := range results {
		if r.CheckID == id {
			return r
		}
	}
	t.Fatalf("check %q did not run; ran %v", id, checkIDs(results))
	return Result{}
}

func checkIDs(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.CheckID
	}
	return out
}

func TestValidFixturesPassEveryCheck(t *testing.T) {
	for _, fx := range []string{fxCoaster, fxOmnimover, fxPark} {
		t.Run(filepath.Base(fx), func(t *testing.T) {
			f := patched(t, fx, "", "", "")
			for _, r := range CheckFile(f) {
				if r.Status != StatusPass {
					t.Errorf("%s: %s: %s", r.CheckID, r.Status, r.Detail)
				}
			}
		})
	}
}

// Every error-level check has a broken fixture: the valid fixture with one patch applied.
func TestErrorChecksFailOnBrokenFixtures(t *testing.T) {
	cases := []struct {
		check, fixture, name, old, new string
	}{
		// shared ride checks
		{"ride.id_format", fxCoaster, "testdata/test-fx-coaster.yaml", "id: test.fx.coaster", "id: test_fx_coaster"},
		{"ride.filename", fxCoaster, "testdata/wrong-name.yaml", "", ""},
		{"queues.merge_phases", fxCoaster, "", "when_ll_queue_guests_below: null", "when_ll_queue_guests_below: 30"},
		{"preshow.throughput", fxCoaster, "", "batch_size: 100", "batch_size: 10"},
		{"preshow.throughput", fxCoaster, "", "batch_size: 100", "batch_size: 100\n  staging: lobby"},

		// multi_train_coaster
		{"coaster.duration_sum", fxCoaster, "", "transit_s: { mean: 40, sd: 1 }", "transit_s: { mean: 50, sd: 1 }"},
		{"coaster.block_count", fxCoaster, "", "fleet_size: 3\n  max_in_service: 3", "fleet_size: 5\n  max_in_service: 5"},
		{"coaster.fleet_order", fxCoaster, "", "default_in_service: 2", "default_in_service: 4"},
		{"coaster.brake_fields", fxCoaster, "", "stop_if_next_occupied, release_delay_s: { mean: 2, sd: 0.5 } }", "stop_if_next_occupied }"},
		{"coaster.refs", fxCoaster, "", "requires_empty: [LIFT]", "requires_empty: [NOPE]"},
		{"coaster.refs", fxCoaster, "", "switch: SW1", "switch: SW9"},
		{"coaster.block_kinds", fxCoaster, "", "kind: lift", "kind: elevator"},
		{"coaster.transit_present", fxCoaster, "", "kind: lift, transit_s: { mean: 30, sd: 1 }, next", "kind: lift, next"},
		{"coaster.loop_closed", fxCoaster, "", "    next: UNLOAD\nclearance", "    next: LIFT\nclearance"},
		{"coaster.dispatch_interval_order", fxCoaster, "", "target_dispatch_interval_s: 50", "target_dispatch_interval_s: 30"},
		{"coaster.barn_capacity", fxCoaster, "", "storage_bays: 3", "storage_bays: 0"},
		{"coaster.cascade_scope", fxCoaster, "", "auto_resume: false,", "auto_resume: false, scope: everywhere,"},
		{"coaster.seat_rows", fxCoaster, "", "seats_per_row: 2", "seats_per_row: 3"},
		{"coaster.cascade_scope", fxCoaster, "", "blocked_tolerance_s: 5,", "blocked_tolerance_s: 0,"},

		// continuous_load
		{"continuous.route_duration", fxOmnimover, "", "{ id: scene_a, kind: scene, est_s: 60 }", "{ id: scene_a, kind: scene, est_s: 80 }"},
		{"continuous.capacity_ceiling", fxOmnimover, "", "listed_capacity_per_hr: 2000", "listed_capacity_per_hr: 4000"},
		{"continuous.route_structure", fxOmnimover, "", "{ id: unload_zone, kind: unload", "{ id: unload_zone, kind: scene"},
		{"continuous.seats", fxOmnimover, "", "seats_nominal: 2", "seats_nominal: 4"},
		{"continuous.party_policy", fxOmnimover, "", "party_vehicle_policy: contiguous_vehicles", "party_vehicle_policy: interleave"},
		{"preshow.throughput", fxOmnimover, "", "batch_size: 100", "batch_size: 10"},

		// park (file-local)
		{"park.node_ids", fxPark, "", "{ id: filler, kind", "{ id: hub, kind"},
		{"park.edge_refs", fxPark, "", "to: dining", "to: diner"},
		{"park.edge_refs", fxPark, "", "exit_edge: theater_hub", "exit_edge: nowhere"},
		{"park.edge_refs", fxPark, "", "applies_to: [hub_coaster]", "applies_to: [hub_nowhere]"},
		{"park.node_fields", fxPark, "", "capacity_per_hr: 100", "capacity_per_hr: 0"},
		{"park.node_fields", fxPark, "", "capacity_per_hr: 100", "capacity_per_hr: 50"},
		{"park.node_fields", fxPark, "", "seats: 10,", "seats: 1,"},
		{"park.node_fields", fxPark, "", "kind: ride, ride_id: test.fx.coaster,", "kind: ride,"},
		{"park.arrival_shares", fxPark, "", `"10": 0.4`, `"10": 0.5`},
		{"park.party_sizes", fxPark, "", "2: 0.5", "2: 0.4"},
		{"park.walking_mode", fxPark, "", "walking_mode: congestion", "walking_mode: teleport"},
		{"park.hours", fxPark, "", "close_hour: 18", "close_hour: 10"},
		{"park.node_fields", fxPark, "", "visit_min: { mean: 40, sd: 10 }, ", ""},
		{"park.edge_refs", fxPark, "", "applies_to: [hub_coaster]", "applies_to: [gate_hub]"},
		{"park.lightning_lane", fxPark, "", "inventory_share: 0.3", "inventory_share: 1.3"},
		{"park.lightning_lane", fxPark, "", "window_min: 60", "window_min: 0"},
		{"park.lightning_lane", fxPark, "", "advance_bookings: 3", "advance_bookings: -1"},
		{"park.decision", fxPark, "", "temperature: 0.25", "temperature: 0"},
		{"park.decision", fxPark, "", `meal_windows: ["11:30-13:30"]`, `meal_windows: ["13:30-11:30"]`},
		{"park.connected", fxPark, "", "  - { id: exit, kind: exit }", "  - { id: island, kind: plaza, comfortable_guests: 1 }\n  - { id: exit, kind: exit }"},
	}
	for _, c := range cases {
		t.Run(c.check+"/"+c.new, func(t *testing.T) {
			f := patched(t, c.fixture, c.name, c.old, c.new)
			r := resultFor(t, CheckFile(f), c.check)
			if r.Severity != SevError {
				t.Fatalf("%s severity = %v, want error", c.check, r.Severity)
			}
			if r.Status != StatusFail {
				t.Fatalf("%s status = %v, want FAIL (detail %q)", c.check, r.Status, r.Detail)
			}
			if r.Detail == "" {
				t.Errorf("%s: failing check must explain itself", c.check)
			}
		})
	}
}

func TestWarnChecksWarnButDoNotBlock(t *testing.T) {
	cases := []struct{ check, fixture, old, new string }{
		{"continuous.spacing_vs_route", fxOmnimover, "spacing_s: 3.0", "spacing_s: 4.0"},
		{"continuous.loop_time", fxOmnimover, "total_experience_s: 100", "total_experience_s: 300"},
	}
	for _, c := range cases {
		t.Run(c.check, func(t *testing.T) {
			f := patched(t, c.fixture, "", c.old, c.new)
			results := CheckFile(f)
			r := resultFor(t, results, c.check)
			if r.Severity != SevWarn || r.Status != StatusWarn {
				t.Fatalf("%s = %v/%v, want warn/WARN", c.check, r.Severity, r.Status)
			}
			for _, other := range results {
				if other.Status == StatusFail {
					t.Errorf("unexpected FAIL %s: %s", other.CheckID, other.Detail)
				}
			}
		})
	}
}

func TestDatasetChecks(t *testing.T) {
	coaster := patched(t, fxCoaster, "", "", "")

	t.Run("park.ride_refs", func(t *testing.T) {
		park := patched(t, fxPark, "", "ride_id: test.fx.coaster", "ride_id: test.fx.nope")
		rep := Validate(&Dataset{Files: []*File{coaster, park}})
		r := resultFor(t, rep.FileReport(fxPark).Results, "park.ride_refs")
		if r.Status != StatusFail || r.Severity != SevError {
			t.Fatalf("got %v/%v, want error FAIL", r.Severity, r.Status)
		}
		if !rep.HasErrors() {
			t.Fatal("report must have errors")
		}
	})

	t.Run("park.ride_refs resolves", func(t *testing.T) {
		park := patched(t, fxPark, "", "", "")
		rep := Validate(&Dataset{Files: []*File{coaster, park}})
		if r := resultFor(t, rep.FileReport(fxPark).Results, "park.ride_refs"); r.Status != StatusPass {
			t.Fatalf("got %v: %s", r.Status, r.Detail)
		}
	})

	t.Run("dataset.unique_ride_ids", func(t *testing.T) {
		dup := patched(t, fxCoaster, "testdata/other/test-fx-coaster.yaml", "", "")
		rep := Validate(&Dataset{Files: []*File{coaster, dup}})
		r := resultFor(t, rep.FileReport(dup.Path).Results, "dataset.unique_ride_ids")
		if r.Status != StatusFail {
			t.Fatalf("got %v, want FAIL", r.Status)
		}
	})
}

func TestLoaderRejects(t *testing.T) {
	raw, err := os.ReadFile(fxCoaster)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	cases := map[string]string{
		"old version":       strings.Replace(src, `schema_version: "1.0"`, `schema_version: "0.5"`, 1),
		"missing version":   strings.Replace(src, `schema_version: "1.0"`, ``, 1),
		"unknown archetype": strings.Replace(src, "archetype: multi_train_coaster", "archetype: flume", 1),
		"unknown field":     strings.Replace(src, "seats_per_train: 20", "seats_per_train: 20\n  wheels: 8", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadBytes("x.yaml", []byte(body)); err == nil {
				t.Fatal("expected load error")
			}
		})
	}
}

// The real data must load, pass every error check, and show the known Haunted Mansion spacing warning.
func TestRealData(t *testing.T) {
	ds, err := LoadDir(realDataRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, le := range ds.LoadErrors {
		t.Errorf("load error %s: %v", le.Path, le.Err)
	}
	if len(ds.Files) != 20 {
		t.Fatalf("loaded %d files, want 20 (3 rides, 1 park, 13 scenarios, 3 studies)", len(ds.Files))
	}
	rep := Validate(ds)
	for _, fr := range rep.Files {
		for _, r := range fr.Results {
			if r.Status == StatusFail {
				t.Errorf("%s: %s FAIL: %s", fr.Path, r.CheckID, r.Detail)
			}
		}
	}
	hm := rep.FileReport(filepath.Join(realDataRoot, "rides", "wdw-mk-haunted-mansion.yaml"))
	if hm == nil {
		t.Fatal("no Haunted Mansion report")
	}
	if r := resultFor(t, hm.Results, "continuous.spacing_vs_route"); r.Status != StatusWarn {
		t.Errorf("Haunted Mansion spacing check = %v, want WARN (known ~22%% miss)", r.Status)
	}
}
