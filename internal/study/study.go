// Package study runs A/B comparisons (spec §12 M6): a base scenario and variants, each replicated over
// the same seeds. Named RNG streams make those common random numbers, so a variant that only changes,
// say, the train count sees the same arrivals and failures and the paired difference has little noise.
package study

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/metrics"
	"github.com/jasrags/ridesim/internal/scenario"
)

// Result is every replication of every variant, in seed order.
type Result struct {
	Study    string
	Base     string
	Seeds    []uint64
	Variants []VariantResult // base first
}

// VariantResult holds one variant's replications.
type VariantResult struct {
	ID      string
	Reports []*metrics.Report
}

// Run resolves and replicates a study. seeds overrides the study's count when > 0; parallel bounds the
// concurrent runs (0 = GOMAXPROCS). Each run is single-threaded and deterministic; results are ordered
// by seed, so the output does not depend on scheduling.
func Run(ds *config.Dataset, st *config.Study, seeds, parallel int) (*Result, error) {
	variants, err := st.Resolve(ds)
	if err != nil {
		return nil, err
	}
	n := seeds
	if n <= 0 {
		n = st.Seeds
	}
	if n <= 0 {
		n = config.DefaultStudySeeds
	}
	res := &Result{Study: st.ID, Base: st.Base}
	for i := 1; i <= n; i++ {
		res.Seeds = append(res.Seeds, uint64(i))
	}
	for _, v := range variants {
		runner, err := build(ds, v)
		if err != nil {
			return nil, fmt.Errorf("study %s variant %s: %w", st.ID, v.ID, err)
		}
		reps, err := replicate(runner, res.Seeds, parallel)
		if err != nil {
			return nil, fmt.Errorf("study %s variant %s: %w", st.ID, v.ID, err)
		}
		res.Variants = append(res.Variants, VariantResult{ID: v.ID, Reports: reps})
	}
	return res, nil
}

// build turns a resolved variant into a runner, swapping in its ride override if it has one.
func build(ds *config.Dataset, v config.ResolvedVariant) (scenario.Runner, error) {
	data := ds
	if v.Ride != nil {
		files := make([]*config.File, 0, len(ds.Files))
		for _, f := range ds.Files {
			if f.RideID() == v.Ride.RideID() {
				files = append(files, v.Ride)
			} else {
				files = append(files, f)
			}
		}
		data = &config.Dataset{Files: files}
	}
	run, err := scenario.Load(data, v.Scenario)
	if err != nil {
		return nil, err
	}
	arr, ll := 1.0, 1.0
	if k, ok := v.Scale[config.ScaleArrivals]; ok {
		arr = k
	}
	if k, ok := v.Scale[config.ScaleLightningLane]; ok {
		ll = k
	}
	if arr != 1 || ll != 1 {
		run = scenario.WithScale(run, arr, ll)
	}
	return run, nil
}

func replicate(run scenario.Runner, seeds []uint64, parallel int) ([]*metrics.Report, error) {
	if parallel <= 0 {
		parallel = runtime.GOMAXPROCS(0)
	}
	out := make([]*metrics.Report, len(seeds))
	errs := make([]error, len(seeds))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, seed uint64) {
			defer wg.Done()
			defer func() { <-sem }()
			log, err := scenario.WithSeed(run, seed).Run()
			if err != nil {
				errs[i] = err
				return
			}
			out[i], errs[i] = metrics.Compute(log.Events(), run.WarmupDuration())
		}(i, seed)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("seed %d: %w", seeds[i], err)
		}
	}
	return out, nil
}
