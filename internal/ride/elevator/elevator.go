// Package elevator implements the elevator_drop archetype (M10): a drop-shaft ride with self-propelled
// cabs, such as Tower of Terror. The building has mirrored sides. On each side a cab:
//
//  1. drives from an unload door into a free show shaft, which lifts it one floor to load;
//  2. loads (parties take whole rows, as on a coaster), then rides the show shaft up through the show
//     scenes to the drop shaft door;
//  3. moves into the drop shaft when it is free, rides the drop sequence and comes down to the basement;
//  4. drives to a free unload door and unloads.
//
// Every shaft and door holds one cab. A cab that finishes its step and finds the next place taken waits
// where it is (hold_start / hold_end), still holding its current place; waiters are served first come,
// first served. Validation keeps the cabs per side below the number of places, so the cycle cannot
// lock. Cabs stay on their own side.
//
// Stops: a ride stop (brief stop, long downtime, weather, ops closure) pauses every cab where it is
// with its remaining time. A side_down outage pauses only that side's cabs; the other side keeps
// loading, so capacity halves.
package elevator

import (
	"fmt"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/ride"
	"github.com/jasrags/ridesim/internal/ride/lines"
)

// Stop causes recorded in ride_stop / ride_resume events.
const (
	CauseBriefStop    = config.FailBriefStop
	CauseLongDowntime = config.FailLongDowntime
	CauseWeatherHold  = "weather_hold"
)

// Event types for one-side outages (a ride_stop would read as the whole ride stopping).
const (
	EvSideDown = "side_down" // Block = side name
	EvSideUp   = "side_up"
)

// LoadBlock is the block name used in board, dispatch and train_enter events at load.
const LoadBlock = "LOAD"

// Basement is where cabs start, before they hold any place.
const Basement = "BASEMENT"

// ReasonNextOccupied is the hold reason when the next shaft or door is taken.
const ReasonNextOccupied = "next_occupied"

const throughputWindow = 15 * engine.Minute

// Options are run-level settings that are not part of the ride file.
type Options struct {
	FixedTimes bool
	Failures   []string // config.FailBriefStop, config.FailLongDowntime, config.FailSideDown
}

// pool is one kind of place on a side (show shafts, drop shafts or unload doors).
type pool struct {
	names   []string // free places, in name order
	waiters []*cab
}

type side struct {
	name             string
	show, drop, door *pool
	downDepth        int
}

type cab struct {
	id   int
	side *side
	at   string   // the place the cab holds, "" if none
	pool *pool    // the pool `at` belongs to
	next func()   // what to do once the requested place is granted
	held bool     // waiting for a place (hold_start logged)
	work work     // the one pending timer
	seat seatings // this load
}

// Ride is one elevator_drop ride. It implements ride.Ride and lines.Ride.
type Ride struct {
	sim   *engine.Sim
	cfg   *config.ElevatorFile
	opts  Options
	lines *lines.Lines

	sides   []*side
	cabs    []*cab
	loading []*cab // cabs at load, in arrival order

	stopCount int
	stopDepth map[string]int
}

var _ ride.Ride = (*Ride)(nil)

// New builds the model. The config must have passed validation.
func New(sim *engine.Sim, cfg *config.ElevatorFile, opts Options) (*Ride, error) {
	s, v := cfg.Sides, cfg.Vehicles
	if s.Count < 1 || s.ShowShafts < 1 || s.DropShafts < 1 || s.UnloadDoors < 1 || v.PerSide < 1 || v.SeatsPerVehicle < 1 {
		return nil, fmt.Errorf("%s: elevator_drop needs sides, shafts, doors, cabs and seats", cfg.Ride.ID)
	}
	if v.PerSide >= s.ShowShafts+s.DropShafts+s.UnloadDoors {
		return nil, fmt.Errorf("%s: %d cabs per side could gridlock %d shafts and doors", cfg.Ride.ID, v.PerSide, s.ShowShafts+s.DropShafts+s.UnloadDoors)
	}
	r := &Ride{sim: sim, cfg: cfg, opts: opts, stopDepth: map[string]int{}}
	places := func(prefix, side string, n int) *pool {
		p := &pool{}
		for i := 1; i <= n; i++ {
			name := prefix + "_" + side
			if n > 1 {
				name = fmt.Sprintf("%s_%s_%d", prefix, side, i)
			}
			p.names = append(p.names, name)
		}
		return p
	}
	for i := range s.Count {
		name := fmt.Sprintf("side%d", i+1)
		if len(s.Names) == s.Count {
			name = s.Names[i]
		}
		r.sides = append(r.sides, &side{name: name,
			show: places("SHOW", name, s.ShowShafts), drop: places("DROP", name, s.DropShafts), door: places("DOOR", name, s.UnloadDoors)})
	}
	for _, sd := range r.sides {
		for range v.PerSide {
			r.cabs = append(r.cabs, &cab{id: len(r.cabs) + 1, side: sd, at: Basement})
		}
	}
	r.lines = lines.New(sim, r, &cfg.Queues, cfg.Preshow, nil)
	return r, nil
}

