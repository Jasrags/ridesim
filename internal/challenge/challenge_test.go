package challenge_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasrags/ridesim/internal/challenge"
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/live"
	"github.com/jasrags/ridesim/internal/score"
)

func fixture(t *testing.T) (*config.Dataset, *challenge.Challenge) {
	t.Helper()
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	c, err := challenge.Load(ds, "fx_challenge")
	if err != nil {
		t.Fatal(err)
	}
	return ds, c
}

func val(v float64) *float64 { return &v }

// play runs a live session of the challenge day with the given levers and returns the export and the
// score of the live log.
func play(t *testing.T, ds *config.Dataset, c *challenge.Challenge, seed uint64, levers []config.OpsEvent) (*config.ScenarioFile, score.Card) {
	t.Helper()
	s, err := live.New(ds, c.Day(seed))
	if err != nil {
		t.Fatal(err)
	}
	for i, op := range levers {
		if err := s.Advance(engine.Duration(i+1) * 30 * engine.Minute); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(op); err != nil {
			t.Fatalf("lever %+v: %v", op, err)
		}
	}
	if err := s.Advance(s.End().Sub(s.Now())); err != nil {
		t.Fatal(err)
	}
	f, err := config.LoadBytes("session.yaml", s.Export("played"))
	if err != nil {
		t.Fatal(err)
	}
	return f.Scenario, score.Compute(s.Log().Events(), c.Scoring)
}

func TestBaselineIsDeterministicAndChecksGoals(t *testing.T) {
	ds, c := fixture(t)
	a, err := c.Run(ds, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := c.Run(ds, 1)
	if a.Card != b.Card {
		t.Fatal("baseline scores differ between runs")
	}
	if len(a.Goals) != 2 || a.Goals[1].Pending || a.Goals[0].Value != a.Card.Score() {
		t.Fatalf("goals %+v", a.Goals)
	}
}

// A played session verifies, and the rerun scores exactly what the live day scored.
func TestVerifyRerunsAPlayedSession(t *testing.T) {
	ds, c := fixture(t)
	session, liveCard := play(t, ds, c, 1, []config.OpsEvent{
		{Action: config.OpLLMerge, Node: "coaster", Value: val(1)},
		{Action: config.OpLLInventory, Node: "coaster", Value: val(0.1)},
	})
	out, err := c.Verify(ds, session, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Card != liveCard {
		t.Fatalf("rerun card %+v\n live card %+v", out.Card, liveCard)
	}
	if out.Levers != 2 {
		t.Errorf("%d levers counted, want 2", out.Levers)
	}
}

func TestVerifyRejectsAnInvalidPlay(t *testing.T) {
	ds, c := fixture(t)
	cheat, _ := play(t, ds, c, 1, []config.OpsEvent{{Action: config.OpCloseRide, Node: "coaster", Minutes: 10}})
	if _, err := c.Verify(ds, cheat, 1); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("close_ride (not allowed): %v", err)
	}
	ok, _ := play(t, ds, c, 1, nil)
	if _, err := c.Verify(ds, ok, 2); err == nil {
		t.Error("wrong seed accepted")
	}
	crowd := *ok
	crowd.Scenario.CrowdLevel = config.CrowdLow
	if _, err := c.Verify(ds, &crowd, 1); err == nil {
		t.Error("changed crowd level accepted")
	}
	dropped := *ok
	dropped.Scenario.Ops = dropped.Scenario.Ops[1:]
	if _, err := c.Verify(ds, &dropped, 1); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("dropped scripted op: %v", err)
	}
}

func TestDailySeed(t *testing.T) {
	if s, err := challenge.DailySeed("2026-10-19"); err != nil || s != 20261019 {
		t.Fatalf("seed %d, %v", s, err)
	}
	for _, bad := range []string{"", "2026-13-01", "19/10/2026"} {
		if _, err := challenge.DailySeed(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestBoardRanksAndRejectsDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b, err := challenge.LoadBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []challenge.Entry{
		{Challenge: "c", Seed: 1, Name: "ann", Score: 10, GoalsMet: 1, Session: "a"},
		{Challenge: "c", Seed: 1, Name: "bob", Score: 12, GoalsMet: 0, Session: "b"},
		{Challenge: "c", Seed: 1, Name: "cat", Score: 10, GoalsMet: 2, Session: "c"},
		{Challenge: "c", Seed: 2, Name: "dan", Score: 99, Session: "d"},
	} {
		if err := b.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Add(challenge.Entry{Challenge: "c", Seed: 1, Name: "eve", Session: "a"}); err == nil {
		t.Error("duplicate session accepted")
	}
	if err := b.Add(challenge.Entry{Challenge: "c", Seed: 1, Name: " ", Session: "z"}); err == nil {
		t.Error("blank name accepted")
	}
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	b, err = challenge.LoadBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range b.Top("c", 1) {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "bob,cat,ann" {
		t.Errorf("ranking %s, want bob,cat,ann", got)
	}
}
