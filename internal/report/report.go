// Package report builds the JSON result — the contract weeks 9-10's
// baseline comparison will read. Matches the Python version's schema exactly.
package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"benchmarkpro/internal/runner"
	"benchmarkpro/internal/stats"
)

const SchemaVersion = 1

type BenchmarkJSON struct {
	Name      string    `json:"name"`
	Runs      int       `json:"runs"`
	Warmup    int       `json:"warmup"`
	Error     string    `json:"error,omitempty"`
	AvgUS     float64   `json:"avg_us,omitempty"`
	MinUS     float64   `json:"min_us,omitempty"`
	MaxUS     float64   `json:"max_us,omitempty"`
	P99US     float64   `json:"p99_us,omitempty"`
	StdevUS   float64   `json:"stdev_us,omitempty"`
	SamplesUS []float64 `json:"samples_us,omitempty"`
}

type Report struct {
	SchemaVersion int             `json:"schema_version"`
	CommitSHA     *string         `json:"commit_sha"`
	CreatedAt     string          `json:"created_at"`
	Engine        string          `json:"engine"`
	GoVersion     string          `json:"go_version"`
	Benchmarks    []BenchmarkJSON `json:"benchmarks"`
}

func currentCommit() *string {
	if sha := os.Getenv("GITHUB_SHA"); sha != "" {
		return &sha
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return nil
	}
	sha := strings.TrimSpace(string(out))
	return &sha
}

func Build(results []runner.Result, engine string) Report {
	benchmarks := make([]BenchmarkJSON, len(results))
	for i, r := range results {
		b := BenchmarkJSON{Name: r.Name, Runs: r.Runs, Warmup: r.Warmup}
		if r.Err != nil {
			b.Error = r.Err.Error()
		} else {
			s, _ := r.Stats()
			b.AvgUS, b.MinUS, b.MaxUS, b.P99US, b.StdevUS = s.AvgUS, s.MinUS, s.MaxUS, s.P99US, s.StdevUS
			b.SamplesUS = stats.SamplesUS(r.Timings)
		}
		benchmarks[i] = b
	}
	return Report{
		SchemaVersion: SchemaVersion,
		CommitSHA:     currentCommit(),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Engine:        engine,
		GoVersion:     runtime.Version(),
		Benchmarks:    benchmarks,
	}
}

func Write(rep Report, path string) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
