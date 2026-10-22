package compare

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"benchmarkpro/internal/report"
)

// reportOf builds a minimal report from name -> avg microseconds.
func reportOf(m map[string]float64) report.Report {
	rep := report.Report{SchemaVersion: report.SchemaVersion, Engine: "sqlite"}
	for name, avg := range m {
		rep.Benchmarks = append(rep.Benchmarks, report.BenchmarkJSON{Name: name, Runs: 10, AvgUS: avg})
	}
	return rep
}

// verdictOf finds one query's verdict by name.
func verdictOf(t *testing.T, r Result, name string) Verdict {
	t.Helper()
	for _, q := range r.Queries {
		if q.Name == name {
			return q.Verdict
		}
	}
	t.Fatalf("query %q not found in comparison", name)
	return ""
}

func TestFasterQuery(t *testing.T) {
	base := reportOf(map[string]float64{"user_lookup": 100})
	cur := reportOf(map[string]float64{"user_lookup": 80}) // -20%
	r := Compare(base, cur, DefaultThresholdPct)
	if v := verdictOf(t, r, "user_lookup"); v != VerdictFaster {
		t.Fatalf("got %s, want FASTER", v)
	}
	if r.HasRegressions() {
		t.Fatal("a faster query must not be a regression")
	}
}

func TestNoChangeWithinNoiseBand(t *testing.T) {
	base := reportOf(map[string]float64{"create_user": 100})
	cur := reportOf(map[string]float64{"create_user": 100.5}) // +0.5%
	r := Compare(base, cur, DefaultThresholdPct)
	if v := verdictOf(t, r, "create_user"); v != VerdictNoChange {
		t.Fatalf("got %s, want NO CHANGE", v)
	}
}

func TestSlowerButWithinThreshold(t *testing.T) {
	base := reportOf(map[string]float64{"logs_count": 100})
	cur := reportOf(map[string]float64{"logs_count": 105}) // +5%, under the 10% rule
	r := Compare(base, cur, DefaultThresholdPct)
	if v := verdictOf(t, r, "logs_count"); v != VerdictSlower {
		t.Fatalf("got %s, want SLOWER", v)
	}
	if r.HasRegressions() {
		t.Fatal("5% slower must not block the merge")
	}
}

func TestRegressionOverThreshold(t *testing.T) {
	base := reportOf(map[string]float64{"batch_delete": 5000})
	cur := reportOf(map[string]float64{"batch_delete": 5500}) // +10% exactly? 10% is not > 10%
	r := Compare(base, cur, DefaultThresholdPct)
	if v := verdictOf(t, r, "batch_delete"); v != VerdictSlower {
		t.Fatalf("exactly +10%% (not more) got %s, want SLOWER", v)
	}

	cur = reportOf(map[string]float64{"batch_delete": 5600}) // +12%
	r = Compare(base, cur, DefaultThresholdPct)
	if v := verdictOf(t, r, "batch_delete"); v != VerdictRegression {
		t.Fatalf("got %s, want REGRESSION", v)
	}
	if !r.HasRegressions() || r.Regressions != 1 {
		t.Fatalf("Regressions = %d, want 1", r.Regressions)
	}
}

func TestCustomThreshold(t *testing.T) {
	base := reportOf(map[string]float64{"q": 100})
	cur := reportOf(map[string]float64{"q": 104}) // +4%
	if r := Compare(base, cur, 2); !r.HasRegressions() {
		t.Fatal("+4% with a 2% threshold must be a regression")
	}
	if r := Compare(base, cur, 5); r.HasRegressions() {
		t.Fatal("+4% with a 5% threshold must not be a regression")
	}
}

func TestThresholdZeroFallsBackToDefault(t *testing.T) {
	base := reportOf(map[string]float64{"q": 100})
	cur := reportOf(map[string]float64{"q": 115}) // +15%
	r := Compare(base, cur, 0)
	if r.ThresholdPct != DefaultThresholdPct {
		t.Fatalf("ThresholdPct = %v, want default %v", r.ThresholdPct, DefaultThresholdPct)
	}
	if !r.HasRegressions() {
		t.Fatal("+15% must be a regression under the default 10% rule")
	}
}

func TestNewAndMissingQueries(t *testing.T) {
	base := reportOf(map[string]float64{"old_query": 100})
	cur := reportOf(map[string]float64{"brand_new": 50})
	r := Compare(base, cur, DefaultThresholdPct)
	if v := verdictOf(t, r, "brand_new"); v != VerdictNew {
		t.Fatalf("new query got %s, want NEW", v)
	}
	if v := verdictOf(t, r, "old_query"); v != VerdictMissing {
		t.Fatalf("removed query got %s, want MISSING", v)
	}
	if r.HasRegressions() {
		t.Fatal("NEW/MISSING must never count as regressions")
	}
}

func TestErroredBenchmarkIsNotANewQuery(t *testing.T) {
	base := reportOf(map[string]float64{"q": 100})
	cur := report.Report{SchemaVersion: report.SchemaVersion, Benchmarks: []report.BenchmarkJSON{
		{Name: "q", Runs: 10, Error: "no such column: foo"},
	}}
	r := Compare(base, cur, DefaultThresholdPct)
	if v := verdictOf(t, r, "q"); v != VerdictError {
		t.Fatalf("got %s, want ERROR", v)
	}
	if r.Errors != 1 || r.HasRegressions() {
		t.Fatalf("Errors = %d, HasRegressions = %v; want 1 error, no regression", r.Errors, r.HasRegressions())
	}
}

func TestLoadBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	rep := reportOf(map[string]float64{"q": 123})
	if err := report.Write(rep, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Benchmarks) != 1 || loaded.Benchmarks[0].AvgUS != 123 {
		t.Fatalf("unexpected baseline content: %+v", loaded)
	}
}

func TestLoadBaselineBadFile(t *testing.T) {
	if _, err := LoadBaseline(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("want error for missing baseline file")
	}
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(path); err == nil {
		t.Fatal("want error for unparseable baseline file")
	}
}

func TestFormatTableAndMarkdown(t *testing.T) {
	base := reportOf(map[string]float64{"fast_q": 100, "slow_q": 5000})
	cur := reportOf(map[string]float64{"fast_q": 80, "slow_q": 5600})
	r := Compare(base, cur, DefaultThresholdPct)

	table := FormatTable(r)
	for _, want := range []string{"fast_q", "slow_q", "REGRESSION", "FASTER", "❌ REGRESSION"} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing %q:\n%s", want, table)
		}
	}

	md := FormatMarkdown(r)
	for _, want := range []string{"## BenchmarkPro Results", "| Query |", "REGRESSION", "Blocking merge until fixed."} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestFormatMarkdownPass(t *testing.T) {
	base := reportOf(map[string]float64{"q": 100})
	cur := reportOf(map[string]float64{"q": 95})
	md := FormatMarkdown(Compare(base, cur, DefaultThresholdPct))
	if !strings.Contains(md, "✅ PASS") {
		t.Fatalf("passing run should render ✅ PASS:\n%s", md)
	}
	if strings.Contains(md, "Blocking merge") {
		t.Fatalf("passing run must not mention blocking:\n%s", md)
	}
}
