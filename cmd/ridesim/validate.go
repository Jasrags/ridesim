package main

import (
	"flag"
	"io"

	"github.com/jasrags/ridesim/internal/config"
)

func runValidate(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	dir := fs.String("data", "data", "data directory to scan")
	verbose := fs.Bool("v", false, "list passing checks individually")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ds, err := config.LoadDir(*dir)
	if err != nil {
		return err
	}
	rep := config.Validate(ds)
	if err := rep.Write(stdout, *verbose); err != nil {
		return err
	}
	if rep.HasErrors() {
		return errFailed
	}
	return nil
}
