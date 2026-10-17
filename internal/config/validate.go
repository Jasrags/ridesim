package config

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileReport is every check result for one file plus its recorded data conflicts.
type FileReport struct {
	Path      string
	Kind      Kind
	ID        string
	Results   []Result
	Conflicts []DataConflict
}

// Report is the outcome of validating a dataset.
type Report struct {
	Files      []FileReport
	LoadErrors []LoadError
}

// Validate runs file-local checks on every file, then the cross-file checks: unique ride ids and park
// ride_id references.
func Validate(ds *Dataset) *Report {
	rep := &Report{LoadErrors: ds.LoadErrors}
	idCount := map[string]int{}
	for _, f := range ds.Files {
		if id := f.RideID(); id != "" {
			idCount[id]++
		}
	}
	for _, f := range ds.Files {
		fr := FileReport{Path: f.Path, Kind: f.Kind, Results: CheckFile(f)}
		if ri := f.RideInfo(); ri != nil {
			fr.ID = ri.ID
			fr.Conflicts = ri.DataConflicts
			ok := idCount[ri.ID] == 1
			fr.Results = append(fr.Results, Result{
				CheckID: "dataset.unique_ride_ids", Severity: SevError, Status: passFail(ok),
				Detail: fmt.Sprintf("ride.id %s appears in %d file(s)", ri.ID, idCount[ri.ID]),
			})
		}
		if f.Kind == KindPark {
			fr.ID = f.Park.Park.ID
			fr.Results = append(fr.Results, checkParkRideRefs(ds, f.Park))
		}
		rep.Files = append(rep.Files, fr)
	}
	return rep
}

func passFail(ok bool) Status {
	if ok {
		return StatusPass
	}
	return StatusFail
}

func checkParkRideRefs(ds *Dataset, p *ParkFile) Result {
	var bad, good []string
	for _, n := range p.Nodes {
		if n.Kind != NodeRide {
			continue
		}
		if rf := ds.Ride(n.RideID); rf == nil {
			bad = append(bad, fmt.Sprintf("%s -> %s", n.ID, n.RideID))
		} else {
			good = append(good, fmt.Sprintf("%s -> %s", n.ID, rf.Path))
		}
	}
	r := Result{CheckID: "park.ride_refs", Severity: SevError, Status: passFail(len(bad) == 0)}
	if len(bad) > 0 {
		r.Detail = "unresolved ride_id: " + strings.Join(bad, ", ")
	} else {
		r.Detail = strings.Join(good, ", ")
	}
	return r
}

// FileReport returns the report for path, or nil.
func (r *Report) FileReport(path string) *FileReport {
	for i := range r.Files {
		if r.Files[i].Path == path {
			return &r.Files[i]
		}
	}
	return nil
}

// HasErrors reports whether any file failed to load or any error-level check failed.
func (r *Report) HasErrors() bool {
	if len(r.LoadErrors) > 0 {
		return true
	}
	for _, f := range r.Files {
		for _, res := range f.Results {
			if res.Status == StatusFail {
				return true
			}
		}
	}
	return false
}

// Counts returns pass, warn and fail totals across all files.
func (r *Report) Counts() (pass, warn, fail int) {
	for _, f := range r.Files {
		for _, res := range f.Results {
			switch res.Status {
			case StatusPass:
				pass++
			case StatusWarn:
				warn++
			case StatusFail:
				fail++
			}
		}
	}
	return pass, warn, fail + len(r.LoadErrors)
}

// Write renders the report as plain text.
func (r *Report) Write(w io.Writer, verbose bool) error {
	var b strings.Builder
	for _, le := range r.LoadErrors {
		fmt.Fprintf(&b, "%s\n  FAIL  load: %v\n\n", le.Path, le.Err)
	}
	for _, f := range r.Files {
		fmt.Fprintf(&b, "%s  [%s %s]\n", f.Path, f.Kind, f.ID)
		for _, res := range f.Results {
			if res.Status == StatusPass && !verbose {
				continue
			}
			fmt.Fprintf(&b, "  %-4s  %-32s %s\n", res.Status, res.CheckID, res.Detail)
		}
		if !verbose {
			if n := countStatus(f.Results, StatusPass); n > 0 {
				fmt.Fprintf(&b, "  PASS  %d check(s)\n", n)
			}
		}
		for _, c := range f.Conflicts {
			fmt.Fprintf(&b, "  CONFLICT %s: %s; chosen %s", c.Field, renderNode(&c.Values), renderNode(&c.Chosen))
			if c.Note != "" {
				fmt.Fprintf(&b, " (%s)", c.Note)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	pass, warn, fail := r.Counts()
	verdict := "OK"
	if r.HasErrors() {
		verdict = "FAILED"
	}
	fmt.Fprintf(&b, "%s: %d file(s), %d pass, %d warn, %d fail\n", verdict, len(r.Files)+len(r.LoadErrors), pass, warn, fail)
	_, err := io.WriteString(w, b.String())
	return err
}

func countStatus(rs []Result, s Status) int {
	n := 0
	for _, r := range rs {
		if r.Status == s {
			n++
		}
	}
	return n
}

// renderNode prints a YAML node compactly in source order: maps as k=v, lists as [a, b], null as null.
func renderNode(n *yaml.Node) string {
	switch n.Kind {
	case 0:
		return "null"
	case yaml.ScalarNode:
		return n.Value
	case yaml.SequenceNode:
		parts := make([]string, len(n.Content))
		for i, c := range n.Content {
			parts[i] = renderNode(c)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			parts = append(parts, n.Content[i].Value+"="+renderNode(n.Content[i+1]))
		}
		return strings.Join(parts, ", ")
	case yaml.AliasNode:
		return renderNode(n.Alias)
	}
	return n.Value
}
