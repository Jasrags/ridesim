package scenario

import (
	"fmt"
	"strings"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/park"
)

// ParkRun is one park day (M7): every ride in the park runs its full model in one simulation.
type ParkRun struct {
	ID      string
	Dataset *config.Dataset
	Park    *config.ParkFile
	Seed    uint64
	Length  engine.Duration
	Warmup  engine.Duration
	Options park.Options
}

// WarmupDuration implements Runner.
func (r ParkRun) WarmupDuration() engine.Duration { return r.Warmup }

func parkFromFile(ds *config.Dataset, f *config.ScenarioFile) (ParkRun, error) {
	s := f.Scenario
	pf := ds.Park(s.ParkID)
	if pf == nil {
		return ParkRun{}, fmt.Errorf("scenario %s: park %s not found", s.ID, s.ParkID)
	}
	return ParkRun{
		ID: s.ID, Dataset: ds, Park: pf, Seed: s.Seed,
		Length: engine.Seconds(s.DurationH * 3600), Warmup: engine.Seconds(s.WarmupMin * 60),
		Options: park.Options{Attendance: s.ParkAttendance(pf), FixedTimes: s.FixedTimes, Failures: s.Failures,
			WalkingMode: s.WalkingMode, NoLightningLane: s.NoLightningLane},
	}, nil
}

// Run executes the park day and returns its event log.
func (r ParkRun) Run() (*engine.Log, error) {
	sim := engine.New(r.Seed)
	mode := r.Options.WalkingMode
	if mode == "" {
		mode = r.Park.Park.WalkingMode
	}
	scale := r.Options.Scale
	if scale == 0 {
		scale = 1
	}
	failures := "none"
	if len(r.Options.Failures) > 0 {
		failures = strings.Join(r.Options.Failures, ",")
	}
	id := r.ID
	if id == "" {
		id = "adhoc"
	}
	ct := r.Park.CalibrationTargets
	sim.Emit(engine.Event{Type: engine.EvRunStart, Info: fmt.Sprintf(
		"scenario=%s park=%s attendance=%d scale=%g walking=%s lightning_lane=%t seed=%d fixed_times=%t length_s=%.0f warmup_s=%.0f failures=%s targets=everest:%g:%g,haunted_mansion:%g:%g",
		id, r.Park.Park.ID, r.Options.Attendance, scale, mode, r.Park.LightningLane != nil && !r.Options.NoLightningLane, r.Seed,
		r.Options.FixedTimes, r.Length.Seconds(), r.Warmup.Seconds(), failures,
		ct.EverestAvgStandbyMin, ct.EverestPeakStandbyMin, ct.HauntedMansionAvgStandbyMin, ct.HauntedMansionPeakStandbyMin)})
	p, err := park.New(sim, r.Dataset, r.Park, r.Options)
	if err != nil {
		return nil, err
	}
	p.Start()
	sim.RunUntil(engine.Time(r.Length))
	sim.Emit(engine.Event{Type: engine.EvRunEnd, N: p.InPark()})
	return sim.Log, nil
}
