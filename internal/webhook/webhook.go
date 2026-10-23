// Package webhook verifies and parses GitHub's pull_request webhook payloads.
// It knows nothing about running benchmarks or calling the GitHub API — it
// only answers "is this request genuinely from GitHub, and if so, what PR
// event is it?" The server (cmd/benchmark-pro-server) decides what to do
// with that answer.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// RunActions are the pull_request actions that should trigger a benchmark
// run, per the V1 brief ("when PR created/updated, run tests").
var RunActions = map[string]bool{
	"opened":      true,
	"synchronize": true, // a new commit was pushed to the PR branch
	"reopened":    true,
}

// VerifySignature checks the X-Hub-Signature-256 header GitHub sends against
// an HMAC-SHA256 of the raw request body, using the webhook secret both
// sides were configured with. This MUST be checked before the body is
// trusted or parsed — otherwise anyone who finds the URL can trigger runs
// (and, once weeks 9-11 exist, forge PR comments and status checks) with a
// crafted payload.
func VerifySignature(secret string, body []byte, signatureHeader string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	return hmac.Equal(got, want)
}

// PullRequestEvent holds the fields of a GitHub pull_request webhook payload
// that the server needs to check out the PR's code and report back on it.
// (https://docs.github.com/en/webhooks/webhook-events-and-payloads#pull_request)
type PullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"` // "owner/repo"
		CloneURL string `json:"clone_url"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// ShouldRun reports whether this event's action warrants a benchmark run.
func (e PullRequestEvent) ShouldRun() bool { return RunActions[e.Action] }

// ParsePullRequestEvent decodes a pull_request webhook payload. Call this
// only after VerifySignature has passed — this function does not check
// authenticity, only shape.
func ParsePullRequestEvent(body []byte) (*PullRequestEvent, error) {
	var e PullRequestEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("invalid pull_request payload: %w", err)
	}
	if e.Repository.FullName == "" || e.PullRequest.Head.SHA == "" {
		return nil, fmt.Errorf("pull_request payload missing repository.full_name or pull_request.head.sha")
	}
	return &e, nil
}
