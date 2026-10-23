// Package ghclient posts results back to GitHub: a commit status (the
// ✅/❌ check on the PR) and a comment with the benchmark table. It uses a
// personal-access or GitHub App installation token via the REST API — no
// third-party SDK, just net/http, so it has no dependency beyond the
// standard library.
//
// A GitHub App's Checks API gives richer UI (collapsible details, separate
// named checks) than the commit-status API used here; swapping it in later
// only touches this package; see docs/DESIGN.md §(webhook) for the tradeoff.
package ghclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const apiBase = "https://api.github.com"

// Reporter is the interface the server depends on, so tests can substitute a
// fake instead of making real GitHub API calls.
type Reporter interface {
	SetStatus(owner, repo, sha string, status StatusState, description, context string) error
	PostComment(owner, repo string, prNumber int, body string) error
}

type StatusState string

const (
	StatusPending StatusState = "pending"
	StatusSuccess StatusState = "success"
	StatusFailure StatusState = "failure"
	StatusError   StatusState = "error"
)

// Client is the real implementation, backed by api.github.com.
type Client struct {
	Token      string
	HTTPClient *http.Client
}

func New(token string) *Client {
	return &Client{Token: token, HTTPClient: &http.Client{}}
}

func (c *Client) do(method, url string, payload any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github api %s %s: %s: %s", method, url, resp.Status, respBody)
	}
	return nil
}

// SetStatus sets the commit status shown on the PR's checks list. context
// namespaces it (so this doesn't collide with other CI checks) — GitHub
// shows one line per distinct context.
func (c *Client) SetStatus(owner, repo, sha string, status StatusState, description, context string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/statuses/%s", apiBase, owner, repo, sha)
	payload := map[string]string{
		"state":       string(status),
		"description": description,
		"context":     context,
	}
	return c.do(http.MethodPost, url, payload)
}

// PostComment adds a comment to the PR (PRs are issues in GitHub's API, hence
// the /issues/ path).
func (c *Client) PostComment(owner, repo string, prNumber int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", apiBase, owner, repo, prNumber)
	return c.do(http.MethodPost, url, map[string]string{"body": body})
}
