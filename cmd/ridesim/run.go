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
	if err := fs.Parse(args); err != nil {
		return err
	}

	ds, err := config.LoadDir(*dataDir)
	if err != nil {
		return err
	}
	rep := config.Validate(ds)
	if rep.HasErrors() {
		_ = rep.Write(os.Stderr, false)
		return errors.New("data has validation errors; run ridesim validate")
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
