// Package agentsvc is nabu's client half of the agent-server contract: it probes
// the worker's /healthz (on the tailnet, or a public https host such as a
// Cloudflare or Scaleway worker), caches the result, and exposes it
// to the rest of nabu (the /api/agent/health route, the server-offline banner,
// and the teacher queue dashboard).
//
// Division of responsibility: the worker is
// identity-free and only reports capacity; nabu owns identity, jobs, and quotas.
// This package is the read-only health side of that split — no job state lives
// here.
package agentsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// EnvWorkerHealthURL names the env var holding the worker's health endpoint,
// e.g. http://127.0.0.1:8091/healthz (local test),
// http://sa-worker:8091/healthz (tailnet) or
// https://worker.nabu.example/healthz (a public cloud host; see checkTransport). Unset => no worker
// configured, and Health reports Alive=false with a clear reason.
const EnvWorkerHealthURL = "WORKER_HEALTH_URL"

// EnvWorkerDispatchURL names the worker's job-dispatch endpoint. If unset it is
// derived from the health URL (…/healthz -> …/v1/jobs/dispatch), so the common
// case only needs WORKER_HEALTH_URL set.
const EnvWorkerDispatchURL = "WORKER_DISPATCH_URL"

// EnvDispatchToken is the shared bearer secret sent on job dispatch, matching the
// worker's WORKER_DISPATCH_TOKEN. Sourced from Infisical; app-layer defense in
// depth over the Tailscale ACL. Empty => no header sent (local dev).
const EnvDispatchToken = "WORKER_DISPATCH_TOKEN"

// EnvAccessClientID and EnvAccessClientSecret name an optional Cloudflare Access
// service token. When both are set, every probe and dispatch carries them as
// CF-Access-Client-Id / CF-Access-Client-Secret, so a worker published behind
// Cloudflare Access (e.g. through a Cloudflare Tunnel) admits nabu.
const (
	EnvAccessClientID     = "WORKER_ACCESS_CLIENT_ID"
	EnvAccessClientSecret = "WORKER_ACCESS_CLIENT_SECRET"
)

// EnvProbeTimeout and EnvDispatchTimeout override the probe and dispatch
// timeouts, as Go durations (e.g. "8s", "75s").
const (
	EnvProbeTimeout    = "WORKER_PROBE_TIMEOUT"
	EnvDispatchTimeout = "WORKER_DISPATCH_TIMEOUT"
)

// dispatchTimeout caps one job run by default. Generous vs the health probe: a
// real agent may take seconds. A hung job fails rather than blocking forever.
// Kept under Cloudflare's 100s edge timeout so a worker behind Cloudflare fails
// with nabu's error rather than an opaque 524.
const dispatchTimeout = 90 * time.Second

// cacheTTL bounds how stale a served health result may be. Short enough that the
// offline banner clears quickly, long enough that a roomful of students polling
// doesn't hammer the worker.
const cacheTTL = 3 * time.Second

// probeTimeout caps a single /healthz request by default so a hung worker can't
// stall nabu. Long enough for a scale-to-zero worker host to answer from a cold
// start without being reported offline.
const probeTimeout = 5 * time.Second

// Health is nabu's view of the worker. The worker-reported fields mirror the
// worker's /healthz JSON; Alive/Reason/CheckedAt are stamped by nabu.
type Health struct {
	Alive      bool     `json:"alive"`
	UptimeS    int      `json:"uptime_s"`
	Version    string   `json:"version"`
	Capacity   int      `json:"capacity"`
	Running    int      `json:"running"`
	QueueDepth int      `json:"queue_depth"`
	Agents     []string `json:"agents"`
	// Reason is a human-readable explanation when Alive is false (unreachable,
	// not configured, bad response). Empty when healthy.
	Reason string `json:"reason,omitempty"`
	// CheckedAt is when nabu last probed; lets the UI show freshness.
	CheckedAt time.Time `json:"checked_at"`
}