// ID implements ride.Ride.
func (r *Ride) ID() string { return r.cfg.Ride.ID }

// Status implements ride.Ride.
func (r *Ride) Status() ride.Status {
	if r.Stopped() {
		return ride.Down
	}
	return ride.Open
}

// Arrive implements ride.Ride.
func (r *Ride) Arrive(p *guest.Party, lane guest.Lane) { r.lines.Arrive(p, lane) }

// WaitingGuests is every guest who joined and has not boarded or reneged.
func (r *Ride) WaitingGuests() int { return r.lines.Waiting() }

// Sample implements lines.Ride (preshow timings).
func (r *Ride) Sample(d config.Dist) engine.Duration { return r.sample(d, engine.StreamStationTimes) }

// Seats is the seats per cab.
func (r *Ride) Seats() int { return r.cfg.Vehicles.SeatsPerVehicle }

// PostedWait implements ride.Ride: standby-side guests over standby riders per second in the last 15
// minutes, falling back to the theoretical rate of the sides in service.
func (r *Ride) PostedWait() engine.Duration {
	riders := r.lines.StandbyBoardedSince(r.sim.Now().Add(-throughputWindow))
	perS := float64(riders) / throughputWindow.Seconds()
	if riders == 0 || r.sim.Now() < engine.Time(throughputWindow) {
		up := 0
		for _, sd := range r.sides {
			if sd.downDepth == 0 {
				up++
			}
		}
		perS = float64(max(1, up)*r.cfg.Vehicles.SeatsPerVehicle) / r.cfg.SideInterval()
	}
	return engine.Seconds(float64(r.lines.StandbyWaiting()) / perS)
}

// Start begins operating: cabs take their places, failures are scheduled, the preshow opens.
func (r *Ride) Start() {
	r.lines.Start()
	r.startStops()
	r.startPosting()
	// Cabs start empty in the basement: each takes an unload door if one is free (or waits for one),
	// then asks for a show shaft like any cab that has just unloaded.
	for _, c := range r.cabs {
		r.request(c, c.side.door, func() { r.wantShow(c) })
	}
}

// postingRefresh matches the park's posted_wait_refresh_s.
const postingRefresh = 300 * engine.Second

func (r *Ride) startPosting() {
	var post func()
	post = func() {
		r.sim.Emit(engine.Event{Type: engine.EvPostedWait, N: int(r.PostedWait() / engine.Second), Info: config.PostedQueueLenOverThroughput})
		r.sim.After(postingRefresh, post)
	}
	post()
}

// ---------------------------------------------------------------------------------------------------
// The cab cycle

func (r *Ride) wantShow(c *cab) {
	r.request(c, c.side.show, func() {
		y := r.cfg.Cycle
		r.startWork(c, r.sample(y.ToShowShaftS, engine.StreamTrackTimes)+r.sample(y.LiftToLoadS, engine.StreamTrackTimes), func() { r.startLoading(c) })
	})
}

func (r *Ride) startLoading(c *cab) {
	r.sim.Emit(engine.Event{Type: engine.EvTrainEnter, Train: c.id, Block: LoadBlock, Info: c.at})
	c.seat = r.newSeating()
	r.loading = append(r.loading, c)
	r.boardCab(c)
	r.startWork(c, r.sample(r.cfg.Cycle.LoadS, engine.StreamStationTimes), func() { r.loadDone(c) })
}

func (r *Ride) loadDone(c *cab) {
	c.seat.fillWait = true
	r.sim.Emit(engine.Event{Type: engine.EvLoadDone, Train: c.id, Block: LoadBlock, N: c.seat.riders})
	r.boardCab(c)
	if !r.atLoad(c) { // boarding re-entered Board, which already started the check
		return
	}
	if r.fillWaitOver(c) {
		r.startCheck(c)
		return
	}
	r.startWork(c, engine.Seconds(r.cfg.Cycle.MaxWaitAfterReadyS), func() { r.startCheck(c) })
}

