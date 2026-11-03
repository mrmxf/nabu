package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRunAgentRespectsDeadline: an expired context aborts the run with a timeout.
func TestRunAgentRespectsDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done
	if _, err := runAgent(ctx, "web_search", map[string]any{"query": "x"}); err == nil {
		t.Fatal("want timeout error on a cancelled context")
	}
}

// TestRunAgentCompletes: a fast job (simulate_ms) returns a result, no error.
func TestRunAgentCompletes(t *testing.T) {
	html, err := runAgent(context.Background(), "web_search",
		map[string]any{"query": "topic", "simulate_ms": float64(1)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "web_search searched") {
		t.Errorf("missing result body: %s", html)
	}
}

func ok(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

// TestRequireTokenEnforced: with a token set, only the exact bearer passes.
func TestRequireTokenEnforced(t *testing.T) {
	h := requireToken("s3cret", ok)
	cases := []struct {
		name, auth string
		want       int
	}{
		{"correct", "Bearer s3cret", http.StatusOK},
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "Bearer nope", http.StatusUnauthorized},
		{"no-scheme", "s3cret", http.StatusUnauthorized},
		{"prefix-only", "Bearer s3cre", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/v1/jobs/dispatch", nil)
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: code = %d, want %d", c.name, rec.Code, c.want)
		}
	}
}

// TestRequireTokenOpen: with no token (local dev), the handler is unguarded.
func TestRequireTokenOpen(t *testing.T) {
	h := requireToken("", ok)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/dispatch", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("open dispatch code = %d, want 200", rec.Code)
	}
}
