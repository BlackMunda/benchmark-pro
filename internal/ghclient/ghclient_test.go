package ghclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSetStatusRequestShape checks the request SetStatus would send is
// correctly shaped (method, path, headers, body) by hitting a local server
// through the same http.Client machinery, bypassing the hardcoded apiBase.
func TestSetStatusRequestShape(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotAccept string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{Token: "test-token", HTTPClient: srv.Client()}
	err := c.do(http.MethodPost, srv.URL+"/repos/me/repo/statuses/abc123", map[string]string{
		"state": "success", "description": "4 queries ran successfully", "context": "benchmark-pro",
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method: want POST, got %s", gotMethod)
	}
	if gotPath != "/repos/me/repo/statuses/abc123" {
		t.Errorf("path: got %s", gotPath)
	}
	if gotAuth != "token test-token" {
		t.Errorf("Authorization header: got %q", gotAuth)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("Accept header: got %q", gotAccept)
	}
	if gotBody["state"] != "success" || gotBody["context"] != "benchmark-pro" {
		t.Errorf("body not sent correctly: %+v", gotBody)
	}
}

func TestDoReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	c := &Client{Token: "bad-token", HTTPClient: srv.Client()}
	err := c.do(http.MethodPost, srv.URL+"/anything", nil)
	if err == nil {
		t.Fatal("expected an error for a 403 response")
	}
}
