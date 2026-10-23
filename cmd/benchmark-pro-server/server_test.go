package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"benchmarkpro/internal/ghclient"
)

// fakeReporter records every SetStatus/PostComment call instead of hitting
// the real GitHub API, so tests can assert on exactly what would be sent.
type fakeReporter struct {
	mu       sync.Mutex
	statuses []string // "state: description"
	comments []string
}

func (f *fakeReporter) SetStatus(owner, repo, sha string, state ghclient.StatusState, desc, ctx string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, fmt.Sprintf("%s: %s", state, desc))
	return nil
}

func (f *fakeReporter) PostComment(owner, repo string, pr int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments = append(f.comments, body)
	return nil
}

func (f *fakeReporter) snapshot() (statuses, comments []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.statuses...), append([]string(nil), f.comments...)
}

func gitRun(t *testing.T, dir string, args ...string) string {
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

// buildFixtureRepoWithSpec creates a bare-ish local repo whose PR #5 head
// carries a real benchmarks.yaml + schema.sql, pointed at an in-memory
// SQLite database — exactly what processEvent will check out and run.
func buildFixtureRepoWithSpec(t *testing.T) (repoDir, headSHA string) {
	t.Helper()
	repoDir = t.TempDir()
	gitRun(t, repoDir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hi\n"), 0o644)
	gitRun(t, repoDir, "add", ".")
	gitRun(t, repoDir, "commit", "-q", "-m", "init")

	gitRun(t, repoDir, "checkout", "-q", "-b", "feature")
	os.MkdirAll(filepath.Join(repoDir, "db"), 0o755)
	os.WriteFile(filepath.Join(repoDir, "db", "schema.sql"), []byte(`
DROP TABLE IF EXISTS logs;
DROP TABLE IF EXISTS users;
CREATE TABLE users (id INTEGER PRIMARY KEY, email VARCHAR(255) NOT NULL, name VARCHAR(100) NOT NULL, created_at TIMESTAMP NOT NULL);
CREATE TABLE logs (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, message TEXT NOT NULL, created_at TIMESTAMP NOT NULL);
`), 0o644)
	os.WriteFile(filepath.Join(repoDir, "benchmarks.yaml"), []byte(`
version: 1
database:
  engine: sqlite
  dsn: ignored-by-server
  schema: db/schema.sql
benchmarks:
  - name: count_users
    sql: "SELECT COUNT(*) FROM users"
    runs: 2
    warmup: 0
`), 0o644)
	gitRun(t, repoDir, "add", ".")
	gitRun(t, repoDir, "commit", "-q", "-m", "add benchmarks")
	headSHA = gitRun(t, repoDir, "rev-parse", "HEAD")
	headSHA = headSHA[:len(headSHA)-1]

	gitRun(t, repoDir, "update-ref", "refs/pull/5/head", "feature")
	gitRun(t, repoDir, "checkout", "-q", "main")
	gitRun(t, repoDir, "branch", "-D", "feature")
	return repoDir, headSHA
}

func signedRequest(t *testing.T, url, secret string, body []byte, event string) *http.Request {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestEndToEndWebhookRun is the full pipeline: a signed HTTP request carrying
// a real pull_request payload -> signature check -> event parse -> git clone
// of a local fixture repo's PR ref -> spec load -> in-memory SQLite ->
// benchmarks actually executed -> comment + status captured by a fake
// Reporter. Nothing here talks to the real network or the real GitHub API.
func TestEndToEndWebhookRun(t *testing.T) {
	repoDir, headSHA := buildFixtureRepoWithSpec(t)

	reporter := &fakeReporter{}
	srv := NewServer("test-secret", reporter)
	srv.WorkDir = t.TempDir()

	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	payload := []byte(fmt.Sprintf(`{
		"action": "opened",
		"number": 5,
		"pull_request": {"head": {"sha": "%s", "ref": "feature"}},
		"repository": {
			"name": "fixture", "full_name": "tester/fixture",
			"clone_url": "file://%s",
			"owner": {"login": "tester"}
		}
	}`, headSHA, repoDir))

	req := signedRequest(t, httpSrv.URL+"/webhook", "test-secret", payload, "pull_request")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("want 202 Accepted, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// processEvent runs in a goroutine after the 202; poll briefly for it to finish.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if statuses, _ := reporter.snapshot(); len(statuses) >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	statuses, comments := reporter.snapshot()
	if len(statuses) < 2 {
		t.Fatalf("expected at least pending+final status, got %v", statuses)
	}
	if statuses[0] != "pending: Running benchmarks..." {
		t.Fatalf("first status should be pending, got %q", statuses[0])
	}
	last := statuses[len(statuses)-1]
	if last != "success: 1 query ran successfully" {
		t.Fatalf("want final success status, got %q (all: %v)", last, statuses)
	}

	if len(comments) != 1 {
		t.Fatalf("want exactly one PR comment, got %d", len(comments))
	}
	if !bytes.Contains([]byte(comments[0]), []byte("count_users")) {
		t.Fatalf("comment should mention the benchmark name, got:\n%s", comments[0])
	}
}

func TestHandlerRejectsBadSignature(t *testing.T) {
	srv := NewServer("real-secret", &fakeReporter{})
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	req := signedRequest(t, httpSrv.URL+"/webhook", "wrong-secret", []byte(`{}`), "pull_request")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestHandlerIgnoresNonPullRequestEvents(t *testing.T) {
	srv := NewServer("s", &fakeReporter{})
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	req := signedRequest(t, httpSrv.URL+"/webhook", "s", []byte(`{}`), "ping")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for an ignored event type, got %d", resp.StatusCode)
	}
}

func TestHandlerIgnoresNonTriggeringActions(t *testing.T) {
	reporter := &fakeReporter{}
	srv := NewServer("s", reporter)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	payload := []byte(`{
		"action": "closed",
		"number": 1,
		"pull_request": {"head": {"sha": "abc", "ref": "x"}},
		"repository": {"name": "r", "full_name": "o/r", "clone_url": "file:///nonexistent", "owner": {"login": "o"}}
	}`)
	req := signedRequest(t, httpSrv.URL+"/webhook", "s", payload, "pull_request")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for a non-triggering action, got %d", resp.StatusCode)
	}
	time.Sleep(100 * time.Millisecond)
	statuses, _ := reporter.snapshot()
	if len(statuses) != 0 {
		t.Fatalf("a 'closed' action must not trigger a benchmark run, got statuses: %v", statuses)
	}
}
