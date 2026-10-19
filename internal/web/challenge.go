package web

import (
	"fmt"

	"github.com/jasrags/ridesim/internal/challenge"
	"github.com/jasrags/ridesim/internal/metrics"
	"github.com/jasrags/ridesim/internal/score"
)

// challengeJSON is the challenge panel: the brief, the running score and the goals. Posted-wait goals are
// pending until the day ends, when the park report is computed once.
type challengeJSON struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Seed        uint64     `json:"seed"`
	Daily       bool       `json:"daily"`
	Levers      []string   `json:"levers"`
	Score       scoreJSON  `json:"score"`
	Goals       []goalJSON `json:"goals"`
	Verify      string     `json:"verify"` // the command that enters an exported session on the leaderboard
}

// scoreJSON is the card per guest; penalties are positive.
type scoreJSON struct {
	Guests      int     `json:"guests"`
	Score       float64 `json:"score"`
	Experiences float64 `json:"experiences_per_guest"`
	Experience  float64 `json:"experience"`
	Wait        float64 `json:"wait"`
	Balk        float64 `json:"balk"`
	Renege      float64 `json:"renege"`
	Walk        float64 `json:"walk"`
	Jam         float64 `json:"jam"`
	Lapse       float64 `json:"lapse"`
	Balks       int     `json:"balks"`
	Reneges     int     `json:"reneges"`
	LLLapsed    int     `json:"ll_lapsed"`
	LLRedeemed  int     `json:"ll_redeemed"`
}

type goalJSON struct {
	Label   string  `json:"label"`
	Value   float64 `json:"value"`
	Met     bool    `json:"met"`
	Pending bool    `json:"pending"`
}

// challengeState is the server's challenge: the definition and the cached end-of-day report.
type challengeState struct {
	c      *challenge.Challenge
	file   string
	report *metrics.ParkReport // computed once the day ends; cleared when play resumes
}

func newScoreJSON(c score.Card) scoreJSON {
	per := func(v float64) float64 {
		if c.Guests == 0 {
			return 0
		}
		return v / float64(c.Guests)
	}
	return scoreJSON{Guests: c.Guests, Score: c.Score(), Experiences: c.ExperiencesPerGuest(), Experience: per(c.Experience),
		Wait: per(c.WaitPenalty), Balk: per(c.BalkPenalty), Renege: per(c.RenegePenalty), Walk: per(c.WalkPenalty),
		Jam: per(c.JamPenalty), Lapse: per(c.LapsePenalty), Balks: c.Balks, Reneges: c.Reneges, LLLapsed: c.LLLapsed, LLRedeemed: c.LLRedeemed}
}

// challengeLocked builds the panel; the caller holds s.mu.
func (s *Server) challengeLocked() *challengeJSON {
	cs := s.ch
	if cs == nil {
		return nil
	}
	if !s.sess.Done() {
		cs.report = nil
	} else if cs.report == nil {
		pr, err := metrics.ComputePark(s.sess.Log().Events(), s.sess.Warmup())
		if err == nil {
			cs.report = pr
		}
	}
	card, _ := s.sess.Card()
	out := &challengeJSON{ID: cs.c.ID, Title: cs.c.Title, Description: cs.c.Description, Seed: s.sess.Seed(), Daily: cs.c.DailySeed,
		Levers: append([]string{}, cs.c.Levers...), Score: newScoreJSON(card),
		Verify: fmt.Sprintf("ridesim challenge -file %s -session %s.yaml -name YOUR_NAME", cs.file, s.id)}
	for _, g := range cs.c.Evaluate(card, cs.report) {
		out.Goals = append(out.Goals, goalJSON{Label: g.Label(), Value: g.Value, Met: g.Met, Pending: g.Pending})
	}
	return out
}