// Prober probes and caches worker health, and dispatches jobs to the worker.
// Safe for concurrent use.
type Prober struct {
	url            string
	dispatchURL    string
	dispatchToken  string       // bearer secret for dispatch (never logged)
	accessID       string       // optional Cloudflare Access client id
	accessSecret   string       // optional Cloudflare Access client secret (never logged)
	client         *http.Client // short timeout, health probes
	dispatchClient *http.Client // long timeout, job runs

	// probeErr / dispatchErr record a transport-policy refusal found at
	// construction (see checkTransport); while set, that call never goes out.
	probeErr    error
	dispatchErr error

	mu     sync.Mutex
	cached Health
	at     time.Time
}

// New builds a Prober reading the worker URLs from the environment. The dispatch
// URL defaults to the health URL with /healthz swapped for /v1/jobs/dispatch.
func New() *Prober {
	health := os.Getenv(EnvWorkerHealthURL)
	dispatch := os.Getenv(EnvWorkerDispatchURL)
	if dispatch == "" && health != "" {
		dispatch = strings.TrimSuffix(health, "/healthz") + "/v1/jobs/dispatch"
	}
	p := &Prober{
		url:            health,
		dispatchURL:    dispatch,
		dispatchToken:  os.Getenv(EnvDispatchToken),
		accessID:       os.Getenv(EnvAccessClientID),
		accessSecret:   os.Getenv(EnvAccessClientSecret),
		client:         &http.Client{Timeout: envDuration(EnvProbeTimeout, probeTimeout)},
		dispatchClient: &http.Client{Timeout: envDuration(EnvDispatchTimeout, dispatchTimeout)},
	}
	if health != "" {
		if p.probeErr = checkTransport(health, false, ""); p.probeErr != nil {
			slog.Error("refusing to probe agent worker", "url", health, "err", p.probeErr,
				"fix", "serve the worker over https, or reach it over the tailnet/loopback")
		}
	}
	if dispatch != "" {
		if p.dispatchErr = checkTransport(dispatch, true, p.dispatchToken); p.dispatchErr != nil {
			slog.Error("refusing to dispatch to agent worker", "url", dispatch, "err", p.dispatchErr,
				"fix", "serve the worker over https and set "+EnvDispatchToken+" on both nabu and the worker")
		}
	}
	return p
}

// envDuration reads a Go duration from the named env var, falling back to dflt
// (with a warning) when it is unset, unparseable or not positive.
func envDuration(name string, dflt time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return dflt
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		slog.Warn("ignoring bad worker timeout; using default", "env", name, "value", v, "default", dflt)
		return dflt
	}
	return d
}

// tailnetCGNAT is the 100.64.0.0/10 range Tailscale assigns node addresses from.
var tailnetCGNAT = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// isPrivateHost reports whether host is reachable only on a private path, where
// plain http and an open dispatch are acceptable: loopback, a tailnet address or
// MagicDNS name (*.ts.net), an RFC 1918 / link-local LAN address, or a
// single-label name (a MagicDNS short name or a compose service, e.g.
// "sa-worker").
func isPrivateHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".ts.net") || !strings.Contains(host, ".") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || tailnetCGNAT.Contains(ip)
}

// checkTransport enforces the worker transport policy for one URL. A private
// host (see isPrivateHost) may use plain http. A public host — a cloud worker on
// Cloudflare or Scaleway — must use https, and a dispatch to it (needToken) must
// also carry a bearer token, since anyone on the internet can reach it.
func checkTransport(rawURL string, needToken bool, token string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("bad worker URL %q", rawURL)
	}
	if isPrivateHost(u.Hostname()) {
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf("public worker host %q must use https, not %s", u.Hostname(), u.Scheme)
	}
	if needToken && token == "" {
		return fmt.Errorf("public worker host %q needs %s set", u.Hostname(), EnvDispatchToken)
	}
	return nil
}

// setAccessHeaders adds the Cloudflare Access service-token headers when both
// halves are configured.
func (p *Prober) setAccessHeaders(r *http.Request) {
	if p.accessID != "" && p.accessSecret != "" {
		r.Header.Set("CF-Access-Client-Id", p.accessID)
		r.Header.Set("CF-Access-Client-Secret", p.accessSecret)
	}
}

