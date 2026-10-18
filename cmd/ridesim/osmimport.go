package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jasrags/ridesim/internal/osm"
)

// runOSMImport writes a draft park file from a saved OSM extract and a mapping file. It never
// overwrites the draft: once written, the park file is edited by hand.
func runOSMImport(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("osm-import", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "mapping file (YAML) naming the extract, walkway rules and park nodes")
	out := fs.String("out", "", "draft park YAML to create (must not exist)")
	svgPath := fs.String("svg", "", "optional review map (SVG) to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cfgPath == "" || *out == "" {
		return errors.New("osm-import: -config and -out are required")
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("osm-import: %s exists; the importer never rewrites a park file", *out)
	}
	cfg, err := osm.LoadConfig(*cfgPath)
	if err != nil {
		return err
	}
	extract := filepath.Join(filepath.Dir(*cfgPath), cfg.Park.Extract)
	f, err := os.Open(extract)
	if err != nil {
		return err
	}
	x, err := osm.Parse(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", extract, err)
	}
	d, err := osm.Build(x, cfg)
	if err != nil {
		return err
	}
	if err := writeNew(*out, func(w io.Writer) error { return d.WriteYAML(w, filepath.ToSlash(extract)) }); err != nil {
		return err
	}
	if *svgPath != "" {
		sf, err := os.Create(*svgPath)
		if err != nil {
			return err
		}
		if err := d.WriteSVG(sf); err != nil {
			sf.Close()
			return err
		}
		if err := sf.Close(); err != nil {
			return err
		}
	}
	var junctions int
	for _, n := range d.Nodes {
		if n.Kind == "junction" {
			junctions++
		}
	}
	fmt.Fprintf(stdout, "wrote %s: %d nodes (%d junctions), %d edges\n", *out, len(d.Nodes), junctions, len(d.Edges))
	for _, n := range d.Notes {
		fmt.Fprintln(stdout, "  note:", n)
	}
	return nil
}

func writeNew(path string, write func(io.Writer) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
