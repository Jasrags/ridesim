package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/live"
	"github.com/jasrags/ridesim/internal/web"
)

// tickEvery is how often the server advances the sim and pushes a snapshot to open pages.
const tickEvery = 250 * time.Millisecond

func runServe(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dataDir := fs.String("data", "data", "data directory")
	scenarioPath := fs.String("scenario", "", "park scenario to play (required)")
	addr := fs.String("addr", "127.0.0.1:8080", "listen address (loopback by default)")
	seed := fs.Uint64("seed", 0, "RNG seed (0 = the scenario's)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *scenarioPath == "" {
		return errors.New("serve: -scenario is required (a park scenario, e.g. data/scenarios/park-medium-day.yaml)")
	}
	ds, err := config.LoadDir(*dataDir)
	if err != nil {
		return err
	}
	if rep := config.Validate(ds); rep.HasErrors() {
		return errors.New("data has validation errors; run ridesim validate")
	}
	var sf *config.ScenarioFile
	for _, f := range ds.Files {
		if f.Kind == config.KindScenario && filepath.Clean(f.Path) == filepath.Clean(*scenarioPath) {
			sf = f.Scenario
		}
	}
	if sf == nil {
		return fmt.Errorf("%s is not a scenario under %s", *scenarioPath, *dataDir)
	}
	if *seed != 0 {
		c := *sf
		c.Scenario.Seed = *seed
		sf = &c
	}
	sess, err := live.New(ds, sf)
	if err != nil {
		return err
	}
	srv := web.New(sess, sf.Scenario.ID+"_played")
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ridesim: playing %s (seed %d) at http://%s/  (ctrl-c to stop)\n", sf.Scenario.ID, sf.Scenario.Seed, ln.Addr())
	stop := make(chan struct{})
	errc := make(chan error, 1)
	go func() { errc <- srv.Run(tickEvery, stop) }()
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { errc <- httpSrv.Serve(ln) }()
	err = <-errc
	close(stop)
	_ = httpSrv.Close()
	return err
}
