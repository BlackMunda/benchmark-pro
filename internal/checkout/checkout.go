// Package checkout fetches a pull request's code onto local disk via git, so
// the runner can load its benchmarks.yaml and run against its schema/queries.
// Shells out to the git binary rather than a Go git library — one less
// dependency, and git itself is already required for report.go's commit-SHA
// lookup.
package checkout

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// FetchPR clones cloneURL into dir, fetches the PR's head ref
// (refs/pull/<number>/head — GitHub always exposes this, even for PRs from
// forks, unlike fetching by SHA directly) and checks it out. On return, dir
// contains the PR's code at its head commit.
func FetchPR(ctx context.Context, cloneURL, dir string, prNumber int) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	steps := [][]string{
		{"clone", "--no-checkout", "--depth", "50", cloneURL, dir},
		{"-C", dir, "fetch", "--depth", "1", "origin", fmt.Sprintf("pull/%d/head", prNumber)},
		{"-C", dir, "checkout", "FETCH_HEAD"},
	}
	for _, args := range steps {
		cmd := exec.CommandContext(ctx, "git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %v: %w: %s", args, err, out)
		}
	}
	return nil
}
