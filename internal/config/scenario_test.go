package config

import "testing"

const (
	fxScenario     = "testdata/test-fx-scenario.yaml"
	fxParkScenario = "testdata/test-fx-park-scenario.yaml"
)

func TestScenarioFixtureValid(t *testing.T) {
	coaster := patched(t, fxCoaster, "", "", "")
	sc := patched(t, fxScenario, "", "", "")
	rep := Validate(&Dataset{Files: []*File{coaster, sc}})
	for _, r := range rep.FileReport(fxScenario).Results {
		if r.Status != StatusPass {
			t.Errorf("%s: %s: %s", r.CheckID, r.Status, r.Detail)
		}
	}
}

func TestScenarioErrorChecks(t *testing.T) {
	cases := []struct{ check, old, new string }{
		{"scenario.basics", "id: fx_scenario", "id: \"\""},
		{"scenario.basics", "ride_id: test.fx.coaster", "ride_id: coaster"},
		{"scenario.basics", "warmup_min: 30", "warmup_min: 300"},
		{"scenario.arrivals", `{ at: "00:00", rate_per_hr: 800 }`, `{ at: "00:10", rate_per_hr: 800 }`},
		{"scenario.arrivals", `{ at: "02:00", rate_per_hr: 400 }`, `{ at: "02:00", rate_per_hr: -1 }`},
		{"scenario.arrivals", `{ at: "02:00", rate_per_hr: 400 }`, `{ at: "2h", rate_per_hr: 400 }`},
		{"scenario.ops", "action: add_train", "action: teleport_train"},
		{"scenario.ops", `{ at: "03:00", action: remove_train }`, `{ at: "05:00", action: remove_train }`},
		{"scenario.ops", `{ at: "03:00", action: remove_train }`, `{ at: "03:00", action: remove_train, node: x }`},
		{"scenario.weather", `{ from: "02:30", to: "02:45" }`, `{ from: "02:45", to: "02:30" }`},
		{"scenario.weather", "  guests:\n", "  single_rider_closed: [{ from: \"03:00\", to: \"09:00\" }]\n  guests:\n"},
		{"scenario.enums", "failures: [cascade_stop]", "failures: [meteor]"},
		{"scenario.enums", "dispatch_rule: dispatch_when_ready", "dispatch_rule: yolo"},
		{"scenario.enums", "algorithm: last_n_observed", "algorithm: vibes"},
		{"scenario.arrivals", `arrivals: [{ at: "00:00", rate_per_hr: 100 }]`, `arrivals: [{ at: "00:05", rate_per_hr: 100 }]`},
		{"scenario.guests", "party_size: { 1: 0.5, 2: 0.5 }", "party_size: { 1: 0.5, 2: 0.4 }"},
		{"scenario.guests", "party_size: { 1: 0.5, 2: 0.5 }", "party_size: { 0: 0.5, 2: 0.5 }"},
		{"scenario.guests", "single_rider_share: 0.1", "single_rider_share: 1.5"},
		{"scenario.guests", "patience_min: { mean: 30, sd: 5 }", "patience_min: { mean: 0, sd: 5 }"},
	}
	for _, c := range cases {
		t.Run(c.check+"/"+c.new, func(t *testing.T) {
			r := resultFor(t, CheckFile(patched(t, fxScenario, "", c.old, c.new)), c.check)
			if r.Status != StatusFail || r.Severity != SevError {
				t.Fatalf("got %v/%v (%s), want error FAIL", r.Severity, r.Status, r.Detail)
			}
		})
	}
}

func TestScenarioRideRefs(t *testing.T) {
	cases := []struct{ name, fixture, old, new string }{
		{"unknown ride", fxScenario, "ride_id: test.fx.coaster", "ride_id: test.fx.nope"},
		{"too many trains", fxScenario, "trains: 2", "trains: 4"},
		{"ops exceed max", fxScenario, "trains: 2", "trains: 3"},
		{"ops need barn", fxCoaster, "barn: { location: after_station, storage_bays: 3, swap_time_s: { mean: 100, sd: 10 } }\n", ""},
		{"failure not configured", fxScenario, "failures: [cascade_stop]", "failures: [brief_stop]"},
		{"unknown park", fxScenario, "single_rider_share: 0.1", "single_rider_share: 0.1\n    park_id: nowhere"},
		{"ll without skip line", fxCoaster, "    enabled: true\n    merge:", "    enabled: false\n    merge:"},
		{"sr closed without line", fxScenario, "  guests:\n", "  single_rider_closed: [{ from: \"01:00\", to: \"02:00\" }]\n  guests:\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			coaster, sc := patched(t, fxCoaster, "", "", ""), patched(t, fxScenario, "", "", "")
			if c.fixture == fxCoaster {
				coaster = patched(t, fxCoaster, "", c.old, c.new)
			} else {
				sc = patched(t, fxScenario, "", c.old, c.new)
			}
			r := resultFor(t, Validate(&Dataset{Files: []*File{coaster, sc}}).FileReport(fxScenario).Results, "scenario.ride_refs")
			if r.Status != StatusFail {
				t.Fatalf("got %v (%s), want FAIL", r.Status, r.Detail)
			}
		})
	}
}

