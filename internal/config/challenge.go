package config

import (
	"fmt"
	"sort"
	"strings"
)

// ScoringFile holds the guest-experience score weights (M15, decision 2026-10-19). Every weight is
// [assumed]; changes go in docs/tuning-log.md.
type ScoringFile struct {
	SchemaVersion string  `yaml:"schema_version"`
	Scoring       Scoring `yaml:"scoring"`
}

// Scoring weights, all in points per guest.
type Scoring struct {
	ID                string           `yaml:"id"`
	Description       string           `yaml:"description"`
	Experience        ExperiencePoints `yaml:"experience"`           // per completed visit
	WaitComfortMin    float64          `yaml:"wait_comfort_min"`     // a visit's wait up to this costs nothing
	WaitPenaltyPerMin float64          `yaml:"wait_penalty_per_min"` // per minute past the comfort threshold
	BalkPenalty       float64          `yaml:"balk_penalty"`
	RenegePenalty     float64          `yaml:"renege_penalty"`
	WalkPenaltyPerMin float64          `yaml:"walk_penalty_per_min"`
	JamPenaltyPerMin  float64          `yaml:"jam_penalty_per_min"` // waiting to enter a jammed walkway
	LLLapsePenalty    float64          `yaml:"ll_lapse_penalty"`    // per guest on a pass that lapsed
}

// ExperiencePoints by node class: modeled and simple rides, shows, character meets.
type ExperiencePoints struct {
	Ride float64 `yaml:"ride"`
	Show float64 `yaml:"show"`
	Meet float64 `yaml:"meet"`
}

// ChallengeFile is a park scenario plus goals and the levers a player may pull (M15).
type ChallengeFile struct {
	SchemaVersion string    `yaml:"schema_version"`
	Challenge     Challenge `yaml:"challenge"`
}

type Challenge struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Scenario    string   `yaml:"scenario"`   // park scenario id
	Scoring     string   `yaml:"scoring"`    // scoring id
	DailySeed   bool     `yaml:"daily_seed"` // seed = the play date as YYYYMMDD instead of the scenario's
	Levers      []string `yaml:"levers"`     // park ops actions allowed in play
	Goals       []Goal   `yaml:"goals"`
}

// Goal is one pass/fail target on a day metric; exactly one of AtLeast and AtMost is set.
type Goal struct {
	Metric  string   `yaml:"metric"`
	Node    string   `yaml:"node,omitempty"` // posted_* metrics only
	AtLeast *float64 `yaml:"at_least,omitempty"`
	AtMost  *float64 `yaml:"at_most,omitempty"`
}

// Goal metrics.
const (
	MetricScore         = "score"                 // points per guest
	MetricExperiences   = "experiences_per_guest" // rides, shows and meets completed per guest
	MetricBalks         = "balks"                 // guests
	MetricReneges       = "reneges"               // guests
	MetricLLLapsed      = "ll_lapsed"             // passes
	MetricLLRedeemed    = "ll_redeemed"           // passes used: the park still serves its paying guests
	MetricPostedAvgMin  = "posted_avg_min"        // a node's mean posted wait (report window)
	MetricPostedPeakMin = "posted_peak_min"       // a node's highest clock-hour posted wait
)

var (
	validMetrics = map[string]bool{MetricScore: true, MetricExperiences: true, MetricBalks: true, MetricReneges: true,
		MetricLLLapsed: true, MetricLLRedeemed: true, MetricPostedAvgMin: true, MetricPostedPeakMin: true}
	// Levers a challenge may allow. crowd_scale is never one: it would change the day being scored.
	challengeLevers = map[string]bool{OpAddTrain: true, OpRemoveTrain: true, OpSingleRiderOpen: true, OpSingleRiderClose: true,
		OpCloseRide: true, OpLLInventory: true, OpLLMerge: true}
)

// NodeMetric reports whether a goal metric names a node.
func NodeMetric(m string) bool { return m == MetricPostedAvgMin || m == MetricPostedPeakMin }

// Allows reports whether the challenge lets a player pull action.
func (c *Challenge) Allows(action string) bool {
	for _, l := range c.Levers {
		if l == action {
			return true
		}
	}
	return false
}

var scoringChecks = []check[*ScoringFile]{
	{"scoring.basics", SevError, checkScoringBasics},
}

var challengeChecks = []check[*ChallengeFile]{
	{"challenge.basics", SevError, checkChallengeBasics},
}