// WorkerLabel is a short identifier for the worker nabu dispatches to (the host
// of the dispatch URL), recorded on each job so the dashboard shows which worker
// ran it. "local" when unconfigured.
func (p *Prober) WorkerLabel() string {
	if p.dispatchURL == "" {
		return "local"
	}
	if u, err := url.Parse(p.dispatchURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "worker"
}

// DispatchRequest is the payload nabu sends to the worker to run one job. TTLSec
// is a RELATIVE deadline (seconds): the worker enforces it on its own monotonic
// clock from receipt, so there is no cross-host clock dependency. 0 => the worker
// applies its own default cap.
type DispatchRequest struct {
	JobID  string         `json:"job_id"`
	Agent  string         `json:"agent"`
	Inputs map[string]any `json:"inputs"`
	TTLSec int            `json:"ttl_s"`
}

// DispatchResult is the worker's synchronous response for a completed job.
type DispatchResult struct {
	ResultHTML string `json:"result_html"`
	Error      string `json:"error,omitempty"`
}

// Dispatch runs one job on the worker and returns its result. A transport error,
// non-200, or worker-reported error is returned as an error so the caller can
// mark the job failed.
func (p *Prober) Dispatch(ctx context.Context, req DispatchRequest) (DispatchResult, error) {
	if p.dispatchURL == "" {
		return DispatchResult{}, fmt.Errorf("no worker configured (%s / %s unset)", EnvWorkerHealthURL, EnvWorkerDispatchURL)
	}
	if p.dispatchErr != nil {
		return DispatchResult{}, fmt.Errorf("dispatch refused: %w", p.dispatchErr)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("marshal dispatch: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.dispatchURL, bytes.NewReader(body))
	if err != nil {
		return DispatchResult{}, fmt.Errorf("build dispatch request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.dispatchToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.dispatchToken)
	}
	p.setAccessHeaders(httpReq)

	resp, err := p.dispatchClient.Do(httpReq)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("dispatch to worker: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return DispatchResult{}, fmt.Errorf("worker returned %d", resp.StatusCode)
	}
	var res DispatchResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return DispatchResult{}, fmt.Errorf("decode dispatch result: %w", err)
	}
	if res.Error != "" {
		return res, fmt.Errorf("worker error: %s", res.Error)
	}
	return res, nil
}

// Health returns the cached worker health, refreshing it if the cache has
// expired. It never returns an error: an unreachable or unconfigured worker is a
// valid, reportable state (Alive=false), not a failure of this call.
func (p *Prober) Health(ctx context.Context) Health {
	p.mu.Lock()
	if time.Since(p.at) < cacheTTL && !p.at.IsZero() {
		h := p.cached
		p.mu.Unlock()
		return h
	}
	p.mu.Unlock()

	h := p.probe(ctx)

	p.mu.Lock()
	p.cached = h
	p.at = time.Now()
	p.mu.Unlock()
	return h
}

// probe performs one live /healthz request and maps the outcome to a Health.
func (p *Prober) probe(ctx context.Context) Health {
	now := time.Now()
	if p.url == "" {
		return Health{Alive: false, Reason: "no worker configured (" + EnvWorkerHealthURL + " unset)", CheckedAt: now}
	}

	if p.probeErr != nil {
		return Health{Alive: false, Reason: p.probeErr.Error(), CheckedAt: now}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return Health{Alive: false, Reason: fmt.Sprintf("bad worker URL: %v", err), CheckedAt: now}
	}
	p.setAccessHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		// Expected + non-fatal while the worker is down; Warn per the logging guide.
		slog.Warn("agent worker unreachable", "url", p.url, "err", err)
		return Health{Alive: false, Reason: "worker unreachable", CheckedAt: now}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Health{Alive: false, Reason: fmt.Sprintf("worker returned %d", resp.StatusCode), CheckedAt: now}
	}

	var h Health
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return Health{Alive: false, Reason: fmt.Sprintf("bad worker response: %v", err), CheckedAt: now}
	}
	h.Alive = true
	h.Reason = ""
	h.CheckedAt = now
	return h
}
