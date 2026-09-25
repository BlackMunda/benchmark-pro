// Package runner drives benchmarks against an Adapter and collects timings.
package runner

import (
	"benchmarkpro/internal/dbadapter"
	"benchmarkpro/internal/spec"
	"benchmarkpro/internal/stats"
)

type Result struct {
	Name    string
	Runs    int
	Warmup  int
	Timings []int64 // nanoseconds
	Err     error
}

func (r Result) Stats() (stats.Summary, error) {
	return stats.Summarize(r.Timings)
}

func Run(a *dbadapter.Adapter, b spec.Benchmark) Result {
	res := Result{Name: b.Name, Runs: b.Runs, Warmup: b.Warmup}

	for i := 0; i < b.Warmup; i++ {
		if _, err := a.TimeQuery(b.SQL, b.Params, b.Rollback); err != nil {
			res.Err = err
			return res
		}
	}
	timings := make([]int64, 0, b.Runs)
	for i := 0; i < b.Runs; i++ {
		ns, err := a.TimeQuery(b.SQL, b.Params, b.Rollback)
		if err != nil {
			res.Err = err // one broken query must not hide the others
			return res
		}
		timings = append(timings, ns)
	}
	res.Timings = timings
	return res
}

func RunAll(a *dbadapter.Adapter, s *spec.Spec, onStart func(spec.Benchmark)) []Result {
	results := make([]Result, 0, len(s.Benchmarks))
	for _, b := range s.Benchmarks {
		if onStart != nil {
			onStart(b)
		}
		results = append(results, Run(a, b))
	}
	return results
}
