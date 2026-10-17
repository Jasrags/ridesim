package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/study"
)

func runStudy(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("study", flag.ContinueOnError)
	dataDir := fs.String("data", "data", "data directory")
	file := fs.String("file", "", "study file under the data directory")
	seeds := fs.Int("seeds", 0, "replications per variant (0 = the study's, default 20)")
	parallel := fs.Int("parallel", 0, "concurrent runs (0 = number of CPUs)")
	jsonPath := fs.String("json", "", "also write the full result as JSON here")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("study: -file is required")
	}
	ds, err := config.LoadDir(*dataDir)
	if err != nil {
		return err
	}
	if rep := config.Validate(ds); rep.HasErrors() {
		_ = rep.Write(os.Stderr, false)
		return errors.New("data has validation errors; run ridesim validate")
	}
	var st *config.StudyFile
	for _, f := range ds.Files {
		if f.Kind == config.KindStudy && filepath.Clean(f.Path) == filepath.Clean(*file) {
			st = f.Study
		}
	}
	if st == nil {
		return fmt.Errorf("%s is not a study under %s", *file, *dataDir)
	}
	res, err := study.Run(ds, &st.Study, *seeds, *parallel)
	if err != nil {
		return err
	}
	if *jsonPath != "" {
		if err := os.MkdirAll(filepath.Dir(*jsonPath), 0o755); err != nil {
			return err
		}
		f, err := os.Create(*jsonPath)
		if err != nil {
			return err
		}
		if err := res.WriteJSON(f); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return res.WriteText(stdout)
}
