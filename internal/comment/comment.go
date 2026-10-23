// Package comment formats a benchmark report as GitHub-flavored Markdown for
// a PR comment.
//
// IMPORTANT SCOPE NOTE: baseline comparison is a weeks 9-10 deliverable, not
// built yet. This package can only report "did these queries run, and how
// fast" — it has no previous commit's numbers to diff against, so it cannot
// yet say "regression" or enforce the 10% threshold from the V1 brief. The
// heading below says so explicitly rather than implying more than the
// system currently does.
package comment

import (
	"fmt"
	"strings"

	"benchmarkpro/internal/report"
)

func fmtUS(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2fms", v/1000)
	}
	return fmt.Sprintf("%.1fus", v)
}

// Format renders rep as a Markdown table plus a one-line summary.
func Format(rep report.Report) string {
	var b strings.Builder
	b.WriteString("## BenchmarkPro Results\n\n")
	b.WriteString(fmt.Sprintf("Engine: `%s` · _baseline comparison not yet available (weeks 9-10) — these are this PR's raw timings, not a regression check._\n\n", rep.Engine))
	b.WriteString("| Query | Runs | Avg | Min | Max | P99 | Status |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")

	anyError := false
	for _, bm := range rep.Benchmarks {
		if bm.Error != "" {
			anyError = true
			b.WriteString(fmt.Sprintf("| `%s` | %d | - | - | - | - | ❌ %s |\n", bm.Name, bm.Runs, bm.Error))
			continue
		}
		b.WriteString(fmt.Sprintf("| `%s` | %d | %s | %s | %s | %s | ✅ |\n",
			bm.Name, bm.Runs, fmtUS(bm.AvgUS), fmtUS(bm.MinUS), fmtUS(bm.MaxUS), fmtUS(bm.P99US)))
	}

	b.WriteString("\n")
	if anyError {
		b.WriteString("**Overall: ❌ one or more queries failed to run.**\n")
	} else {
		b.WriteString("**Overall: ✅ all queries ran successfully.**\n")
	}
	return b.String()
}

// Summary is a one-line status, used for the commit status description
// (GitHub truncates that field to ~140 chars, so it must stay short).
func Summary(rep report.Report) (state string, description string) {
	for _, bm := range rep.Benchmarks {
		if bm.Error != "" {
			return "failure", fmt.Sprintf("%s failed: %s", bm.Name, truncate(bm.Error, 80))
		}
	}
	return "success", fmt.Sprintf("%d quer%s ran successfully", len(rep.Benchmarks), plural(len(rep.Benchmarks)))
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
