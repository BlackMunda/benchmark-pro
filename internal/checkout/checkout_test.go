package checkout

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// buildFixtureRepo creates a local repo that behaves like a GitHub repo with
// one open PR: a main branch, a feature commit only reachable via
// refs/pull/<prNumber>/head (exactly how GitHub exposes PR heads), so
// FetchPR has to actually use that ref rather than just cloning main.
func buildFixtureRepo(t *testing.T, prNumber int) (repoDir string, headSHA string) {
	t.Helper()
	repoDir = t.TempDir()
	run(t, repoDir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("main\n"), 0o644)
	run(t, repoDir, "add", ".")
	run(t, repoDir, "commit", "-q", "-m", "initial commit on main")

	run(t, repoDir, "checkout", "-q", "-b", "feature")
	os.WriteFile(filepath.Join(repoDir, "benchmarks.yaml"), []byte("version: 1\n"), 0o644)
	run(t, repoDir, "add", ".")
	run(t, repoDir, "commit", "-q", "-m", "feature commit")
	headSHA = run(t, repoDir, "rev-parse", "HEAD")
	headSHA = headSHA[:len(headSHA)-1] // trim trailing newline

	// Expose the feature commit the way GitHub exposes a PR head, and hide
	// the branch itself so a plain clone can't reach it any other way.
	run(t, repoDir, "update-ref", "refs/pull/"+itoa(prNumber)+"/head", "feature")
	run(t, repoDir, "checkout", "-q", "main") // must switch off "feature" before deleting it
	run(t, repoDir, "branch", "-D", "feature")

	return repoDir, headSHA
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestFetchPRChecksOutPRHead(t *testing.T) {
	repoDir, headSHA := buildFixtureRepo(t, 7)
	dest := t.TempDir()
	dest = filepath.Join(dest, "checkout") // FetchPR's git clone creates this dir

	if err := FetchPR(context.Background(), "file://"+repoDir, dest, 7); err != nil {
		t.Fatalf("FetchPR failed: %v", err)
	}

	gotSHA := run(t, dest, "rev-parse", "HEAD")
	gotSHA = gotSHA[:len(gotSHA)-1]
	if gotSHA != headSHA {
		t.Fatalf("checked out %s, want PR head %s", gotSHA, headSHA)
	}

	if _, err := os.Stat(filepath.Join(dest, "benchmarks.yaml")); err != nil {
		t.Fatalf("expected benchmarks.yaml from the feature commit to be present: %v", err)
	}
}

func TestFetchPRFailsForUnknownPR(t *testing.T) {
	repoDir, _ := buildFixtureRepo(t, 1)
	dest := filepath.Join(t.TempDir(), "checkout")
	if err := FetchPR(context.Background(), "file://"+repoDir, dest, 999); err == nil {
		t.Fatal("expected an error fetching a PR ref that doesn't exist")
	}
}
