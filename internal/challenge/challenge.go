// Package challenge plays and checks M15 challenges: a park scenario, a scoring file, goals and the levers a
// player may pull. A played session (the control room's export) is verified by checking it is the
// challenge's day with only allowed levers added, then rerunning it; the rerun's score is the one recorded.
package challenge

import (
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/metrics"
	"github.com/jasrags/ridesim/internal/scenario"
	"github.com/jasrags/ridesim/internal/score"
)

// Challenge is a loaded, cross-checked challenge.
type Challenge struct {
	*config.Challenge
	Scenario *config.ScenarioFile
	Scoring  config.Scoring
}

// Load finds challenge id in a validated dataset.
func Load(ds *config.Dataset, id string) (*Challenge, error) {
	f := ds.Challenge(id)
	if f == nil {
		return nil, fmt.Errorf("challenge %q not found", id)
	}
	return FromFile(ds, f.Challenge)
}

// FromFile resolves a challenge file's scenario and scoring.
func FromFile(ds *config.Dataset, cf *config.ChallengeFile) (*Challenge, error) {
	c := cf.Challenge
	sf := ds.Scenario(c.Scenario)
	sc := ds.Scoring(c.Scoring)
	if sf == nil || sc == nil {
		return nil, fmt.Errorf("challenge %s: scenario %q or scoring %q not found", c.ID, c.Scenario, c.Scoring)
	}
	return &Challenge{Challenge: &cf.Challenge, Scenario: sf.Scenario, Scoring: sc.Scoring.Scoring}, nil
}

// DailySeed is the seed for a play date: YYYYMMDD as a number, so every player of a date gets one day.
func DailySeed(date string) (uint64, error) {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, fmt.Errorf("date %q: want YYYY-MM-DD", date)
	}
	return strconv.ParseUint(d.Format("20060102"), 10, 64)
}

// Seed is the seed a play uses: the date's for a daily challenge, else the scenario's. date is ignored
// for a fixed-seed challenge.
func (c *Challenge) Seed(date string) (uint64, error) {
	if !c.DailySeed {
		return c.Scenario.Scenario.Seed, nil
	}
	if date == "" {
		return 0, fmt.Errorf("challenge %s uses a daily seed: give the play date", c.ID)
	}
	return DailySeed(date)
}

// Day is the scenario file to play for a seed (a copy; the dataset is unchanged).
func (c *Challenge) Day(seed uint64) *config.ScenarioFile {
	f := *c.Scenario
	f.Scenario.Seed = seed
	return &f
}

// GoalResult is one goal checked against a day.
type GoalResult struct {
	Goal    config.Goal
	Value   float64
	Met     bool
	Pending bool // the value needs the whole day (posted waits) and the day is not over
}

// Label is the goal as text, e.g. "posted_avg_min slinky <= 60".
func (r GoalResult) Label() string {
	g := r.Goal
	s := g.Metric
	if g.Node != "" {
		s += " " + g.Node
	}
	if g.AtLeast != nil {
		return fmt.Sprintf("%s >= %g", s, *g.AtLeast)
	}
	return fmt.Sprintf("%s <= %g", s, *g.AtMost)
}

// Evaluate checks every goal. pr is the day's park report; nil leaves posted-wait goals pending.
func (c *Challenge) Evaluate(card score.Card, pr *metrics.ParkReport) []GoalResult {
	out := make([]GoalResult, 0, len(c.Goals))
	for _, g := range c.Goals {
		r := GoalResult{Goal: g}
		switch g.Metric {
		case config.MetricScore:
			r.Value = card.Score()
		case config.MetricExperiences:
			r.Value = card.ExperiencesPerGuest()
		case config.MetricBalks:
			r.Value = float64(card.Balks)
		case config.MetricReneges:
			r.Value = float64(card.Reneges)
		case config.MetricLLLapsed:
			r.Value = float64(card.LLLapsed)
		case config.MetricLLRedeemed:
			r.Value = float64(card.LLRedeemed)
		case config.MetricPostedAvgMin, config.MetricPostedPeakMin:
			n := (*metrics.NodeReport)(nil)
			if pr != nil {
				n = pr.Node(g.Node)
			}
			if n == nil {
				r.Pending = true
				out = append(out, r)
				continue
			}
			r.Value = n.Report.PostedMean
			if g.Metric == config.MetricPostedPeakMin {
				r.Value = n.Report.PostedPeak
			}
		}
		r.Met = g.AtLeast != nil && r.Value >= *g.AtLeast || g.AtMost != nil && r.Value <= *g.AtMost
		out = append(out, r)
	}
	return out
}