// startCheck ends boarding and starts the restraint check. Boarding can re-enter Board (lines.Boarded
// drains the preshow, which calls Board), so a cab whose check has already started is left alone.
func (r *Ride) startCheck(c *cab) {
	if !r.leaveLoading(c) {
		return
	}
	y := r.cfg.Cycle
	d := r.sample(y.RestraintCheckS, engine.StreamStationTimes) + r.sample(y.DispatchClearanceS, engine.StreamStationTimes)
	r.startWork(c, d, func() { r.dispatch(c) })
}

func (r *Ride) dispatch(c *cab) {
	r.sim.Emit(engine.Event{Type: engine.EvDispatch, Train: c.id, Block: LoadBlock, N: c.seat.riders, Info: c.at})
	r.startWork(c, r.sample(r.cfg.Cycle.ShowS, engine.StreamTrackTimes), func() {
		r.request(c, c.side.drop, func() {
			r.startWork(c, r.sample(r.cfg.Cycle.DropS, engine.StreamTrackTimes), func() {
				r.request(c, c.side.door, func() {
					r.startWork(c, r.sample(r.cfg.Cycle.ToUnloadS, engine.StreamTrackTimes), func() { r.unload(c) })
				})
			})
		})
	})
}

func (r *Ride) unload(c *cab) {
	r.startWork(c, r.sample(r.cfg.Cycle.UnloadS, engine.StreamStationTimes), func() {
		s := c.seat
		r.sim.Emit(engine.Event{Type: engine.EvUnload, Train: c.id, Block: c.at, N: s.riders})
		for _, p := range s.parties {
			p.Exited(guest.Rode)
		}
		c.seat = seatings{}
		r.wantShow(c)
	})
}

// request asks for a place in p. If one is free the cab moves there now (giving up its current place)
// and runs then; otherwise it waits where it is, holding its place.
func (r *Ride) request(c *cab, p *pool, then func()) {
	c.next = then
	if len(p.names) > 0 && len(p.waiters) == 0 {
		r.grant(c, p)
		return
	}
	p.waiters = append(p.waiters, c)
	c.held = true
	r.sim.Emit(engine.Event{Type: engine.EvHoldStart, Train: c.id, Block: c.at, Info: ReasonNextOccupied})
}

// grant moves c into the first free place of p, frees its old place (serving that place's waiters)
// and runs its next step.
func (r *Ride) grant(c *cab, p *pool) {
	name := p.names[0]
	p.names = p.names[1:]
	if c.held {
		c.held = false
		r.sim.Emit(engine.Event{Type: engine.EvHoldEnd, Train: c.id, Block: c.at})
	}
	old, oldPool := c.at, c.pool
	c.at, c.pool = name, p
	r.sim.Emit(engine.Event{Type: engine.EvTrainEnter, Train: c.id, Block: name})
	next := c.next
	c.next = nil
	next()
	if oldPool != nil {
		r.release(oldPool, old)
	}
}

// release frees a place and hands it to the first waiter, if any.
func (r *Ride) release(p *pool, name string) {
	p.names = insertSorted(p.names, name)
	if len(p.waiters) > 0 {
		c := p.waiters[0]
		p.waiters = p.waiters[1:]
		r.grant(c, p)
	}
}

func insertSorted(names []string, name string) []string {
	i := 0
	for i < len(names) && names[i] < name {
		i++
	}
	names = append(names, "")
	copy(names[i+1:], names[i:])
	names[i] = name
	return names
}

// leaveLoading removes c from the cabs at load and reports whether it was there.
func (r *Ride) leaveLoading(c *cab) bool {
	for i, x := range r.loading {
		if x == c {
			r.loading = append(r.loading[:i], r.loading[i+1:]...)
			return true
		}
	}
	return false
}

func (r *Ride) atLoad(c *cab) bool {
	for _, x := range r.loading {
		if x == c {
			return true
		}
	}
	return false
}

// sample draws from a normal truncated at zero; FixedTimes uses the mean and consumes no randomness.
func (r *Ride) sample(d config.Dist, stream string) engine.Duration {
	if r.opts.FixedTimes || d.SD == 0 {
		return engine.Seconds(d.Mean)
	}
	return engine.Seconds(max(0, d.Mean+d.SD*r.sim.Stream(stream).NormFloat64()))
}

// SingleRiderOpen reports whether the single-rider line takes new guests.
func (r *Ride) SingleRiderOpen() bool { return r.lines.SingleRiderOpen() }

// SetSingleRiderOpen opens or closes the single-rider line (an operations lever).
func (r *Ride) SetSingleRiderOpen(open bool) { r.lines.SetSingleRiderOpen(open) }
