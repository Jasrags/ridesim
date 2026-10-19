package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jasrags/ridesim/internal/challenge"
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/live"
)

func newChallengeServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	c, err := challenge.Load(ds, "fx_challenge")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := live.New(ds, c.Day(1))
	if err != nil {
		t.Fatal(err)
	}
	s := NewChallenge(sess, c, "testdata/test-fx-challenge.yaml")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

// In challenge play only the challenge's levers are accepted, the state carries the running score and
// goals, posted-wait goals resolve when the day ends, and the export says how to verify it.
func TestChallengePlay(t *testing.T) {
	s, ts := newChallengeServer(t)
	if code, _ := post(t, ts.URL+"/api/speed", `{"speed":3600}`); code != 200 {
		t.Fatalf("speed: %d", code)
	}
	if err := s.Tick(time.Second); err != nil {
		t.Fatal(err)
	}
	if code, _ := post(t, ts.URL+"/api/lever", `{"action":"close_ride","node":"coaster","minutes":30}`); code != http.StatusForbidden {
		t.Fatalf("close_ride in a challenge that does not allow it: %d", code)
	}
	code, st := post(t, ts.URL+"/api/lever", `{"action":"ll_merge","node":"coaster","value":1}`)
	if code != 200 {
		t.Fatalf("ll_merge: %d %v", code, st)
	}
	ch := st["challenge"].(map[string]any)
	goals := ch["goals"].([]any)
	if ch["score"].(map[string]any)["guests"].(float64) == 0 || !goals[1].(map[string]any)["pending"].(bool) {
		t.Fatalf("challenge state mid-day %v", ch)
	}
	for i := 0; i < 12; i++ {
		if err := s.Tick(time.Second); err != nil {
			t.Fatal(err)
		}
	}
	_, st = post(t, ts.URL+"/api/speed", `{"speed":0}`)
	if !st["done"].(bool) {
		t.Fatal("day not over")
	}
	if g := st["challenge"].(map[string]any)["goals"].([]any)[1].(map[string]any); g["pending"].(bool) {
		t.Fatalf("posted goal still pending at day end: %v", g)
	}
	resp, err := http.Get(ts.URL + "/api/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(b), "# Challenge fx_challenge") || !strings.Contains(string(b), "ll_merge") {
		t.Fatalf("export:\n%s", b)
	}
}
