package agentsvc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCheckTransport covers the worker transport policy: private hosts may use
// plain http; a public host needs https, and a dispatch to it needs a token.
func TestCheckTransport(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		needToken bool
		token     string
		wantErr   string // "" => allowed
	}{
		{"loopback http", "http://127.0.0.1:8091/healthz", true, "", ""},
		{"localhost http", "http://localhost:8091/v1/jobs/dispatch", true, "", ""},
		{"tailnet ip http", "http://100.101.102.103:8091/healthz", true, "", ""},
		{"magicdns fqdn http", "http://gpu.tail1234.ts.net:8091/healthz", true, "", ""},
		{"magicdns short name http", "http://sa-worker:8091/healthz", true, "", ""},
		{"lan ip http", "http://192.168.1.20:8091/healthz", true, "", ""},
		{"public http probe", "http://worker.nabu.example/healthz", false, "", "must use https"},
		{"public http dispatch", "http://worker.nabu.example/v1/jobs/dispatch", true, "t", "must use https"},
		{"public ip http", "http://203.0.113.7:8091/healthz", false, "", "must use https"},
		{"public https probe", "https://worker.nabu.example/healthz", false, "", ""},
		{"public https dispatch no token", "https://worker.nabu.example/v1/jobs/dispatch", true, "", "WORKER_DISPATCH_TOKEN"},
		{"public https dispatch with token", "https://worker.nabu.example/v1/jobs/dispatch", true, "s3cret", ""},
		{"no host", "worker", false, "", "bad worker URL"},
	}
	for _, c := range cases {
		err := checkTransport(c.url, c.needToken, c.token)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected refusal: %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: err = %v, want containing %q", c.name, err, c.wantErr)
		}
	}
}

// TestRefusedDispatchNeverSent: a policy refusal found at construction stops the
// dispatch before any request goes out, and the probe reports it as the reason.
func TestRefusedDispatchNeverSent(t *testing.T) {
	t.Setenv(EnvWorkerHealthURL, "http://worker.nabu.example/healthz")
	t.Setenv(EnvDispatchToken, "")
	p := New()
	if _, err := p.Dispatch(context.Background(), DispatchRequest{JobID: "j1", Agent: "echo"}); err == nil ||
		!strings.Contains(err.Error(), "dispatch refused") {
		t.Fatalf("dispatch err = %v, want a refusal", err)
	}
	if h := p.Health(context.Background()); h.Alive || !strings.Contains(h.Reason, "https") {
		t.Fatalf("health = %+v, want offline with an https reason", h)
	}
}

// TestHTTPSWorkerWithAccessHeaders: against a TLS worker, the probe and the
// dispatch both carry the Cloudflare Access service-token headers, and the
// dispatch carries the bearer token.
func TestHTTPSWorkerWithAccessHeaders(t *testing.T) {
	var probed, dispatched bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("CF-Access-Client-Id") != "id-1" || r.Header.Get("CF-Access-Client-Secret") != "sec-1" {
			http.Error(w, "missing access headers", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/healthz":
			probed = true
			_, _ = w.Write([]byte(`{"version":"t","capacity":1,"agents":["echo"]}`))
		case "/v1/jobs/dispatch":
			if r.Header.Get("Authorization") != "Bearer s3cret" {
				http.Error(w, "bad token", http.StatusUnauthorized)
				return
			}
			dispatched = true
			_ = json.NewEncoder(w).Encode(DispatchResult{ResultHTML: "<p>ok</p>"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv(EnvWorkerHealthURL, srv.URL+"/healthz")
	t.Setenv(EnvDispatchToken, "s3cret")
	t.Setenv(EnvAccessClientID, "id-1")
	t.Setenv(EnvAccessClientSecret, "sec-1")
	p := New()
	// trust the test server's self-signed certificate
	p.client.Transport = srv.Client().Transport
	p.dispatchClient.Transport = srv.Client().Transport

	if h := p.Health(context.Background()); !h.Alive {
		t.Fatalf("probe not alive: %q", h.Reason)
	}
	res, err := p.Dispatch(context.Background(), DispatchRequest{JobID: "j1", Agent: "echo"})
	if err != nil || res.ResultHTML != "<p>ok</p>" {
		t.Fatalf("dispatch = %+v, %v", res, err)
	}
	if !probed || !dispatched {
		t.Fatalf("probed=%v dispatched=%v, want both", probed, dispatched)
	}
}

// TestEnvTimeouts: configured timeouts apply; bad values fall back to defaults.
func TestEnvTimeouts(t *testing.T) {
	t.Setenv(EnvWorkerHealthURL, "")
	t.Setenv(EnvProbeTimeout, "11s")
	t.Setenv(EnvDispatchTimeout, "nonsense")
	p := New()
	if p.client.Timeout.String() != "11s" {
		t.Errorf("probe timeout = %v, want 11s", p.client.Timeout)
	}
	if p.dispatchClient.Timeout != dispatchTimeout {
		t.Errorf("dispatch timeout = %v, want default %v", p.dispatchClient.Timeout, dispatchTimeout)
	}
}
