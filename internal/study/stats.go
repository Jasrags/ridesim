package study

import (
	"math"

	"github.com/jasrags/ridesim/internal/metrics"
)

// Metric is one compared quantity, read from a replication's report.
type Metric struct {
	Name, Unit string
	Get        func(*metrics.Report) float64
}

// Metrics is the fixed, ordered list every comparison reports.
var Metrics = []Metric{
	{"riders_per_hr", "/hr", func(r *metrics.Report) float64 { return r.RidersPerHr }},
	{"availability_pct", "%", func(r *metrics.Report) float64 { return r.Ops.Availability * 100 }},
	{"standby_wait_mean_min", "min", func(r *metrics.Report) float64 { return r.LaneWait["standby"].Mean / 60 }},
	{"standby_wait_p95_min", "min", func(r *metrics.Report) float64 { return r.LaneWait["standby"].P95 / 60 }},
	{"ll_share_pct", "%", func(r *metrics.Report) float64 { return r.LLShare * 100 }},
	{"ll_wait_mean_min", "min", func(r *metrics.Report) float64 { return r.LaneWait["lightning_lane"].Mean / 60 }},
	{"balks", "guests", func(r *metrics.Report) float64 { return float64(r.Balks) }},
	{"reneges", "guests", func(r *metrics.Report) float64 { return float64(r.Reneges) }},
	{"empty_seats_per_dispatch", "seats", func(r *metrics.Report) float64 { return r.Ops.EmptySeats.Mean }},
	{"posted_mean_min", "min", func(r *metrics.Report) float64 { return r.PostedMean }},
	{"posted_peak_hr_min", "min", func(r *metrics.Report) float64 { return r.PostedPeak }},
	{"posted_error_min", "min", func(r *metrics.Report) float64 { return r.Ops.Posted.MeanAbsS / 60 }},
	{"max_queue", "guests", func(r *metrics.Report) float64 { return float64(r.QueueMax) }},
}

// Summary is a sample's mean with a 95% confidence interval.
type Summary struct {
	Mean, Low, High float64
}

// Diff is a paired difference (variant - base) with its 95% CI. Significant means the interval
// excludes zero.
type Diff struct {
	Summary
	Pct         float64 // mean difference relative to the base mean
	Significant bool
}

func values(reps []*metrics.Report, m Metric) []float64 {
	out := make([]float64, len(reps))
	for i, r := range reps {
		out[i] = m.Get(r)
	}
	return out
}

func summarize(xs []float64) Summary {
	mean, half := meanCI(xs)
	return Summary{Mean: mean, Low: mean - half, High: mean + half}
}

// Paired returns variant - base per seed, summarized.
func Paired(base, variant []float64) Diff {
	d := make([]float64, len(base))
	for i := range base {
		d[i] = variant[i] - base[i]
	}
	mean, half := meanCI(d)
	bm, _ := meanCI(base)
	out := Diff{Summary: Summary{Mean: mean, Low: mean - half, High: mean + half}}
	if bm != 0 {
		out.Pct = mean / math.Abs(bm) * 100
	}
	out.Significant = out.Low > 0 || out.High < 0
	return out
}

// meanCI returns the mean and the 95% t half-width.
func meanCI(xs []float64) (mean, half float64) {
	n := len(xs)
	if n == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(n)
	if n < 2 {
		return mean, 0
	}
	ss := 0.0
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	sd := math.Sqrt(ss / float64(n-1))
	return mean, t975(n-1) * sd / math.Sqrt(float64(n))
}

// t975 is the two-sided 95% Student t critical value for df degrees of freedom.
func t975(df int) float64 {
	table := []float64{0, 12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
		2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086,
		2.080, 2.074, 2.069, 2.064, 2.060, 2.056, 2.052, 2.048, 2.045, 2.042}
	switch {
	case df < len(table):
		return table[df]
	case df < 40:
		return 2.042 // df 30: conservative for 31-39
	case df < 60:
		return 2.021 // df 40
	case df < 120:
		return 2.000 // df 60
	}
	return 1.960
}
