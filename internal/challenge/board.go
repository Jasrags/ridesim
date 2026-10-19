package challenge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one verified play on the local leaderboard.
type Entry struct {
	Challenge string  `json:"challenge"`
	Seed      uint64  `json:"seed"`
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	GoalsMet  int     `json:"goals_met"`
	Goals     int     `json:"goals"`
	Levers    int     `json:"levers"`
	Session   string  `json:"session_sha256"` // the submitted session file, so one play is entered once
}

// Board is the local leaderboard: verified entries in a JSON file.
type Board struct {
	Entries []Entry `json:"entries"`
}

// maxNameLen keeps names short enough to print in a table.
const maxNameLen = 32

// SessionHash identifies a session file's bytes.
func SessionHash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// LoadBoard reads a leaderboard file; a missing file is an empty board.
func LoadBoard(path string) (*Board, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Board{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read leaderboard: %w", err)
	}
	var bd Board
	if err := json.Unmarshal(b, &bd); err != nil {
		return nil, fmt.Errorf("leaderboard %s: %w", path, err)
	}
	return &bd, nil
}

// Save writes the board, creating its directory.
func (b *Board) Save(path string) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("leaderboard directory: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Add records a verified entry; the same session file cannot be entered twice.
func (b *Board) Add(e Entry) error {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || len(e.Name) > maxNameLen || strings.ContainsAny(e.Name, "\n\r\t") {
		return fmt.Errorf("name must be 1-%d characters on one line", maxNameLen)
	}
	for _, x := range b.Entries {
		if x.Session == e.Session {
			return fmt.Errorf("this session is already on the board (%s, %.2f)", x.Name, x.Score)
		}
	}
	b.Entries = append(b.Entries, e)
	return nil
}

// Top is the entries for one challenge and seed, best score first (ties: more goals met, fewer levers, name).
func (b *Board) Top(challenge string, seed uint64) []Entry {
	var out []Entry
	for _, e := range b.Entries {
		if e.Challenge == challenge && e.Seed == seed {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i], out[j]
		switch {
		case a.Score != c.Score:
			return a.Score > c.Score
		case a.GoalsMet != c.GoalsMet:
			return a.GoalsMet > c.GoalsMet
		case a.Levers != c.Levers:
			return a.Levers < c.Levers
		}
		return a.Name < c.Name
	})
	return out
}

// WriteTop renders the ranking for a challenge and seed.
func (b *Board) WriteTop(w io.Writer, challenge string, seed uint64) error {
	top := b.Top(challenge, seed)
	var s strings.Builder
	fmt.Fprintf(&s, "leaderboard %s, seed %d\n", challenge, seed)
	if len(top) == 0 {
		s.WriteString("  no entries yet\n")
	}
	for i, e := range top {
		fmt.Fprintf(&s, "  %2d. %-*s %8.2f   goals %d/%d   levers %d\n", i+1, maxNameLen, e.Name, e.Score, e.GoalsMet, e.Goals, e.Levers)
	}
	_, err := io.WriteString(w, s.String())
	return err
}
