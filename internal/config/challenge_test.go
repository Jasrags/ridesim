package config

import "testing"

const (
	fxScoring   = "testdata/test-fx-scoring.yaml"
	fxChallenge = "testdata/test-fx-challenge.yaml"
)

// challengeDataset is the park scenario fixture set plus scoring and challenge, one of them patched.
func challengeDataset(t *testing.T, fixture, old, new string) *Dataset {
	t.Helper()
	ds := parkScenarioDataset(t, "", "")
	for _, fx := range []string{fxScoring, fxChallenge} {
		o, n := "", ""
		if fx == fixture {
			o, n = old, new
		}
		ds.Files = append(ds.Files, patched(t, fx, "", o, n))
	}
	return ds
}

func TestChallengeFixturesValid(t *testing.T) {
	rep := Validate(challengeDataset(t, "", "", ""))
	for _, fx := range []string{fxScoring, fxChallenge} {
		for _, r := range rep.FileReport(fx).Results {
			if r.Status != StatusPass {
				t.Errorf("%s: %s: %s: %s", fx, r.CheckID, r.Status, r.Detail)
			}
		}
	}
}

func TestChallengeChecks(t *testing.T) {
	cases := []struct{ check, fixture, old, new string }{
		{"scoring.basics", fxScoring, "id: fx_scoring", "id: \"\""},
		{"scoring.basics", fxScoring, "balk_penalty: 2", "balk_penalty: -2"},
		{"scoring.basics", fxScoring, "ride: 10", "ride: 0"},
		{"challenge.basics", fxChallenge, "title: \"Fixture day\"", "title: \"\""},
		{"challenge.basics", fxChallenge, "[ll_merge, ll_inventory]", "[ll_merge, crowd_scale]"},
		{"challenge.basics", fxChallenge, "[ll_merge, ll_inventory]", "[ll_merge, ll_merge]"},
		{"challenge.basics", fxChallenge, "metric: score,", "metric: happiness,"},
		{"challenge.basics", fxChallenge, "at_least: 5 }", "at_least: 5, at_most: 9 }"},
		{"challenge.basics", fxChallenge, "metric: score,", "metric: score, node: coaster,"},
		{"challenge.basics", fxChallenge, "node: coaster, at_most", "at_most"},
		{"challenge.basics", fxChallenge, "goals:\n    - { metric: score, at_least: 5 }\n    - { metric: posted_avg_min, node: coaster, at_most: 30 }", "goals: []"},
		{"challenge.refs", fxChallenge, "scenario: fx_park_day", "scenario: nowhere"},
		{"challenge.refs", fxChallenge, "scoring: fx_scoring", "scoring: nowhere"},
		{"challenge.refs", fxChallenge, "node: coaster", "node: dining"},
	}
	for _, c := range cases {
		t.Run(c.check+"/"+c.new, func(t *testing.T) {
			r := resultFor(t, Validate(challengeDataset(t, c.fixture, c.old, c.new)).FileReport(c.fixture).Results, c.check)
			if r.Status != StatusFail || r.Severity != SevError {
				t.Fatalf("got %v/%v (%s), want error FAIL", r.Severity, r.Status, r.Detail)
			}
		})
	}
}
