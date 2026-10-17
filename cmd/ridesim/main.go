// Command ridesim validates ride/park data, runs ride simulations, and reports metrics from event logs.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// errFailed signals a completed command whose outcome is a failure (for example validation errors);
// the message has already been printed.
var errFailed = errors.New("failed")

type command struct {
	name, summary string
	run           func(args []string, stdout io.Writer) error
}

var commands = []command{
	{"validate", "load every file under the data directory and check all invariants", runValidate},
	{"run", "simulate one ride (multi_train_coaster) and print metrics", runRun},
	{"report", "recompute metrics from a saved event log", runReport},
	{"study", "run an A/B study: base scenario vs variants on paired seeds", runStudy},
	{"serve", "play a park scenario live in the browser (control room)", runServe},
}

func main() {
	if err := dispatch(os.Args[1:], os.Stdout); err != nil {
		if !errors.Is(err, errFailed) {
			fmt.Fprintln(os.Stderr, "ridesim:", err)
		}
		os.Exit(1)
	}
}

func dispatch(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "help" {
		usage(stdout)
		return nil
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:], stdout)
		}
	}
	usage(os.Stderr)
	return fmt.Errorf("unknown command %q", args[0])
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: ridesim <command> [flags]")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-9s %s\n", c.name, c.summary)
	}
}
