package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/live"
)

func newServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	ds, err := config.LoadDir("../config/testdata")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := live.New(ds, ds.Scenario("fx_park_day").Scenario)
	if err != nil {
		t.Fatal(err)
	}
	s := New(sess, "fx_played")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func post(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSpeedTicksAndLevers(t *testing.T) {
	s, ts := newServer(t)
	if code, _ := post(t, ts.URL+"/api/speed", `{"speed":600}`); code != 200 {
		t.Fatalf("speed: %d", code)
	}
	if code, _ := post(t, ts.URL+"/api/speed", `{"speed":7}`); code != 400 {
		t.Fatalf("odd speed accepted: %d", code)
	}
	if err := s.Tick(6 * time.Second); err != nil { // 600x: one hour
		t.Fatal(err)
	}
	code, st := post(t, ts.URL+"/api/lever", `{"action":"close_ride","node":"coaster","minutes":30}`)
	if code != 200 || st["t"].(float64) < 3600000 {
		t.Fatalf("lever: %d %v", code, st["t"])
	}
	closed := false
	for _, r := range st["rides"].([]any) {
		m := r.(map[string]any)
		closed = closed || (m["node"] == "coaster" && m["status"] == "closed")
	}
	if !closed {
		t.Fatal("coaster not closed after the lever")
	}
	if code, st := post(t, ts.URL+"/api/lever", `{"action":"close_ride","node":"nowhere","minutes":30}`); code != 422 || st["error"] == "" {
		t.Fatalf("bad lever: %d %v", code, st)
	}
	if code, _ := post(t, ts.URL+"/api/lever", `{"action":"close_ride","bogus":1}`); code != 400 {
		t.Fatalf("unknown field accepted: %d", code)
	}
	if code, st := post(t, ts.URL+"/api/jump", `{"t_s":1800}`); code != 200 || st["t"].(float64) != 1800000 {
		t.Fatalf("jump: %d %v", code, st["t"])
	}
	resp, err := http.Get(ts.URL + "/api/export")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "id: fx_played") || strings.Contains(string(body), "nowhere") {
		t.Fatalf("export:\n%s", body)
	}
	if strings.Contains(string(body), `minutes: 30`) {
		t.Fatalf("the jump back to 00:30 should have discarded the 01:00 closure:\n%s", body)
	}
}

func TestEventStreamSendsSnapshots(t *testing.T) {
	s, ts := newServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	rd := bufio.NewReader(resp.Body)
	read := func() map[string]any {
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(line, "data: ") {
				var m map[string]any
				if err := json.Unmarshal([]byte(line[6:]), &m); err != nil {
					t.Fatal(err)
				}
				return m
			}
		}
	}
	first := read()
	if first["t"].(float64) != 0 {
		t.Fatalf("first snapshot at %v", first["t"])
	}
	for i := 0; i < 50; i++ { // the subscriber registers asynchronously; tick until a snapshot arrives
		_ = s.Tick(100 * time.Millisecond)
		s.subMu.Lock()
		n := len(s.subs)
		s.subMu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = s.Tick(time.Second)
	if next := read(); next["t"].(float64) <= 0 {
		t.Fatalf("no progress in the stream: %v", next["t"])
	}
}

func TestPageIsServed(t *testing.T) {
	_, ts := newServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("page: %v %v", err, resp)
	}
	resp.Body.Close()
}

// Levers only accept JSON bodies (blocks blind cross-site form posts) and responses carry defensive headers.
func TestLeversRequireJSONAndHeadersAreSet(t *testing.T) {
	_, ts := newServer(t)
	resp, err := http.Post(ts.URL+"/api/lever", "text/plain", strings.NewReader(`{"action":"crowd_scale","value":0}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain lever: %d, want 415", resp.StatusCode)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers %v", resp.Header)
	}
}