func TestRideEnumChecks(t *testing.T) {
	for _, c := range []struct{ check, old, new string }{
		{"coaster.dispatch_rule", "dispatch_rule: hold_for_full_or_max_wait", "dispatch_rule: whenever"},
		{"queues.posted_wait", "algorithm: queue_len_over_throughput", "algorithm: guess"},
	} {
		r := resultFor(t, CheckFile(patched(t, fxCoaster, "", c.old, c.new)), c.check)
		if r.Status != StatusFail {
			t.Errorf("%s: got %v, want FAIL", c.check, r.Status)
		}
	}
}

func TestParseOffset(t *testing.T) {
	for in, want := range map[string]float64{"00:00": 0, "01:30": 5400, "02:00:30": 7230} {
		if got, err := ParseOffset(in); err != nil || got != want {
			t.Errorf("ParseOffset(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"1", "01:60", "a:b", "1:2:3:4"} {
		if _, err := ParseOffset(bad); err == nil {
			t.Errorf("ParseOffset(%q) accepted", bad)
		}
	}
}

func TestStudyBasicsChecks(t *testing.T) {
	good := []byte("schema_version: \"1.0\"\nstudy:\n  id: s\n  base: b\n  variants:\n    - { id: v, scale: { arrivals: 1.2 } }\n")
	f, err := LoadBytes("s.yaml", good)
	if err != nil {
		t.Fatal(err)
	}
	if r := resultFor(t, CheckFile(f), "study.basics"); r.Status != StatusPass {
		t.Fatalf("valid study: %s", r.Detail)
	}
	for name, body := range map[string]string{
		"bad scale key": "schema_version: \"1.0\"\nstudy:\n  id: s\n  base: b\n  variants:\n    - { id: v, scale: { trains: 2 } }\n",
		"dup ids":       "schema_version: \"1.0\"\nstudy:\n  id: s\n  base: b\n  variants:\n    - { id: v }\n    - { id: v }\n",
		"no base":       "schema_version: \"1.0\"\nstudy:\n  id: s\n",
	} {
		f, err := LoadBytes("s.yaml", []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if r := resultFor(t, CheckFile(f), "study.basics"); r.Status != StatusFail {
			t.Errorf("%s: got %v", name, r.Status)
		}
	}
}

func parkScenarioDataset(t *testing.T, old, new string) *Dataset {
	t.Helper()
	return &Dataset{Files: []*File{patched(t, fxCoaster, "", "", ""), patched(t, fxPark, "", "", ""),
		patched(t, fxOmnimover, "", "", ""), patched(t, fxParkScenario, "", old, new)}}
}

func TestParkScenarioFixtureValid(t *testing.T) {
	for _, r := range Validate(parkScenarioDataset(t, "", "")).FileReport(fxParkScenario).Results {
		if r.Status != StatusPass {
			t.Errorf("%s: %s: %s", r.CheckID, r.Status, r.Detail)
		}
	}
}

func TestParkScenarioChecks(t *testing.T) {
	cases := []struct{ check, old, new string }{
		{"scenario.basics", "park_id: fx_park", "park_id: fx_park\n  ride_id: test.fx.coaster"},
		{"scenario.basics", "crowd_level: medium", "crowd_level: medium\n  trains: 2"},
		{"scenario.basics", "crowd_level: medium", "crowd_level: medium\n  arrivals: [{ at: \"00:00\", rate_per_hr: 100 }]"},
		{"scenario.basics", "crowd_level: medium", "crowd_level: medium\n  attendance: 500"},
		{"scenario.basics", "  crowd_level: medium\n", ""},
		{"scenario.enums", "crowd_level: medium", "crowd_level: mob"},
		{"scenario.enums", "walking_mode: fixed_delay", "walking_mode: teleport"},
		{"scenario.ride_refs", "park_id: fx_park", "park_id: nowhere"},
		{"scenario.ops", "action: crowd_scale, value: 1.5", "action: crowd_scale, value: 2.5"},
		{"scenario.ops", "action: crowd_scale, value: 1.5", "action: crowd_scale"},
		{"scenario.ops", "action: close_ride, node: coaster, minutes: 20", "action: close_ride, node: coaster"},
		{"scenario.ops", "action: remove_train, node: coaster", "action: remove_train"},
		{"scenario.ops", "action: crowd_scale, value: 1.5", "action: teleport"},
		{"scenario.ride_refs", "action: close_ride, node: coaster", "action: close_ride, node: nowhere"},
		{"scenario.ride_refs", "action: add_train, node: coaster", "action: add_train, node: filler"},
		{"scenario.ride_refs", "action: close_ride, node: coaster", "action: close_ride, node: dining"},
	}
	for _, c := range cases {
		t.Run(c.check+"/"+c.new, func(t *testing.T) {
			r := resultFor(t, Validate(parkScenarioDataset(t, c.old, c.new)).FileReport(fxParkScenario).Results, c.check)
			if r.Status != StatusFail || r.Severity != SevError {
				t.Fatalf("got %v/%v (%s), want error FAIL", r.Severity, r.Status, r.Detail)
			}
		})
	}
}
