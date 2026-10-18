// Package lines is a ride's front of house, shared by every archetype (spec §8): arrival, balking,
// outdoor standby / single-rider / Lightning Lane lines with reneging, the LL merge, an optional preshow
// batch stage, and the load-area line the ride boards from. How guests are seated (train rows, doom
// buggies) stays in the ride.
//
// Merge point: with a preshow the merge admits parties into the preshow (Rock 'n' Roller hallway,
// Haunted Mansion foyer); without one the merge sits on the load line (Everest station entrance).
// After a preshow the load-area line is one combined line ([user], Rock 'n' Roller): release order is
// kept, kept as two queues ordered by LineSeq so pulling the earliest single rider stays O(1).
package lines

import (
	"fmt"
	"math"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/queue"
)

// Ride is what the lines need from the ride that owns them.
type Ride interface {
	ID() string
	PostedWait() engine.Duration          // the sign guests balk against
	Board()                               // the load-area line changed: seat whoever can be seated
	Sample(d config.Dist) engine.Duration // preshow timings
}

type record struct {
	at engine.Time
	n  int
}

// Lines holds every queue in front of a ride's load point.
type Lines struct {
	sim     *engine.Sim
	ride    Ride
	queues  *config.Queues
	preshow *config.Preshow

	standby, single         *queue.FIFO // outdoor lines (reneging allowed)
	loadStandby, loadSingle *queue.FIFO // load-area line; the outdoor lines themselves without a preshow
	ll                      *queue.FIFO // Lightning Lane up to the merge point (nil without skip_line)
	merge                   *queue.Merge
	stage                   *queue.Preshow // the preshow: first batch stage, fed by the outdoor lines

	// Further batch stages in guest order (M11, Rise of the Resistance). buffers[i] holds guests released
	// by stage i (stages[0] is the preshow) waiting for stage i+1; the last stage releases into the load line.
	stages   []*queue.Preshow
	stageCfg []*config.Preshow
	buffers  []*queue.FIFO

	llInside   int   // LL guests past the merge not yet boarded
	releaseSeq int64 // release order into the combined post-preshow line
	srClosed   bool

	standbyBoards []record // non-LL guests seated, for the standby posted wait
}

// New builds the lines for a ride's queues, optional preshow and optional further stages (which need the
// preshow).
func New(sim *engine.Sim, ride Ride, queues *config.Queues, preshow *config.Preshow, more []config.Preshow) *Lines {
	l := &Lines{sim: sim, ride: ride, queues: queues, standby: &queue.FIFO{}, single: &queue.FIFO{}}
	l.loadStandby, l.loadSingle = l.standby, l.single
	if sl := queues.SkipLine; sl.Enabled && len(sl.Merge.Phases) > 0 {
		l.ll = &queue.FIFO{}
		l.merge = queue.NewMerge(sim, sl.Merge)
	}
	if preshow != nil && preshow.Enabled && preshow.Rooms > 0 {
		l.preshow = preshow
		l.loadStandby, l.loadSingle = &queue.FIFO{}, &queue.FIFO{}
		l.stage = queue.NewPreshow(sim, preshow, stageHost{l})
		l.stages, l.stageCfg = []*queue.Preshow{l.stage}, []*config.Preshow{preshow}
		for i := range more {
			l.buffers = append(l.buffers, &queue.FIFO{})
			l.stageCfg = append(l.stageCfg, &more[i])
			l.stages = append(l.stages, queue.NewPreshow(sim, &more[i], laterStage{l, i + 1}))
		}
	}
	return l
}

// Start opens the preshow, if any.
func (l *Lines) Start() {
	for _, s := range l.stages {
		s.Start()
	}
}

// HasPreshow reports whether guests pass a preshow before the load area.
func (l *Lines) HasPreshow() bool { return l.stage != nil }

