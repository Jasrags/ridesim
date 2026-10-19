package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jasrags/ridesim/internal/challenge"
	"github.com/jasrags/ridesim/internal/config"
)

// defaultBoard is where verified plays are recorded (out/ is not committed).
const defaultBoard = "out/leaderboard.json"

func runChallenge(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("challenge", flag.ContinueOnError)
	dataDir := fs.String("data", "data", "data directory")
	file := fs.String("file", "", "challenge file (required), e.g. data/challenges/dhs-rise-late.yaml")
	date := fs.String("date", "", "play date YYYY-MM-DD for daily-seed challenges (default today)")
	sessionPath := fs.String("session", "", "played session (the control room's export) to verify and enter")
	name := fs.String("name", "", "player name for the leaderboard (with -session)")
	boardPath := fs.String("board", defaultBoard, "local leaderboard file")
	logPath := fs.String("log", "", "write the scored day's JSONL event log here")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("challenge: -file is required")
	}
	ds, err := loadValid(*dataDir)
	if err != nil {
		return err
	}
	c, err := findChallenge(ds, *file)
	if err != nil {
		return err
	}
	board, err := challenge.LoadBoard(*boardPath)
	if err != nil {
		return err
	}
	if *sessionPath == "" {
		seed, err := c.Seed(playDate(*date))
		if err != nil {
			return err
		}
		out, err := c.Run(ds, seed)
		if err != nil {
			return err
		}
		if err := writeOutcome(stdout, c, out, "baseline, no levers", *logPath); err != nil {
			return err
		}
		return board.WriteTop(stdout, c.ID, seed)
	}
	return enterSession(stdout, ds, c, board, *sessionPath, *name, *boardPath, *logPath)
}

// enterSession verifies a played session, scores the rerun and records it on the leaderboard.
func enterSession(stdout io.Writer, ds *config.Dataset, c *challenge.Challenge, board *challenge.Board, path, name, boardPath, logPath string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("challenge: -name is required with -session")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read session: %w", err)
	}
	f, err := config.LoadBytes(path, raw)
	if err != nil {
		return err
	}
	if f.Kind != config.KindScenario {
		return fmt.Errorf("%s is not a scenario file (export one from the control room)", path)
	}
	seed, err := sessionSeed(c, f.Scenario.Scenario.Seed)
	if err != nil {
		return err
	}
	out, err := c.Verify(ds, f.Scenario, seed)
	if err != nil {
		return err
	}
	if err := writeOutcome(stdout, c, out, fmt.Sprintf("verified session, %d lever(s)", out.Levers), logPath); err != nil {
		return err
	}
	e := challenge.Entry{Challenge: c.ID, Seed: seed, Name: name, Score: out.Card.Score(), GoalsMet: out.Met(),
		Goals: len(out.Goals), Levers: out.Levers, Session: challenge.SessionHash(raw)}
	if err := board.Add(e); err != nil {
		return err
	}
	if err := board.Save(boardPath); err != nil {
		return err
	}
	return board.WriteTop(stdout, c.ID, seed)
}

// sessionSeed is the seed a session must use: the scenario's, or for a daily challenge the session's own
// seed, which must be a date.
func sessionSeed(c *challenge.Challenge, seed uint64) (uint64, error) {
	if !c.DailySeed {
		return c.Seed("")
	}
	s := strconv.FormatUint(seed, 10)
	if len(s) != 8 {
		return 0, fmt.Errorf("session seed %d is not a play date (YYYYMMDD)", seed)
	}
	return challenge.DailySeed(s[:4] + "-" + s[4:6] + "-" + s[6:])
}

// playDate defaults to today; the wall clock only picks the date, never anything in the sim.
func playDate(date string) string {
	if date != "" {
		return date
	}
	return time.Now().Format("2006-01-02")
}

func loadValid(dir string) (*config.Dataset, error) {
	ds, err := config.LoadDir(dir)
	if err != nil {
		return nil, err
	}
	if config.Validate(ds).HasErrors() {
		return nil, errors.New("data has validation errors; run ridesim validate")
	}
	return ds, nil
}

func findChallenge(ds *config.Dataset, path string) (*challenge.Challenge, error) {
	for _, f := range ds.Files {
		if f.Kind == config.KindChallenge && filepath.Clean(f.Path) == filepath.Clean(path) {
			return challenge.FromFile(ds, f.Challenge)
		}
	}
	return nil, fmt.Errorf("%s is not a challenge under the data directory", path)
}

func writeOutcome(w io.Writer, c *challenge.Challenge, out *challenge.Outcome, what, logPath string) error {
	if logPath != "" {
		if err := writeLog(logPath, out.Log); err != nil {
			return err
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "challenge %s: %s\nscenario %s, seed %d (%s)\n\n%s\ngoals %d/%d met\n", c.ID, c.Title, c.Scenario.Scenario.ID,
		out.Seed, what, out.Card, out.Met(), len(out.Goals))
	for _, g := range out.Goals {
		mark := "miss"
		if g.Met {
			mark = "MET "
		}
		fmt.Fprintf(&b, "  %s  %-40s %8.2f\n", mark, g.Label(), g.Value)
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}
