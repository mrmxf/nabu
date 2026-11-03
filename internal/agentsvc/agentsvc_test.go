package agentsvc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProbeAlive: a worker serving a healthy /healthz is reported Alive with its
// fields mapped through.
func TestProbeAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uptime_s":42,"version":"0.1.0","capacity":4,"running":1,"queue_depth":2,"agents":["echo","web_search"]}`))
	}))
	defer srv.Close()

	p := &Prober{url: srv.URL, client: srv.Client()}
	h := p.Health(context.Background())
	if !h.Alive {
		t.Fatalf("want Alive, got reason %q", h.Reason)
	}
	if h.Version != "0.1.0" || h.Capacity != 4 || h.QueueDepth != 2 {
		t.Errorf("fields not mapped: %+v", h)
	}
	if len(h.Agents) != 2 || h.Agents[0] != "echo" {
		t.Errorf("agents not mapped: %v", h.Agents)
	}
}

// TestProbeUnreachable: a worker that isn't listening is a valid offline state,
// not an error.
func TestProbeUnreachable(t *testing.T) {
	// a closed server's URL is guaranteed unreachable.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	p := &Prober{url: url, client: &http.Client{}}
	h := p.Health(context.Background())
	if h.Alive {
		t.Error("closed worker should be offline")
	}
	if h.Reason == "" {
		t.Error("offline health should carry a reason")
	}
}

// TestProbeUnconfigured: no WORKER_HEALTH_URL => offline with a clear reason,
// never a panic or error.
func TestProbeUnconfigured(t *testing.T) {
	p := &Prober{url: "", client: &http.Client{}}
	h := p.Health(context.Background())
	if h.Alive {
		t.Error("unconfigured worker should be offline")
	}
	if h.Reason == "" {
		t.Error("unconfigured health should explain why")
	}
}

// TestProbeBadStatus: a non-200 is offline, reported with the code.
func TestProbeBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := &Prober{url: srv.URL, client: srv.Client()}
	if h := p.Health(context.Background()); h.Alive {
		t.Error("503 worker should be offline")
	}
}