// Arrive admits a party: Lightning Lane joins its line (no balking or reneging, it holds a return time);
// anyone else may balk on the posted wait, then joins standby or single rider and may renege.
func (l *Lines) Arrive(p *guest.Party, lane guest.Lane) {
	switch lane {
	case guest.LightningLane:
		if l.merge == nil {
			panic(fmt.Sprintf("%s: Lightning Lane arrival but skip_line is not enabled", l.ride.ID()))
		}
		p.Lane, p.JoinedAt = lane, l.sim.Now()
		l.sim.Emit(engine.Event{Type: engine.EvQueueJoin, Guest: p.ID, N: p.Size, Info: lane.String()})
		l.ll.Push(p)
		l.merge.Update(l.ll.Guests())
		l.Flow()
		return
	case guest.SingleRider:
		if !l.SingleRiderOpen() {
			lane = guest.Standby // no open single-rider line: they queue standby as a party of 1
		}
	}
	p.Lane = lane
	if p.Patience > 0 && l.ride.PostedWait() > p.Patience {
		l.sim.Emit(engine.Event{Type: engine.EvBalk, Guest: p.ID, N: p.Size, Info: lane.String()})
		p.Exited(guest.Balked)
		return
	}
	p.JoinedAt = l.sim.Now()
	l.sim.Emit(engine.Event{Type: engine.EvQueueJoin, Guest: p.ID, N: p.Size, Info: lane.String()})
	line := l.standby
	if lane == guest.SingleRider {
		line = l.single
	}
	line.Push(p)
	if p.Patience > 0 {
		l.sim.At(p.JoinedAt.Add(p.Patience), func() {
			if p.Committed || p.Gone {
				return
			}
			line.Remove(p)
			l.sim.Emit(engine.Event{Type: engine.EvRenege, Guest: p.ID, N: p.Size, Info: lane.String()})
			p.Exited(guest.Reneged)
		})
	}
	l.Flow()
}

// Flow moves guests forward after any queue change.
func (l *Lines) Flow() {
	for _, s := range l.stages {
		s.Pull()
	}
	l.ride.Board()
}

// SingleRiderOpen reports whether the single-rider line takes new guests ([user]: operations open and
// close it; criteria unknown).
func (l *Lines) SingleRiderOpen() bool { return l.queues.SingleRider.Enabled && !l.srClosed }

// SetSingleRiderOpen opens or closes the single-rider line. Closing keeps the guests already in it.
func (l *Lines) SetSingleRiderOpen(open bool) {
	if l.srClosed == !open {
		return
	}
	l.srClosed = !open
	state := "closed"
	if open {
		state = "open"
	}
	l.sim.Emit(engine.Event{Type: engine.EvSingleRider, Info: state})
}

// Head is the next party in line to board and the queue holding it.
func (l *Lines) Head() (*guest.Party, *queue.FIFO) {
	s := l.loadStandby.Peek()
	if l.stage == nil {
		if l.merge != nil && l.merge.Choose(l.ll.Peek(), s) {
			return l.ll.Peek(), l.ll
		}
		return s, l.loadStandby
	}
	if sr := l.loadSingle.Peek(); sr != nil && (s == nil || sr.LineSeq < s.LineSeq) {
		return sr, l.loadSingle
	}
	return s, l.loadStandby
}

// Pop removes the head of line (from Head); at a load-line merge it also feeds the merge.
func (l *Lines) Pop(line *queue.FIFO) *guest.Party {
	llWaiting := l.ll != nil && l.ll.Peek() != nil
	p := line.Pop()
	if p != nil && l.merge != nil && l.stage == nil && line != l.loadSingle {
		l.merge.Took(p, llWaiting)
		l.merge.Update(l.ll.Guests())
	}
	return p
}

// CallOut takes the first party of at most max guests among the next window standby-side parties in the
// load line, or nil: cast calling into the line for a party that fills the spare seats ([user] Runaway
// Railway). Single riders are not counted here; PopSingle takes them.
func (l *Lines) CallOut(max, window int) *guest.Party {
	llWaiting := l.ll != nil && l.ll.Peek() != nil
	p := l.loadStandby.TakeFirst(window, func(p *guest.Party) bool { return p.Size <= max })
	if p != nil && l.merge != nil && l.stage == nil {
		l.merge.Took(p, llWaiting)
		l.merge.Update(l.ll.Guests())
	}
	return p
}

// PeekSingle and PopSingle give the earliest single rider anywhere in the load line (the grouper's pull).
func (l *Lines) PeekSingle() *guest.Party { return l.loadSingle.Peek() }
func (l *Lines) PopSingle() *guest.Party  { return l.loadSingle.Pop() }

// Seated records that p boarded; the ride emits the board event.
func (l *Lines) Seated(p *guest.Party) {
	p.Committed = true
	if p.Lane == guest.LightningLane {
		if l.stage != nil {
			l.llInside -= p.Size
		}
		return
	}
	now := l.sim.Now()
	if k := len(l.standbyBoards); k > 0 && l.standbyBoards[k-1].at == now {
		l.standbyBoards[k-1].n += p.Size
		return
	}
	l.standbyBoards = append(l.standbyBoards, record{now, p.Size})
	if len(l.standbyBoards) > 1024 {
		l.standbyBoards = append([]record(nil), l.standbyBoards[512:]...)
	}
}

// Boarded is called after a boarding pass seated anyone: load-area space freed.
func (l *Lines) Boarded() {
	if n := len(l.stages); n > 0 {
		l.stages[n-1].Drain()
	}
}

// StandbyBoardedSince is the number of non-LL guests seated after t.
func (l *Lines) StandbyBoardedSince(t engine.Time) int {
	n := 0
	for _, r := range l.standbyBoards {
		if r.at > t {
			n += r.n
		}
	}
	return n
}

