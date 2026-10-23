package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignatureAccepts(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	if !VerifySignature("s3cret", body, sign("s3cret", body)) {
		t.Fatal("correct signature was rejected")
	}
}

func TestVerifySignatureRejectsWrongSecret(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	if VerifySignature("s3cret", body, sign("wrong-secret", body)) {
		t.Fatal("signature from the wrong secret was accepted")
	}
}

func TestVerifySignatureRejectsTamperedBody(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	sig := sign("s3cret", body)
	tampered := []byte(`{"action":"closed"}`)
	if VerifySignature("s3cret", tampered, sig) {
		t.Fatal("tampered body was accepted with the original signature")
	}
}

func TestVerifySignatureRejectsMalformedHeader(t *testing.T) {
	body := []byte(`{}`)
	cases := []string{"", "not-hex", "sha1=abcd", "sha256="}
	for _, h := range cases {
		if VerifySignature("s3cret", body, h) {
			t.Fatalf("malformed header %q was accepted", h)
		}
	}
}

const samplePayload = `{
  "action": "opened",
  "number": 42,
  "pull_request": {"head": {"sha": "abc123", "ref": "feature-branch"}},
  "repository": {
    "name": "myrepo",
    "full_name": "me/myrepo",
    "clone_url": "https://github.com/me/myrepo.git",
    "owner": {"login": "me"}
  }
}`

func TestParsePullRequestEvent(t *testing.T) {
	e, err := ParsePullRequestEvent([]byte(samplePayload))
	if err != nil {
		t.Fatal(err)
	}
	if e.Number != 42 || e.PullRequest.Head.SHA != "abc123" || e.Repository.FullName != "me/myrepo" {
		t.Fatalf("fields not parsed correctly: %+v", e)
	}
	if !e.ShouldRun() {
		t.Fatal("action=opened should trigger a run")
	}
}

func TestShouldRunByAction(t *testing.T) {
	cases := map[string]bool{
		"opened": true, "synchronize": true, "reopened": true,
		"closed": false, "labeled": false, "edited": false,
	}
	for action, want := range cases {
		e := PullRequestEvent{Action: action}
		if got := e.ShouldRun(); got != want {
			t.Errorf("action %q: want ShouldRun=%v, got %v", action, want, got)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := ParsePullRequestEvent([]byte("not json")); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestParseRejectsMissingFields(t *testing.T) {
	if _, err := ParsePullRequestEvent([]byte(`{"action":"opened"}`)); err == nil {
		t.Fatal("expected an error when repository/head sha are missing")
	}
}
