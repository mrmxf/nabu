// Command worker is the minimal home agent server for the Phase 3 health slice.
// It exposes only GET /healthz over plain HTTP so nabu's prober (and the
// serverstatus widget behind it) can be proven end-to-end before any real job
// runs. Job dispatch endpoints (/v1/jobs/*) are the next slice.
//
// It binds to a tailnet-reachable address in production (no public port); for
// the local test it listens on 127.0.0.1. Run it with:
//
//	go run ./cmd/nabu-worker            # :8091 on localhost
//	WORKER_ADDR=:8091 go run ./cmd/nabu-worker
//
// then point nabu at it with WORKER_HEALTH_URL=http://127.0.0.1:8091/healthz.
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mrmxf/util/buildinfo"
)

// maxWorkerTTL bounds a job when nabu sends no ttl_s (0). A safety cap so the
// worker never runs a job truly unbounded even if the control plane omits a TTL.
const maxWorkerTTL = 120 * time.Second

// EnvDispatchToken is the shared bearer secret the worker requires on the job
// dispatch endpoint. Sourced from Infisical and injected as ENV on both nabu and
// the worker (same value). This is app-layer defense-in-depth on top of the
// Tailscale ACL — see docs and the security-machine-auth decision.
//
// If unset, dispatch runs OPEN (local dev only) and the worker warns loudly. Any
// non-local deployment MUST set it.
const EnvDispatchToken = "WORKER_DISPATCH_TOKEN"

// version is the worker build version reported in /healthz: the git-derived
// tag stamped at build time (clog's bc-golang via -ldflags), so a deployed
// worker says which release it is.
var version = buildinfo.Info().Tag

// agents lists the tools this worker advertises. nabu's presence test asserts
// every agent a live lesson dispatches appears here. Static for the health slice;
// a real worker would derive it from loaded adapters.
var agents = []string{"web_search", "echo"}

// capacity is the global max concurrent jobs this worker will run. A backstop to
// nabu's per-group quotas; unused until dispatch lands, but reported now so the
// dashboard header is complete.
const capacity = 4

// health is the /healthz response shape. It mirrors agentsvc.Health's
// worker-reported fields (nabu adds alive/reason/checked_at itself).
type health struct {
	Alive      bool     `json:"alive"`
	UptimeS    int      `json:"uptime_s"`
	Version    string   `json:"version"`
	Capacity   int      `json:"capacity"`
	Running    int      `json:"running"`
	QueueDepth int      `json:"queue_depth"`
	Agents     []string `json:"agents"`
}

func main() {
	addr := flag.String("addr", envOr("WORKER_ADDR", "127.0.0.1:8091"), "listen address")
	flag.Parse()

	start := time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		h := health{
			Alive:      true,
			UptimeS:    int(time.Since(start).Seconds()),
			Version:    version,
			Capacity:   capacity,
			Running:    0,
			QueueDepth: 0,
			Agents:     agents,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(h); err != nil {
			slog.Error("encode healthz response", "err", err)
		}
	})

	token := os.Getenv(EnvDispatchToken)
	if token == "" {
		slog.Warn("dispatch auth DISABLED: " + EnvDispatchToken + " unset — local dev only, set it in any deployment")
	} else {
		slog.Info("dispatch auth enabled")
	}

	// POST /v1/jobs/dispatch — run one job synchronously and return its result.
	// Guarded by the bearer token (defense-in-depth over the Tailscale ACL). The
	// agents here are canned so the running framework works end-to-end before real
	// model runtimes land; the shapes are realistic so the UI is honest.
	mux.HandleFunc("POST /v1/jobs/dispatch", requireToken(token, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JobID  string         `json:"job_id"`
			Agent  string         `json:"agent"`
			Inputs map[string]any `json:"inputs"`
			TTLSec int            `json:"ttl_s"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		slog.Info("job dispatch", "job", req.JobID, "agent", req.Agent, "ttl_s", req.TTLSec)

		// enforce the relative TTL on our own clock; self-abort if the job overruns.
		ttl := time.Duration(req.TTLSec) * time.Second
		if ttl <= 0 {
			ttl = maxWorkerTTL
		}
		ctx, cancel := context.WithTimeout(r.Context(), ttl)
		defer cancel()

		res := struct {
			ResultHTML string `json:"result_html"`
			Error      string `json:"error,omitempty"`
		}{}
		html, err := runAgent(ctx, req.Agent, req.Inputs)
		if err != nil {
			res.Error = err.Error()
			slog.Warn("job aborted", "job", req.JobID, "err", err)
		} else {
			res.ResultHTML = html
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			slog.Error("encode dispatch response", "err", err)
		}
	}))

	slog.Info("agent worker starting", "addr", *addr, "version", version, "agents", agents)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("worker stopped", "err", err)
		os.Exit(1)
	}
}

// requireToken wraps a handler with bearer-token auth. When token is empty the
// handler is returned unguarded (local dev). The comparison is constant-time to
// avoid leaking the secret through timing. The token value is never logged.
func requireToken(token string, h http.HandlerFunc) http.HandlerFunc {
	if token == "" {
		return h
	}
	want := "Bearer " + token
	return func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			slog.Warn("dispatch rejected: bad or missing token", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// runAgent produces a canned-but-realistic result for one agent, respecting the
// context deadline (so a real long-running model would self-abort on TTL).
// Deterministic (no model yet) so the running framework is predictable during
// user testing; swap the bodies for real adapters later without changing the
// contract. inputs["simulate_ms"] forces a longer run, for exercising timeouts.
func runAgent(ctx context.Context, agent string, inputs map[string]any) (string, error) {
	work := 1200 * time.Millisecond
	if ms, ok := toInt(inputs["simulate_ms"]); ok && ms > 0 {
		work = time.Duration(ms) * time.Millisecond
	}
	select {
	case <-time.After(work):
		// completed within the deadline.
	case <-ctx.Done():
		return "", fmt.Errorf("timeout: %w", ctx.Err())
	}

	query := str(inputs["query"])
	switch agent {
	case "web_search":
		q := html.EscapeString(query)
		return fmt.Sprintf(`<p><strong>web_search searched for:</strong> %s</p>`+
			`<table><thead><tr><th>Fact</th><th>Value</th><th>Source</th></tr></thead><tbody>`+
			`<tr><td>Result A</td><td>illustrative value</td><td>source-a.nabu.example</td></tr>`+
			`<tr><td>Result B</td><td>illustrative value</td><td>source-b.nabu.example</td></tr>`+
			`<tr><td>Result C</td><td>illustrative value</td><td>source-c.nabu.example</td></tr>`+
			`</tbody></table>`+
			`<p><em>Canned worker result — a real run cites live sources.</em></p>`, q), nil
	default:
		return fmt.Sprintf(`<p><strong>%s</strong> ran on: %s</p><p><em>Canned worker result.</em></p>`,
			html.EscapeString(agent), html.EscapeString(query)), nil
	}
}

// toInt coerces a JSON number/string to an int (JSON numbers decode as float64).
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	default:
		return 0, false
	}
}

// str coerces an inputs value to a string for display.
func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