// Waiting is every guest who joined and has not boarded or reneged.
func (l *Lines) Waiting() int {
	n := l.standby.Guests() + l.single.Guests()
	if l.ll != nil {
		n += l.ll.Guests()
	}
	if l.stage != nil {
		n += l.loadStandby.Guests() + l.loadSingle.Guests()
	}
	for _, s := range l.stages {
		n += s.Guests()
	}
	for _, b := range l.buffers {
		n += b.Guests()
	}
	return n
}

// StandbyWaiting is Waiting minus Lightning Lane guests, wherever they are.
func (l *Lines) StandbyWaiting() int {
	n := l.Waiting()
	if l.ll != nil {
		n -= l.ll.Guests() + l.llInside
	}
	return n
}

// OutdoorSingle is the number of guests in the outdoor single-rider line (tests and metrics).
func (l *Lines) OutdoorSingle() int { return l.single.Guests() }

// stageHost adapts Lines to queue.PreshowHost.
type stageHost struct{ l *Lines }

// standbySide is the earliest-joined head of the standby and single-rider lines.
func (h stageHost) standbySide() *guest.Party {
	a, b := h.l.standby.Peek(), h.l.single.Peek()
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.JoinedAt < a.JoinedAt || (b.JoinedAt == a.JoinedAt && b.ID < a.ID):
		return b
	}
	return a
}

func (h stageHost) NextParty() *guest.Party {
	s := h.standbySide()
	if h.l.merge != nil && h.l.merge.Choose(h.l.ll.Peek(), s) {
		return h.l.ll.Peek()
	}
	return s
}

func (h stageHost) TakeParty() *guest.Party {
	next := h.NextParty()
	if next == nil {
		return nil
	}
	llWaiting := h.l.ll != nil && h.l.ll.Peek() != nil
	var p *guest.Party
	switch next.Lane {
	case guest.LightningLane:
		p = h.l.ll.Pop()
		h.l.llInside += p.Size
	case guest.SingleRider:
		p = h.l.single.Pop()
	default:
		p = h.l.standby.Pop()
	}
	if h.l.merge != nil {
		h.l.merge.Took(p, llWaiting)
		h.l.merge.Update(h.l.ll.Guests())
	}
	return p
}

func (h stageHost) LoadAreaFree() int {
	if len(h.l.buffers) > 0 {
		return free(h.l.preshow.LoadAreaCapacity, h.l.buffers[0].Guests())
	}
	return h.l.loadLineFree(h.l.preshow)
}

func free(limit, used int) int {
	if limit <= 0 {
		return math.MaxInt
	}
	return limit - used
}

func (l *Lines) loadLineFree(last *config.Preshow) int {
	return free(last.LoadAreaCapacity, l.loadStandby.Guests()+l.loadSingle.Guests())
}

func (h stageHost) Release(p *guest.Party) {
	if len(h.l.buffers) > 0 {
		h.l.buffers[0].Push(p)
		h.l.Flow()
		return
	}
	h.l.releaseToLoad(p)
}

// releaseToLoad moves a party from the last stage into the load-area line.
func (l *Lines) releaseToLoad(p *guest.Party) {
	l.releaseSeq++
	p.LineSeq = l.releaseSeq
	if p.Lane == guest.SingleRider {
		l.loadSingle.Push(p)
	} else {
		l.loadStandby.Push(p)
	}
	l.Flow()
}

func (h stageHost) Sample(d config.Dist) engine.Duration { return h.l.ride.Sample(d) }

// laterStage adapts Lines to queue.PreshowHost for stage i >= 1, fed in release order by buffers[i-1].
type laterStage struct {
	l *Lines
	i int
}

func (h laterStage) last() bool { return h.i == len(h.l.stages)-1 }

func (h laterStage) NextParty() *guest.Party { return h.l.buffers[h.i-1].Peek() }

// TakeParty frees space in the buffer, so the stage before may release more guests into it.
func (h laterStage) TakeParty() *guest.Party {
	p := h.l.buffers[h.i-1].Pop()
	h.l.stages[h.i-1].Drain()
	return p
}

func (h laterStage) LoadAreaFree() int {
	if h.last() {
		return h.l.loadLineFree(h.l.stageCfg[h.i])
	}
	return free(h.l.stageCfg[h.i].LoadAreaCapacity, h.l.buffers[h.i].Guests())
}

func (h laterStage) Release(p *guest.Party) {
	if h.last() {
		h.l.releaseToLoad(p)
		return
	}
	h.l.buffers[h.i].Push(p)
	h.l.Flow()
}

func (h laterStage) Sample(d config.Dist) engine.Duration { return h.l.ride.Sample(d) }
