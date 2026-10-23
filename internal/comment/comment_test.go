package comment

import (
	"strings"
	"testing"

	"benchmarkpro/internal/report"
)

func sampleReport(withError bool) report.Report {
	rep := report.Report{Engine: "sqlite", Benchmarks: []report.BenchmarkJSON{
		{Name: "user_lookup_by_email", Runs: 10, AvgUS: 42.5, MinUS: 39.8, MaxUS: 50.1, P99US: 50.1},
	}}
	if withError {
		rep.Benchmarks = append(rep.Benchmarks, report.BenchmarkJSON{
			Name: "broken_query", Runs: 10, Error: "no such table: nope",
		})
	}
	return rep
}

func TestFormatIncludesEveryBenchmark(t *testing.T) {
	out := Format(sampleReport(false))
	if !strings.Contains(out, "user_lookup_by_email") {
		t.Fatal("missing benchmark name")
	}
	if !strings.Contains(out, "42.5us") {
		t.Fatalf("missing formatted avg timing, got:\n%s", out)
	}
	if !strings.Contains(out, "✅ all queries ran successfully") {
		t.Fatalf("missing success summary, got:\n%s", out)
	}
}

func TestFormatSaysBaselineNotYetAvailable(t *testing.T) {
	// This is the important scope boundary: the comment must not imply a
	// regression check exists before weeks 9-10 build it.
	out := Format(sampleReport(false))
	if !strings.Contains(out, "baseline comparison not yet available") {
		t.Fatalf("comment should be explicit about not doing baseline comparison yet, got:\n%s", out)
	}
}

func TestFormatShowsErroredQueries(t *testing.T) {
	out := Format(sampleReport(true))
	if !strings.Contains(out, "broken_query") || !strings.Contains(out, "no such table: nope") {
		t.Fatalf("errored benchmark not shown, got:\n%s", out)
	}
	if !strings.Contains(out, "❌ one or more queries failed") {
		t.Fatalf("missing failure summary, got:\n%s", out)
	}
}

func TestSummarySuccess(t *testing.T) {
	state, desc := Summary(sampleReport(false))
	if state != "success" {
		t.Fatalf("want success, got %s", state)
	}
	if !strings.Contains(desc, "1 query") {
		t.Fatalf("unexpected description: %s", desc)
	}
}

func TestSummaryFailureNamesTheQuery(t *testing.T) {
	state, desc := Summary(sampleReport(true))
	if state != "failure" {
		t.Fatalf("want failure, got %s", state)
	}
	if !strings.Contains(desc, "broken_query") {
		t.Fatalf("description should name the failing query: %s", desc)
	}
}

func TestSummaryDescriptionFitsGitHubLimit(t *testing.T) {
	rep := report.Report{Benchmarks: []report.BenchmarkJSON{
		{Name: "x", Error: strings.Repeat("very long driver error message ", 20)},
	}}
	_, desc := Summary(rep)
	if len(desc) > 140 {
		t.Fatalf("description too long for GitHub's status field: %d chars", len(desc))
	}
}
