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
	scenarioPath := fs.String("scenario", "", "park scenario to play (this or -challenge)")
	challengePath := fs.String("challenge", "", "challenge to play, e.g. data/challenges/dhs-tame-slinky.yaml")
	date := fs.String("date", "", "play date YYYY-MM-DD for daily-seed challenges (default today)")
	addr := fs.String("addr", "127.0.0.1:8080", "listen address (loopback by default)")
	seed := fs.Uint64("seed", 0, "RNG seed (0 = the scenario's; not with -challenge)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*scenarioPath == "") == (*challengePath == "") {
		return errors.New("serve: give -scenario (a park scenario) or -challenge (a challenge file)")
	}
	ds, err := loadValid(*dataDir)
	if err != nil {
		return err
	}
	var srv *web.Server
	var title string
	var played *config.ScenarioFile
	if *challengePath != "" {
		if *seed != 0 {
			return errors.New("serve: -seed does not apply to a challenge (its seed is fixed or the date's)")
		}
		c, err := findChallenge(ds, *challengePath)
		if err != nil {
			return err
		}
		sd, err := c.Seed(playDate(*date))
		if err != nil {
			return err
		}
		played = c.Day(sd)
		sess, err := live.New(ds, played)
		if err != nil {
			return err
		}
		srv, title = web.NewChallenge(sess, c, *challengePath), "challenge "+c.Title
	} else {
		played, err = findScenario(ds, *scenarioPath)
		if err != nil {
			return err
		}
		if *seed != 0 {
			c := *played
			c.Scenario.Seed = *seed
			played = &c
		}
		sess, err := live.New(ds, played)
		if err != nil {
			return err
		}
		srv, title = web.New(sess, played.Scenario.ID+"_played"), played.Scenario.ID
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ridesim: playing %s (seed %d) at http://%s/  (ctrl-c to stop)\n", title, played.Scenario.Seed, ln.Addr())
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

func findScenario(ds *config.Dataset, path string) (*config.ScenarioFile, error) {
	for _, f := range ds.Files {
		if f.Kind == config.KindScenario && filepath.Clean(f.Path) == filepath.Clean(path) {
			return f.Scenario, nil
		}
	}
	return nil, fmt.Errorf("%s is not a scenario under the data directory", path)
}
