// Package compare checks a fresh benchmark report against a stored baseline
// and classifies every query as faster, unchanged, slower, or a regression.
// This is the local engine behind the project rule: "block the merge if a
// query is more than 10% slower than baseline".
package compare

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"benchmarkpro/internal/report"
)

// DefaultThresholdPct is how much slower a query may get before it is
// flagged as a regression (the 10% rule from the project spec).
const DefaultThresholdPct = 10.0

// noiseBandPct is the band inside which a change is treated as run-to-run
// noise and reported as "no change" instead of faster/slower.
const noiseBandPct = 1.0

// Verdict is the classification of one query against its baseline.
type Verdict string

const (
	VerdictFaster     Verdict = "FASTER"     // improved by more than the threshold
	VerdictNoChange   Verdict = "NO CHANGE"  // within the noise band
	VerdictSlower     Verdict = "SLOWER"     // slower, but still within the threshold
	VerdictRegression Verdict = "REGRESSION" // slower than the threshold — blocks merge
	VerdictNew        Verdict = "NEW"        // no usable baseline for this query
	VerdictMissing    Verdict = "MISSING"    // in baseline, absent from this run
	VerdictError      Verdict = "ERROR"      // the benchmark itself failed in this run
)

// Query is one benchmark's comparison against the baseline.
type Query struct {
	Name          string
	BaselineAvgUS float64 // 0 when there is no usable baseline
	CurrentAvgUS  float64
	DeltaUS       float64
	DeltaPct      float64 // percent change, positive = slower
	Verdict       Verdict
}

// Result is the full comparison of a run against a baseline.
type Result struct {
	ThresholdPct float64
	Queries      []Query
	Regressions  int
	Faster       int
	New          int
	Missing      int
	Errors       int
}

// HasRegressions reports whether any query is slower than the threshold.
func (r Result) HasRegressions() bool { return r.Regressions > 0 }

// LoadBaseline reads a baseline report written by `benchmark-pro run --out`.
func LoadBaseline(path string) (report.Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return report.Report{}, fmt.Errorf("read baseline: %w", err)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return report.Report{}, fmt.Errorf("parse baseline %s: %w", path, err)
	}
	return rep, nil
}

// Compare matches benchmarks by name and classifies each one. A threshold
// <= 0 falls back to DefaultThresholdPct. Benchmarks that only exist on one
// side are reported (NEW / MISSING) but never count as regressions.
func Compare(baseline, current report.Report, thresholdPct float64) Result {
	if thresholdPct <= 0 {
		thresholdPct = DefaultThresholdPct
	}
	res := Result{ThresholdPct: thresholdPct}

	base := make(map[string]report.BenchmarkJSON, len(baseline.Benchmarks))
	for _, b := range baseline.Benchmarks {
		base[b.Name] = b
	}
	seen := make(map[string]bool, len(current.Benchmarks))

	for _, cur := range current.Benchmarks {
		seen[cur.Name] = true
		q := Query{Name: cur.Name, CurrentAvgUS: cur.AvgUS}
		b, inBaseline := base[cur.Name]

		switch {
		case cur.Error != "":
			q.Verdict = VerdictError
			res.Errors++
		case !inBaseline || b.Error != "" || b.AvgUS <= 0:
			q.Verdict = VerdictNew
			res.New++
		default:
			q.BaselineAvgUS = b.AvgUS
			q.DeltaUS = cur.AvgUS - b.AvgUS
			q.DeltaPct = q.DeltaUS / b.AvgUS * 100
			switch {
			case q.DeltaPct > thresholdPct:
				q.Verdict = VerdictRegression
				res.Regressions++
			case q.DeltaPct < -noiseBandPct:
				q.Verdict = VerdictFaster
				res.Faster++
			case q.DeltaPct > noiseBandPct:
				q.Verdict = VerdictSlower
			default:
				q.Verdict = VerdictNoChange
			}
		}
		res.Queries = append(res.Queries, q)
	}

	for _, b := range baseline.Benchmarks {
		if !seen[b.Name] {
			res.Queries = append(res.Queries, Query{
				Name:          b.Name,
				BaselineAvgUS: b.AvgUS,
				Verdict:       VerdictMissing,
			})
			res.Missing++
		}
	}
	return res
}

// Overall is the one-line verdict used at the end of every output format.
func Overall(r Result) string {
	if r.HasRegressions() {
		return fmt.Sprintf("Overall: ❌ REGRESSION (%d query(s) more than %.0f%% slower)",
			r.Regressions, r.ThresholdPct)
	}
	return "Overall: ✅ PASS (no regressions)"
}

func fmtUS(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2fms", v/1000)
	}
	return fmt.Sprintf("%.1fus", v)
}

func (q Query) cells() (baseline, current, change string) {
	baseline, current, change = "-", "-", "-"
	if q.BaselineAvgUS > 0 {
		baseline = fmtUS(q.BaselineAvgUS)
	}
	if q.Verdict != VerdictMissing && q.Verdict != VerdictError {
		current = fmtUS(q.CurrentAvgUS)
	}
	switch q.Verdict {
	case VerdictNew, VerdictMissing, VerdictError:
	default:
		change = fmt.Sprintf("%+.1f%%", q.DeltaPct)
	}
	return
}

// FormatTable renders the comparison for the terminal.
func FormatTable(r Result) string {
	header := fmt.Sprintf("%-26s%11s%11s%12s  %s", "benchmark", "baseline", "current", "change", "verdict")
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", len(header)))
	for _, q := range r.Queries {
		baseline, current, change := q.cells()
		fmt.Fprintf(&b, "\n%-26s%11s%11s%12s  %s", q.Name, baseline, current, change, q.Verdict)
	}
	b.WriteString("\n")
	b.WriteString(Overall(r))
	return b.String()
}

// FormatMarkdown renders the comparison as the PR comment BenchmarkPro
// will eventually post to GitHub.
func FormatMarkdown(r Result) string {
	var b strings.Builder
	b.WriteString("## BenchmarkPro Results\n\n")
	b.WriteString("| Query | Baseline | New | Change | Verdict |\n")
	b.WriteString("|---|---:|---:|---:|---|\n")
	for _, q := range r.Queries {
		baseline, current, change := q.cells()
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", q.Name, baseline, current, change, q.Verdict)
	}
	b.WriteString("\n")
	b.WriteString(Overall(r))
	b.WriteString("\n")
	if r.HasRegressions() {
		b.WriteString("\nBlocking merge until fixed.\n")
	}
	return b.String()
}