// Outcome is a scored day.
type Outcome struct {
	Seed   uint64
	Card   score.Card
	Report *metrics.ParkReport
	Goals  []GoalResult
	Levers int // live levers the player pulled
	Log    *engine.Log
}

// Met counts goals met.
func (o *Outcome) Met() int {
	n := 0
	for _, g := range o.Goals {
		if g.Met {
			n++
		}
	}
	return n
}

// Run plays the challenge's day with no levers (the baseline) at seed.
func (c *Challenge) Run(ds *config.Dataset, seed uint64) (*Outcome, error) {
	return c.score(ds, c.Day(seed), 0)
}

func (c *Challenge) score(ds *config.Dataset, sf *config.ScenarioFile, levers int) (*Outcome, error) {
	run, err := scenario.Load(ds, sf)
	if err != nil {
		return nil, err
	}
	log, err := run.Run()
	if err != nil {
		return nil, err
	}
	pr, err := metrics.ComputePark(log.Events(), run.WarmupDuration())
	if err != nil {
		return nil, err
	}
	card := score.Compute(log.Events(), c.Scoring)
	return &Outcome{Seed: sf.Scenario.Seed, Card: card, Report: pr, Goals: c.Evaluate(card, pr), Levers: levers, Log: log}, nil
}

// Verify checks that a played session is this challenge's day at seed with only allowed levers added,
// then reruns it and scores the rerun.
func (c *Challenge) Verify(ds *config.Dataset, session *config.ScenarioFile, seed uint64) (*Outcome, error) {
	levers, err := c.checkSession(ds, session, seed)
	if err != nil {
		return nil, fmt.Errorf("session is not a valid play of %s: %w", c.ID, err)
	}
	return c.score(ds, session, levers)
}

// checkSession returns the number of live levers in session.
func (c *Challenge) checkSession(ds *config.Dataset, session *config.ScenarioFile, seed uint64) (int, error) {
	base := c.Day(seed)
	if session.Scenario.Seed != seed {
		return 0, fmt.Errorf("seed %d, want %d", session.Scenario.Seed, seed)
	}
	a, b := base.Scenario, session.Scenario
	a.ID, b.ID, a.Description, b.Description, a.Ops, b.Ops = "", "", "", "", nil, nil
	if !reflect.DeepEqual(a, b) {
		return 0, fmt.Errorf("its settings differ from scenario %s (only ops may be added)", base.Scenario.ID)
	}
	scripted, err := opsOf(base.Scenario.Ops)
	if err != nil {
		return 0, err
	}
	played, err := opsOf(session.Scenario.Ops)
	if err != nil {
		return 0, err
	}
	for _, o := range scripted {
		i := indexOf(played, o)
		if i < 0 {
			return 0, fmt.Errorf("scripted %s at %s is missing", o.Action, o.At)
		}
		played = append(played[:i], played[i+1:]...)
	}
	for _, o := range played {
		if !c.Allows(o.Action) {
			return 0, fmt.Errorf("lever %s at %s is not allowed in this challenge", o.Action, o.At)
		}
	}
	vds := &config.Dataset{Files: append(append([]*config.File{}, ds.Files...), &config.File{Path: "(session)", Kind: config.KindScenario, Scenario: session})}
	for _, r := range config.Validate(vds).FileReport("(session)").Results {
		if r.Status == config.StatusFail {
			return 0, fmt.Errorf("%s: %s", r.CheckID, r.Detail)
		}
	}
	return len(played), nil
}

// op is an ops event with its time normalized ("01:00" and "01:00:00" are the same op).
type op struct {
	config.OpsEvent
	sec float64
}

func opsOf(evs []config.OpsEvent) ([]op, error) {
	out := make([]op, 0, len(evs))
	for _, e := range evs {
		sec, err := config.ParseOffset(e.At)
		if err != nil {
			return nil, err
		}
		out = append(out, op{e, sec})
	}
	return out, nil
}

func indexOf(ops []op, o op) int {
	for i, p := range ops {
		if p.sec == o.sec && p.Action == o.Action && p.Node == o.Node && p.Minutes == o.Minutes &&
			(p.Value == nil) == (o.Value == nil) && (p.Value == nil || *p.Value == *o.Value) {
			return i
		}
	}
	return -1
}
