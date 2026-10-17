package config

import "testing"

const fxScenario = "testdata/test-fx-scenario.yaml"

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
		{"scenario.weather", `{ from: "02:30", to: "02:45" }`, `{ from: "02:45", to: "02:30" }`},
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
