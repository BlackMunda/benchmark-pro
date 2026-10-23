// The webhook server: receives GitHub pull_request events, checks out the
// PR's code, runs its benchmarks.yaml, and reports back as a commit status
// + PR comment. This is the weeks 3-4 "GitHub Webhook" deliverable; it calls
// the same runner/spec/dbadapter packages the local CLI (cmd/benchmark-pro)
// uses, so local and CI runs behave identically.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"benchmarkpro/internal/checkout"
	"benchmarkpro/internal/comment"
	"benchmarkpro/internal/dbadapter"
	"benchmarkpro/internal/ghclient"
	"benchmarkpro/internal/report"
	"benchmarkpro/internal/runner"
	"benchmarkpro/internal/sampledata"
	"benchmarkpro/internal/spec"
	"benchmarkpro/internal/webhook"
)

const statusContext = "benchmark-pro"
const maxBodyBytes = 1 << 20 // 1 MiB: pull_request payloads are small; refuse anything larger

// Server holds everything the handler needs. Reporter and now/clone/fetch
// are fields (not hardcoded calls) so tests can substitute fakes.
type Server struct {
	Secret   string
	Reporter ghclient.Reporter
	WorkDir  string // base dir for per-PR checkouts; defaults to os.TempDir()
	FetchPR  func(ctx context.Context, cloneURL, dir string, prNumber int) error
}

func NewServer(secret string, reporter ghclient.Reporter) *Server {
	return &Server{
		Secret:   secret,
		Reporter: reporter,
		WorkDir:  os.TempDir(),
		FetchPR:  checkout.FetchPR,
	}
}

func (s *Server) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, "could not read body", http.StatusBadRequest)
			return
		}

		if !webhook.VerifySignature(s.Secret, body, r.Header.Get("X-Hub-Signature-256")) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		if r.Header.Get("X-GitHub-Event") != "pull_request" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ignored: not a pull_request event")
			return
		}

		event, err := webhook.ParsePullRequestEvent(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if !event.ShouldRun() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "ignored: action %q does not trigger a run\n", event.Action)
			return
		}

		// Ack immediately — GitHub expects a response within ~10s, and a
		// clone + benchmark run can take longer. The real work happens in
		// the background; results land as a status + comment on the PR.
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintln(w, "accepted: benchmark run started")

		go func() {
			if err := s.processEvent(context.Background(), *event); err != nil {
				log.Printf("pull_request #%d (%s): %v", event.Number, event.Repository.FullName, err)
			}
		}()
	}
}

// processEvent does the real work for one PR event. Exported behavior (not
// just the HTTP handler) so tests can call it directly against a local git
// fixture without going through a real webhook delivery.
func (s *Server) processEvent(ctx context.Context, event webhook.PullRequestEvent) error {
	owner := event.Repository.Owner.Login
	repoName := event.Repository.Name
	sha := event.PullRequest.Head.SHA

	setStatus := func(state ghclient.StatusState, desc string) {
		if err := s.Reporter.SetStatus(owner, repoName, sha, state, desc, statusContext); err != nil {
			log.Printf("SetStatus failed: %v", err)
		}
	}

	setStatus(ghclient.StatusPending, "Running benchmarks...")

	dir, err := os.MkdirTemp(s.WorkDir, fmt.Sprintf("bp-pr%d-", event.Number))
	if err != nil {
		setStatus(ghclient.StatusError, "internal error: could not create checkout dir")
		return err
	}
	defer os.RemoveAll(dir)

	if err := s.FetchPR(ctx, event.Repository.CloneURL, dir, event.Number); err != nil {
		setStatus(ghclient.StatusError, "could not fetch PR code")
		return fmt.Errorf("fetch PR: %w", err)
	}

	specPath := filepath.Join(dir, "benchmarks.yaml")
	if _, err := os.Stat(specPath); err != nil {
		setStatus(ghclient.StatusError, "no benchmarks.yaml found in repo root")
		return fmt.Errorf("stat spec: %w", err)
	}
	sp, err := spec.Load(specPath)
	if err != nil {
		setStatus(ghclient.StatusError, "benchmarks.yaml is invalid: "+err.Error())
		return fmt.Errorf("load spec: %w", err)
	}
	if sp.Database.Engine != "sqlite" {
		// Postgres needs a dedicated, isolated database per run; wiring that
		// up for CI is left for a later iteration (see docs/DESIGN.md).
		setStatus(ghclient.StatusError, "server currently only supports engine: sqlite")
		return fmt.Errorf("unsupported engine for CI runs: %s", sp.Database.Engine)
	}

	// In-memory, isolated per run — concurrent webhook deliveries never share state.
	adapter, err := dbadapter.Open("sqlite", ":memory:")
	if err != nil {
		setStatus(ghclient.StatusError, "internal error: could not open database")
		return fmt.Errorf("open db: %w", err)
	}
	defer adapter.Close()

	script, err := os.ReadFile(sp.Database.Schema)
	if err != nil {
		setStatus(ghclient.StatusError, "internal error: could not read schema")
		return fmt.Errorf("read schema: %w", err)
	}
	if err := adapter.ExecuteScript(string(script)); err != nil {
		setStatus(ghclient.StatusError, "internal error: schema failed to apply")
		return fmt.Errorf("apply schema: %w", err)
	}
	if err := adapter.SeedWith(sampledata.DefaultConfig()); err != nil {
		setStatus(ghclient.StatusError, "internal error: seeding failed")
		return fmt.Errorf("seed: %w", err)
	}

	results := runner.RunAll(adapter, sp, nil)
	rep := report.Build(results, "sqlite")

	if err := s.Reporter.PostComment(owner, repoName, event.Number, comment.Format(rep)); err != nil {
		log.Printf("PostComment failed: %v", err)
	}

	state, desc := comment.Summary(rep)
	setStatus(ghclient.StatusState(state), desc)
	return nil
}
