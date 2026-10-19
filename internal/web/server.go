// Package web serves the M8 control room: a page that draws the park and sends levers, over a live
// session. The server paces the sim with a real-time ticker (outside simulation logic; the sim itself stays
// single-threaded under one mutex) and pushes snapshots to every open page over server-sent events.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jasrags/ridesim/internal/challenge"
	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/live"
)

//go:embed static
var static embed.FS

// Speeds the page offers: sim seconds per real second (0 = paused).
var Speeds = []float64{0, 1, 10, 60, 600, 3600}

const maxBody = 4 << 10 // request bodies are small JSON objects

// Server owns one session.
type Server struct {
	mu    sync.Mutex
	sess  *live.Session
	speed float64
	id    string          // export id
	ch    *challengeState // nil outside challenge play

	subMu sync.Mutex
	subs  map[chan []byte]struct{}
}

// New wraps a session; exportID names the exported scenario.
func New(sess *live.Session, exportID string) *Server {
	return &Server{sess: sess, speed: 10, id: exportID, subs: map[chan []byte]struct{}{}}
}

// NewChallenge serves a challenge's day: only its levers are accepted and the page shows the running score
// and goals. file is the challenge file's path, quoted in the leaderboard command. The session must be the
// challenge's day.
func NewChallenge(sess *live.Session, c *challenge.Challenge, file string) *Server {
	s := New(sess, c.ID+"_played")
	sess.EnableScore(c.Scoring)
	s.ch = &challengeState{c: c, file: file}
	return s
}

// Handler routes the page and the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	page, _ := fs.Sub(static, "static")
	mux.Handle("GET /", http.FileServer(http.FS(page)))
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/speed", s.handleSpeed)
	mux.HandleFunc("POST /api/lever", s.handleLever)
	mux.HandleFunc("POST /api/jump", s.handleJump)
	mux.HandleFunc("GET /api/export", s.handleExport)
	return secure(mux)
}

// secure sets defensive headers. The page is same-origin only, apart from Google Fonts.
func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// Tick advances the session by real × speed and broadcasts a snapshot. Run calls it on a ticker; tests call
// it directly.
func (s *Server) Tick(real time.Duration) error {
	s.mu.Lock()
	err := s.sess.Advance(engine.Duration(float64(real.Milliseconds()) * s.speed))
	msg := s.snapshotLocked()
	s.mu.Unlock()
	s.broadcast(msg)
	return err
}

// Run ticks every interval until stop is closed.
func (s *Server) Run(interval time.Duration, stop <-chan struct{}) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-t.C:
			if err := s.Tick(interval); err != nil {
				return err
			}
		}
	}
}

type stateJSON struct {
	live.Snapshot
	Speed     float64        `json:"speed"`
	Speeds    []float64      `json:"speeds"`
	Challenge *challengeJSON `json:"challenge,omitempty"`
}

func (s *Server) snapshotLocked() []byte {
	b, err := json.Marshal(stateJSON{Snapshot: s.sess.Snapshot(), Speed: s.speed, Speeds: Speeds, Challenge: s.challengeLocked()})
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	b := s.snapshotLocked()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// handleEvents streams snapshots: one on connect, then one per tick.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := make(chan []byte, 4)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()
	defer func() {
		s.subMu.Lock()
		delete(s.subs, ch)
		s.subMu.Unlock()
	}()
	s.mu.Lock()
	first := s.snapshotLocked()
	s.mu.Unlock()
	send := func(b []byte) bool {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	if !send(first) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			if !send(b) {
				return
			}
		}
	}
}

// broadcast offers msg to every subscriber, dropping it for one that is behind (the next tick catches up).
func (s *Server) broadcast(msg []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subs { // delivery order does not reach the sim
		select {
		case ch <- msg:
		default:
		}
	}
}

// errNotJSON rejects bodies that are not application/json: a cross-site page cannot send that type without
// a CORS preflight this server never grants, so other sites cannot pull levers on a local session.
var errNotJSON = errors.New("Content-Type must be application/json")

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); ct != "application/json" && !strings.HasPrefix(ct, "application/json;") {
		return errNotJSON
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("bad request body: %w", err)
	}
	return nil
}

func fail(w http.ResponseWriter, code int, err error) {
	if errors.Is(err, errNotJSON) {
		code = http.StatusUnsupportedMediaType
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func (s *Server) handleSpeed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Speed float64 `json:"speed"`
	}
	if err := readJSON(w, r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	ok := false
	for _, sp := range Speeds {
		ok = ok || sp == req.Speed
	}
	if !ok {
		fail(w, http.StatusBadRequest, fmt.Errorf("speed %g is not one of %v", req.Speed, Speeds))
		return
	}
	s.mu.Lock()
	s.speed = req.Speed
	s.mu.Unlock()
	s.handleState(w, r)
}

func (s *Server) handleLever(w http.ResponseWriter, r *http.Request) {
	var op config.OpsEvent
	var req struct {
		Action  string   `json:"action"`
		Node    string   `json:"node"`
		Minutes float64  `json:"minutes"`
		Value   *float64 `json:"value"`
	}
	if err := readJSON(w, r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	op = config.OpsEvent{Action: req.Action, Node: req.Node, Minutes: req.Minutes, Value: req.Value}
	if s.ch != nil && !s.ch.c.Allows(op.Action) {
		fail(w, http.StatusForbidden, fmt.Errorf("%s is not allowed in the %q challenge", op.Action, s.ch.c.Title))
		return
	}
	s.mu.Lock()
	_, err := s.sess.Apply(op)
	msg := s.snapshotLocked()
	s.mu.Unlock()
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	s.broadcast(msg)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(msg)
}

func (s *Server) handleJump(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TS *float64 `json:"t_s"`
	}
	if err := readJSON(w, r, &req); err != nil || req.TS == nil {
		fail(w, http.StatusBadRequest, errors.New(`body must be {"t_s": seconds after opening}`))
		return
	}
	s.mu.Lock()
	err := s.sess.JumpTo(engine.Time(engine.Seconds(*req.TS)))
	msg := s.snapshotLocked()
	s.mu.Unlock()
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	s.broadcast(msg)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(msg)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	b := s.sess.Export(s.id)
	if s.ch != nil {
		b = append([]byte(fmt.Sprintf("# Challenge %s (%s), seed %d. Enter it on the leaderboard with:\n#   ridesim challenge -file %s -session %s.yaml -name YOUR_NAME\n",
			s.ch.c.ID, s.ch.c.Title, s.sess.Seed(), s.ch.file, s.id)), b...)
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", s.id+".yaml"))
	_, _ = w.Write(b)
}