func checkScoringBasics(f *ScoringFile) (bool, string) {
	s := f.Scoring
	var bad []string
	if s.ID == "" {
		bad = append(bad, "scoring needs an id")
	}
	weights := []struct {
		name string
		v    float64
	}{{"experience.ride", s.Experience.Ride}, {"experience.show", s.Experience.Show}, {"experience.meet", s.Experience.Meet},
		{"wait_comfort_min", s.WaitComfortMin}, {"wait_penalty_per_min", s.WaitPenaltyPerMin}, {"balk_penalty", s.BalkPenalty},
		{"renege_penalty", s.RenegePenalty}, {"walk_penalty_per_min", s.WalkPenaltyPerMin}, {"jam_penalty_per_min", s.JamPenaltyPerMin},
		{"ll_lapse_penalty", s.LLLapsePenalty}}
	for _, w := range weights {
		if w.v < 0 {
			bad = append(bad, fmt.Sprintf("%s %g must be >= 0 (penalties are subtracted)", w.name, w.v))
		}
	}
	if s.Experience.Ride <= 0 {
		bad = append(bad, "experience.ride must be > 0")
	}
	if len(bad) > 0 {
		return false, strings.Join(bad, "; ")
	}
	return true, fmt.Sprintf("ride %g, show %g, meet %g points; waits over %g min cost %g/min", s.Experience.Ride, s.Experience.Show,
		s.Experience.Meet, s.WaitComfortMin, s.WaitPenaltyPerMin)
}

func checkChallengeBasics(f *ChallengeFile) (bool, string) {
	c := f.Challenge
	var bad []string
	if c.ID == "" || c.Title == "" || c.Scenario == "" || c.Scoring == "" {
		bad = append(bad, "challenge needs id, title, scenario and scoring")
	}
	seen := map[string]bool{}
	for _, l := range c.Levers {
		if !challengeLevers[l] || seen[l] {
			bad = append(bad, fmt.Sprintf("lever %q is not an allowed park lever or is listed twice", l))
		}
		seen[l] = true
	}
	if len(c.Goals) == 0 {
		bad = append(bad, "challenge needs at least one goal")
	}
	for i, g := range c.Goals {
		switch {
		case !validMetrics[g.Metric]:
			bad = append(bad, fmt.Sprintf("goal %d: unknown metric %q", i+1, g.Metric))
		case (g.AtLeast == nil) == (g.AtMost == nil):
			bad = append(bad, fmt.Sprintf("goal %d (%s): set exactly one of at_least and at_most", i+1, g.Metric))
		case NodeMetric(g.Metric) != (g.Node != ""):
			bad = append(bad, fmt.Sprintf("goal %d (%s): node is required for posted_* metrics and only for them", i+1, g.Metric))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return false, strings.Join(bad, "; ")
	}
	seed := "scenario seed"
	if c.DailySeed {
		seed = "daily seed"
	}
	return true, fmt.Sprintf("%d goal(s), %d lever(s), %s", len(c.Goals), len(c.Levers), seed)
}

// Scoring returns the loaded scoring file with the given id, or nil.
func (d *Dataset) Scoring(id string) *File {
	for _, f := range d.Files {
		if f.Kind == KindScoring && f.Scoring.Scoring.ID == id {
			return f
		}
	}
	return nil
}

// Challenge returns the loaded challenge file with the given id, or nil.
func (d *Dataset) Challenge(id string) *File {
	for _, f := range d.Files {
		if f.Kind == KindChallenge && f.Challenge.Challenge.ID == id {
			return f
		}
	}
	return nil
}

// checkChallengeRefs is the cross-file check: the scenario is a park scenario, the scoring exists, and
// each goal node is a node of that park that posts waits.
func checkChallengeRefs(ds *Dataset, f *ChallengeFile) Result {
	c := f.Challenge
	r := Result{CheckID: "challenge.refs", Severity: SevError, Status: StatusFail}
	sf := ds.Scenario(c.Scenario)
	if sf == nil || !sf.Scenario.Scenario.IsPark() {
		r.Detail = fmt.Sprintf("scenario %q is not a loaded park scenario", c.Scenario)
		return r
	}
	if ds.Scoring(c.Scoring) == nil {
		r.Detail = fmt.Sprintf("scoring %q not found", c.Scoring)
		return r
	}
	var park *ParkFile
	for _, pf := range ds.Files {
		if pf.Kind == KindPark && pf.Park.Park.ID == sf.Scenario.Scenario.ParkID {
			park = pf.Park
		}
	}
	if park == nil {
		r.Detail = fmt.Sprintf("park %q of scenario %s not found", sf.Scenario.Scenario.ParkID, c.Scenario)
		return r
	}
	kinds := map[string]string{}
	for _, n := range park.Nodes {
		kinds[n.ID] = n.Kind
	}
	for _, g := range c.Goals {
		if g.Node == "" {
			continue
		}
		if k := kinds[g.Node]; k != NodeRide && k != NodeRideSimple && k != NodeMeet {
			r.Detail = fmt.Sprintf("goal %s: %q is not a ride or meet in park %s", g.Metric, g.Node, park.Park.ID)
			return r
		}
	}
	r.Status, r.Detail = StatusPass, fmt.Sprintf("park scenario %s, scoring %s", c.Scenario, c.Scoring)
	return r
}
