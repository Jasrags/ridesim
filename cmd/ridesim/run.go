package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
	"github.com/jasrags/ridesim/internal/metrics"
	"github.com/jasrags/ridesim/internal/scenario"
)

func runRun(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	dataDir := fs.String("data", "data", "data directory")
	rideID := fs.String("ride", "wdw.ak.expedition_everest", "ride id")
	trains := fs.Int("trains", 0, "trains in service (0 = default_in_service)")
	seed := fs.Uint64("seed", 1, "RNG seed")
	fixed := fs.Bool("fixed-times", false, "use distribution means (sd = 0) for ride timings")
	hours := fs.Float64("hours", 4, "simulated hours")
	arrivals := fs.String("arrivals", "4000", `standby arrivals in guests/hr: "rate" or "min:rate,min:rate"`)
	warmup := fs.Float64("warmup", 60, "minutes excluded from metrics")
	logPath := fs.String("log", "", "write the JSONL event log here")
	scenarioPath := fs.String("scenario", "", "run a scenario file (other flags except -seed, -log, -data are ignored)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	seedSet := false
	fs.Visit(func(f *flag.Flag) { seedSet = seedSet || f.Name == "seed" })

	ds, err := config.LoadDir(*dataDir)
	if err != nil {
		return err
	}
	rep := config.Validate(ds)
	if rep.HasErrors() {
		_ = rep.Write(os.Stderr, false)
		return errors.New("data has validation errors; run ridesim validate")
	}
	if *scenarioPath != "" {
		return runScenarioFile(ds, *scenarioPath, *seed, seedSet, *logPath, stdout)
	}
	f := ds.Ride(*rideID)
	if f == nil {
		return fmt.Errorf("no ride %q under %s", *rideID, *dataDir)
	}
	if f.Kind != config.KindCoaster {
		return fmt.Errorf("ride %s is %s; only %s is modeled (continuous_load is M5)", *rideID, f.Kind, config.ArchetypeMultiTrainCoaster)
	}
	curve, err := guest.ParseCurve(*arrivals)
	if err != nil {
		return err
	}
	log, err := scenario.CoasterRun{
		Ride: f.Coaster, Trains: *trains, Seed: *seed, FixedTimes: *fixed,
		Length: engine.Seconds(*hours * 3600), Arrivals: curve,
	}.Run()
	if err != nil {
		return err
	}
	if *logPath != "" {
		if err := writeLog(*logPath, log); err != nil {
			return err
		}
	}
	m, err := metrics.Compute(log.Events(), engine.Seconds(*warmup*60))
	if err != nil {
		return err
	}
	return m.Write(stdout)
}

func runScenarioFile(ds *config.Dataset, path string, seed uint64, seedSet bool, logPath string, stdout io.Writer) error {
	var sf *config.ScenarioFile
	for _, f := range ds.Files {
		if f.Kind == config.KindScenario && filepath.Clean(f.Path) == filepath.Clean(path) {
			sf = f.Scenario
		}
	}
	if sf == nil {
		return fmt.Errorf("%s is not a scenario under the data directory", path)
	}
	run, err := scenario.FromFile(ds, sf)
	if err != nil {
		return err
	}
	if seedSet {
		run.Seed = seed
	}
	log, err := run.Run()
	if err != nil {
		return err
	}
	if logPath != "" {
		if err := writeLog(logPath, log); err != nil {
			return err
		}
	}
	m, err := metrics.Compute(log.Events(), run.Warmup)
	if err != nil {
		return err
	}
	return m.Write(stdout)
}

func writeLog(path string, log *engine.Log) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := log.Write(f); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}

func runReport(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	logPath := fs.String("log", "", "JSONL event log written by ridesim run")
	warmup := fs.Float64("warmup", 60, "minutes excluded from metrics")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *logPath == "" {
		return errors.New("report: -log is required")
	}
	f, err := os.Open(*logPath)
	if err != nil {
		return err
	}
	defer f.Close()
	events, err := engine.ReadLog(f)
	if err != nil {
		return fmt.Errorf("%s: %w", *logPath, err)
	}
	m, err := metrics.Compute(events, engine.Seconds(*warmup*60))
	if err != nil {
		return err
	}
	return m.Write(stdout)
}
