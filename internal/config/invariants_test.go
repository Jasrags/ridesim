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
	fxElevator   = "testdata/test-fx-elevator.yaml"
	fxBays       = "testdata/test-fx-bays.yaml"
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
		{"continuous.capacity_ceiling", fxOmnimover, "", "topology: closed_loop }", "topology: closed_loop, chains: -1 }"},
		{"continuous.party_policy", fxOmnimover, "", "chain_stop: { rate_per_hr: 0.1, duration_s: { mean: 100, sd: 10 } }", "chain_stop: { rate_per_hr: 0.1, duration_s: { mean: 100, sd: 10 } }\n  side_down: { rate_per_hr: 0.1, duration_s: { mean: 100, sd: 10 } }"}, // side_down on one chain
		{"continuous.stages", fxOmnimover, "", "queues:\n  party_split_policy: contiguous_vehicles", "stages:\n  - { enabled: true, rooms: 2, batch_size: 50, duration_s: { mean: 30, sd: 1 }, turnover_s: { mean: 10, sd: 1 } }\nqueues:\n  party_split_policy: contiguous_vehicles"},

		// coaster further stages (M11)
		{"coaster.stages", fxCoaster, "", "queues:\n  party_split_policy: never", "stages:\n  - { id: cell, enabled: true, rooms: 1, batch_size: 10, duration_s: { mean: 60, sd: 1 }, turnover_s: { mean: 10, sd: 1 } }\nqueues:\n  party_split_policy: never"},                    // below listed
		{"coaster.stages", fxCoaster, "", "queues:\n  party_split_policy: never", "stages:\n  - { enabled: true, rooms: 2, batch_size: 20, duration_s: { mean: 60, sd: 1 }, turnover_s: { mean: 10, sd: 1 } }\nqueues:\n  party_split_policy: never"},                              // no id
		{"coaster.stages", fxCoaster, "", "queues:\n  party_split_policy: never", "stages:\n  - { id: cell, enabled: true, rooms: 2, batch_size: 20, merge_point: true, duration_s: { mean: 60, sd: 1 }, turnover_s: { mean: 10, sd: 1 } }\nqueues:\n  party_split_policy: never"}, // merge
		{"coaster.stages", fxCoaster, "", "queues:\n  party_split_policy: never", "stages:\n  - { id: cell, enabled: true, rooms: 0, batch_size: 20, duration_s: { mean: 60, sd: 1 }, turnover_s: { mean: 10, sd: 1 } }\nqueues:\n  party_split_policy: never"},                    // rooms
		// simulator_bays
		{"bays.count", fxBays, "", "count: 2,", "count: 0,"},
		{"bays.count", fxBays, "", "count: 2,", "count: 1,"}, // side_down needs two bays
		{"bays.seats", fxBays, "", "boarding_groups: [5, 5]", "boarding_groups: [5, 4]"},
		{"bays.seats", fxBays, "", "callout_parties: 5", "callout_parties: -1"},
		{"bays.times", fxBays, "", "flight_s: { mean: 60, sd: 1 }", "flight_s: { mean: 0, sd: 1 }"},
		{"bays.duration", fxBays, "", "ride_duration_s: 60", "ride_duration_s: 90"},
		// elevator_drop
		{"elevator.sides", fxElevator, "", "drop_shafts: 1", "drop_shafts: 0"},
		{"elevator.sides", fxElevator, "", "names: [left, right]", "names: [left]"},
		{"elevator.sides", fxElevator, "", "names: [left, right]", "names: [left, left]"},
		{"elevator.sides", fxElevator, "", "names: [left, right]", "names: [left, \"\"]"},
		{"elevator.sides", fxElevator, "", "names: [left, right]", "names: [left, \"a b\"]"},
		{"elevator.fleet", fxElevator, "", "fleet_size: 6", "fleet_size: 5"},
		{"elevator.no_gridlock", fxElevator, "", "fleet_size: 6, per_side: 3", "fleet_size: 10, per_side: 5"},
		{"elevator.seat_rows", fxElevator, "", "seats_per_row: 1", "seats_per_row: 3"},
		{"elevator.times", fxElevator, "", "drop_s: { mean: 40, sd: 2 }", "drop_s: { mean: 0, sd: 2 }"},
		{"elevator.times", fxElevator, "", "max_wait_after_ready_s: 5", "max_wait_after_ready_s: -1"},
		{"elevator.duration_sum", fxElevator, "", "ride_duration_s: 100", "ride_duration_s: 120"},

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
		{"park.node_fields", fxPark, "", "kind: plaza, comfortable_guests: 100", "kind: shop"},
		{"park.edge_refs", fxPark, "", "applies_to: [hub_coaster]", "applies_to: [gate_hub]"},
		{"park.lightning_lane", fxPark, "", "inventory_share: 0.3", "inventory_share: 1.3"},
		{"park.lightning_lane", fxPark, "", "window_min: 60", "window_min: 0"},
		{"park.lightning_lane", fxPark, "", "advance_bookings: 3", "advance_bookings: -1"},
		{"park.decision", fxPark, "", "temperature: 0.25", "temperature: 0"},
		{"park.decision", fxPark, "", `meal_windows: ["11:30-13:30"]`, `meal_windows: ["13:30-11:30"]`},
		{"park.map_positions", fxPark, "", "kind: entrance, x: 0, y: 300", "kind: entrance, x: 0"},
		{"park.map_positions", fxPark, "", "kind: entrance, x: 0, y: 300", "kind: entrance"},
		{"park.connected", fxPark, "", "  - { id: exit, kind: exit, x: 0, y: 380 }", "  - { id: island, kind: plaza, comfortable_guests: 1, x: 9, y: 9 }\n  - { id: exit, kind: exit, x: 0, y: 380 }"},
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
		{"elevator.capacity", fxElevator, "listed_capacity_per_hr: 1400", "listed_capacity_per_hr: 1680"},
		{"bays.capacity", fxBays, "listed_capacity_per_hr: 720", "listed_capacity_per_hr: 900"},
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
	if len(ds.Files) != 41 {
		t.Fatalf("loaded %d files, want 41 (12 rides, 3 parks, 19 scenarios, 3 studies, 1 scoring, 3 challenges)", len(ds.Files))
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

func TestParkJunctionNeedsNoFields(t *testing.T) {
	f := patched(t, fxPark, "", "kind: plaza, comfortable_guests: 100", "kind: junction")
	if r := resultFor(t, CheckFile(f), "park.node_fields"); r.Status != StatusPass {
		t.Fatalf("junction hub: %s", r.Detail)
	}
}

// tiered patches the park fixture into a tiered Lightning Lane park: the coaster is Tier 1, the filler
// ride a Single Pass.
func tiered(t *testing.T, extra ...[2]string) *File {
	t.Helper()
	b, err := os.ReadFile(fxPark)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, r := range append([][2]string{
		{"ride_id: test.fx.coaster, appeal", "ride_id: test.fx.coaster, lightning_lane: tier1, appeal"},
		{"capacity_per_hr: 100, duration_s: 60, appeal: 0.1", "capacity_per_hr: 300, duration_s: 60, lightning_lane: single_pass, appeal: 0.1"},
		{"  elsewhere_share: 0.5", "  elsewhere_share: 0.5\n  advance_by_tier: { tier1: 1, tier2: 2 }\n  single_pass_share: 0.2"},
	}, extra...) {
		if !strings.Contains(s, r[0]) {
			t.Fatalf("fixture lacks %q", r[0])
		}
		s = strings.Replace(s, r[0], r[1], 1)
	}
	f, err := LoadBytes(fxPark, []byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestParkLightningLaneTiers(t *testing.T) {
	if r := resultFor(t, CheckFile(tiered(t)), "park.lightning_lane"); r.Status != StatusPass {
		t.Fatalf("tiered fixture: %s", r.Detail)
	}
	for name, patch := range map[string][2]string{
		"tier on dining":       {"seats: 10, service", "seats: 10, lightning_lane: tier2, service"},
		"unknown tier":         {"lightning_lane: tier1", "lightning_lane: tier9"},
		"no advance_by_tier":   {"  advance_by_tier: { tier1: 1, tier2: 2 }\n", ""},
		"bad advance tier key": {"advance_by_tier: { tier1: 1, tier2: 2 }", "advance_by_tier: { tier1: 1, gold: 2 }"},
		"single pass share":    {"single_pass_share: 0.2", "single_pass_share: 1.5"},
		"LL share too small":   {"capacity_per_hr: 300,", "capacity_per_hr: 120,"}, // 2 seats: LL may take 1, the largest party is 2
	} {
		if r := resultFor(t, CheckFile(tiered(t, patch)), "park.lightning_lane"); r.Status != StatusFail {
			t.Errorf("%s: got %v (%s), want FAIL", name, r.Status, r.Detail)
		}
	}
}

func TestParkMeetNode(t *testing.T) {
	meet := patched(t, fxPark, "", "kind: ride_simple, capacity_per_hr: 100, duration_s: 60,", "kind: meet, meet_s: { mean: 120, sd: 30 },")
	if r := resultFor(t, CheckFile(meet), "park.node_fields"); r.Status != StatusPass {
		t.Fatalf("meet node: %s", r.Detail)
	}
	bad := patched(t, fxPark, "", "kind: ride_simple, capacity_per_hr: 100, duration_s: 60,", "kind: meet,")
	if r := resultFor(t, CheckFile(bad), "park.node_fields"); r.Status != StatusFail {
		t.Fatalf("meet without meet_s: got %v", r.Status)
	}
}

func TestCalibrationTargetsRunInfo(t *testing.T) {
	ct := CalibrationTargets{EverestAvgStandbyMin: 19, EverestPeakStandbyMin: 35, HauntedMansionAvgStandbyMin: 26, HauntedMansionPeakStandbyMin: 42}
	if got, want := ct.RunInfo(), "everest:19:35,haunted_mansion:26:42"; got != want {
		t.Errorf("legacy targets = %q, want %q", got, want)
	}
	ct = CalibrationTargets{Rides: map[string]StandbyTarget{"slinky_dog_dash": {AvgMin: 63, PeakMin: 83}, "rise_of_the_resistance": {AvgMin: 47, PeakMin: 65}}}
	if got, want := ct.RunInfo(), "rise_of_the_resistance:47:65,slinky_dog_dash:63:83"; got != want {
		t.Errorf("ride targets = %q, want %q (sorted by node)", got, want)
	}
}

func TestParkTierNeedsALightningLaneLine(t *testing.T) {
	park := tiered(t)
	coaster := patched(t, fxCoaster, "", "  skip_line:\n    enabled: true", "  skip_line:\n    enabled: false")
	r := checkParkRideRefs(&Dataset{Files: []*File{coaster, park}}, park.Park)
	if r.Status != StatusFail || !strings.Contains(r.Detail, "Lightning Lane line") {
		t.Fatalf("tier on a ride without a skip line: %v (%s), want FAIL", r.Status, r.Detail)
	}
}

// A valid further stage passes, and a stage without the preshow before it fails.
func TestStagesValidAndNeedPreshow(t *testing.T) {
	stage := "stages:\n  - { id: cell, enabled: true, rooms: 2, batch_size: 20, duration_s: { mean: 60, sd: 1 }, turnover_s: { mean: 10, sd: 1 } }\nqueues:\n  party_split_policy: never"
	f := patched(t, fxCoaster, "", "queues:\n  party_split_policy: never", stage)
	if r := resultFor(t, CheckFile(f), "coaster.stages"); r.Status != StatusPass {
		t.Fatalf("valid stage: %s %s", r.Status, r.Detail)
	}
	f.Coaster.Preshow.LoadAreaCapacity = 10 // below the cell's batch of 20: its rooms could never fill
	if r := resultFor(t, CheckFile(f), "coaster.stages"); r.Status != StatusFail || r.Severity != SevError {
		t.Fatalf("holding area below the next batch: %s %s", r.Status, r.Detail)
	}
	f.Coaster.Preshow.LoadAreaCapacity = 0
	f.Coaster.Preshow.Enabled = false
	if r := resultFor(t, CheckFile(f), "coaster.stages"); r.Status != StatusFail {
		t.Fatalf("stage without preshow: %s %s", r.Status, r.Detail)
	}
}

// Shared vehicles are a valid party policy, and chains multiply the capacity ceiling.
func TestContinuousSharedPolicyAndChains(t *testing.T) {
	f := patched(t, fxOmnimover, "", "party_vehicle_policy: contiguous_vehicles", "party_vehicle_policy: shared")
	if r := resultFor(t, CheckFile(f), "continuous.party_policy"); r.Status != StatusPass {
		t.Fatalf("shared policy: %s %s", r.Status, r.Detail)
	}
	f = patched(t, fxOmnimover, "", "listed_capacity_per_hr: 2000", "listed_capacity_per_hr: 4000")
	f.Continuous.Conveyor.Chains = 2
	if r := resultFor(t, CheckFile(f), "continuous.capacity_ceiling"); r.Status != StatusPass {
		t.Fatalf("2 chains, listed 4,000: %s %s", r.Status, r.Detail)
	}
}

// A ride built below its listed figure checks the ceiling against its target instead.
func TestContinuousCeilingUsesTheTarget(t *testing.T) {
	f := patched(t, fxOmnimover, "", "listed_capacity_per_hr: 2000", "listed_capacity_per_hr: 4000")
	if r := resultFor(t, CheckFile(f), "continuous.capacity_ceiling"); r.Status != StatusFail {
		t.Fatalf("listed 4,000 over the ceiling with no target: %s %s", r.Status, r.Detail)
	}
	f.Continuous.ValidationTargets.TheoreticalRidersPerHrAtTarget = 2000
	if r := resultFor(t, CheckFile(f), "continuous.capacity_ceiling"); r.Status != StatusPass {
		t.Fatalf("target 2,000 under the ceiling: %s %s", r.Status, r.Detail)
	}
}

// opening_delay needs a probability in 0..1 and a positive duration.
func TestOpeningDelayChecked(t *testing.T) {
	f := patched(t, fxCoaster, "", "queues:\n  party_split_policy: never", "queues:\n  party_split_policy: never")
	f.Coaster.Failures.OpeningDelay = &OpeningDelay{Probability: 0.25, DurationS: Dist{Mean: 600}}
	if r := resultFor(t, CheckFile(f), "coaster.opening_delay"); r.Status != StatusPass {
		t.Fatalf("valid opening_delay: %s %s", r.Status, r.Detail)
	}
	f.Coaster.Failures.OpeningDelay.Probability = 1.5
	if r := resultFor(t, CheckFile(f), "coaster.opening_delay"); r.Status != StatusFail || r.Severity != SevError {
		t.Fatalf("probability 1.5: %s %s", r.Status, r.Detail)
	}
}
