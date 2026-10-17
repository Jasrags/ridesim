package study

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// MetricJSON is one metric for one variant in the JSON output.
type MetricJSON struct {
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	Mean        float64   `json:"mean"`
	CILow       float64   `json:"ci95_low"`
	CIHigh      float64   `json:"ci95_high"`
	Diff        *DiffJSON `json:"diff_vs_base,omitempty"`
	Replication []float64 `json:"replications"`
}

// DiffJSON is the paired difference against the base.
type DiffJSON struct {
	Mean        float64 `json:"mean"`
	CILow       float64 `json:"ci95_low"`
	CIHigh      float64 `json:"ci95_high"`
	Pct         float64 `json:"pct"`
	Significant bool    `json:"significant"`
}

// ResultJSON is the machine-readable study output (for the future what-if front end).
type ResultJSON struct {
	Study    string        `json:"study"`
	Base     string        `json:"base_scenario"`
	Seeds    []uint64      `json:"seeds"`
	Variants []VariantJSON `json:"variants"`
}

type VariantJSON struct {
	ID      string       `json:"id"`
	Metrics []MetricJSON `json:"metrics"`
}

// JSON builds the machine-readable form.
func (r *Result) JSON() ResultJSON {
	out := ResultJSON{Study: r.Study, Base: r.Base, Seeds: r.Seeds}
	base := r.Variants[0]
	for vi, v := range r.Variants {
		vj := VariantJSON{ID: v.ID}
		for _, m := range Metrics {
			xs := values(v.Reports, m)
			s := summarize(xs)
			mj := MetricJSON{Name: m.Name, Unit: m.Unit, Mean: s.Mean, CILow: s.Low, CIHigh: s.High, Replication: xs}
			if vi > 0 {
				d := Paired(values(base.Reports, m), xs)
				mj.Diff = &DiffJSON{Mean: d.Mean, CILow: d.Low, CIHigh: d.High, Pct: d.Pct, Significant: d.Significant}
			}
			vj.Metrics = append(vj.Metrics, mj)
		}
		out.Variants = append(out.Variants, vj)
	}
	return out
}

// WriteJSON writes the JSON form, indented.
func (r *Result) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.JSON())
}

// WriteText renders a table: base mean +/- CI, then each variant's mean and paired difference.
func (r *Result) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "study %s: base %s, %d paired seeds (95%% CI, * = significant)\n\n", r.Study, r.Base, len(r.Seeds))
	j := r.JSON()
	header := fmt.Sprintf("%-26s %-22s", "metric", "base")
	for _, v := range j.Variants[1:] {
		header += fmt.Sprintf(" %-36s", v.ID)
	}
	b.WriteString(header + "\n")
	for mi, m := range Metrics {
		bm := j.Variants[0].Metrics[mi]
		row := fmt.Sprintf("%-26s %-22s", m.Name, fmt.Sprintf("%.2f +/-%.2f", bm.Mean, bm.CIHigh-bm.Mean))
		for _, v := range j.Variants[1:] {
			vm := v.Metrics[mi]
			mark := " "
			if vm.Diff.Significant {
				mark = "*"
			}
			cell := fmt.Sprintf("%.2f (%+.2f +/-%.2f, %+.1f%%)%s", vm.Mean, vm.Diff.Mean, vm.Diff.CIHigh-vm.Diff.Mean, vm.Diff.Pct, mark)
			row += fmt.Sprintf(" %-36s", cell)
		}
		b.WriteString(row + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
