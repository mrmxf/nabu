package admin

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mrmxf/nabu/internal/agentsvc"
	"github.com/mrmxf/nabu/internal/classroom"
)

// capture swaps the default logger for one writing to a buffer.
func capture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestDoctorIsSilentWhenReady(t *testing.T) {
	buf := capture(t)
	v := &validator{hidePass: true}
	v.record(sevPass, "all good")
	v.summary()
	if buf.Len() != 0 {
		t.Errorf("a passing doctor printed:\n%s", buf)
	}
	v.record(sevWarn, "look at this")
	if !strings.Contains(buf.String(), "look at this") {
		t.Error("doctor hid a warning")
	}
	v.record(sevFail, "broken")
	v.record(sevFail, "broken again")
	if v.fails != 2 || !v.failed() {
		t.Errorf("fails = %d, failed = %v", v.fails, v.failed())
	}
}

func TestCheckStaticBuild(t *testing.T) {
	dir := t.TempDir()
	v := &validator{}
	checkStaticBuild(v, dir)
	if !v.failed() {
		t.Error("an empty folder passed as a static build")
	}
	for _, f := range []string{"index.html", "content/index.json", "_headers"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v = &validator{}
	checkStaticBuild(v, dir)
	if v.worst != sevPass {
		t.Errorf("a full static build scored %d", v.worst)
	}
}

func TestCheckSaaSWorker(t *testing.T) {
	for _, tc := range []struct {
		name, health, dispatch, token string
		want                          severity
	}{
		{"none configured", "", "", "", sevFail},
		{"plain http", "", "http://worker.nabu.example/v1/jobs/dispatch", "t", sevFail},
		{"https without token", "https://worker.nabu.example/healthz", "", "", sevFail},
		{"https with token", "https://worker.nabu.example/healthz", "", "t", sevPass},
	} {
		t.Setenv(agentsvc.EnvWorkerHealthURL, tc.health)
		t.Setenv(agentsvc.EnvWorkerDispatchURL, tc.dispatch)
		t.Setenv(agentsvc.EnvDispatchToken, tc.token)
		v := &validator{quiet: true}
		checkSaaSWorker(v, needs{agents: true})
		if v.worst != tc.want {
			t.Errorf("%s: worst = %d, want %d", tc.name, v.worst, tc.want)
		}
	}
	v := &validator{quiet: true}
	checkSaaSWorker(v, needs{})
	if v.worst != sevPass {
		t.Error("content with no agents needs no worker")
	}
}

func TestCheckWebBuiltIn(t *testing.T) {
	v := &validator{quiet: true}
	checkWebBuiltIn(v, fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}})
	if v.worst != sevPass {
		t.Error("an embedded web app did not pass")
	}
	v = &validator{quiet: true}
	checkWebBuiltIn(v, nil)
	if v.worst != sevWarn {
		t.Error("no embedded web app should warn")
	}
}

func TestCheckLANAndOffline(t *testing.T) {
	v := &validator{quiet: true}
	checkLAN(v, nil, nil)
	if !v.failed() {
		t.Error("no LAN address passed")
	}
	v = &validator{quiet: true}
	checkLAN(v, []string{"192.168.1.20"}, nil)
	if v.failed() {
		t.Error("a LAN address failed")
	}

	gaps := []classroom.Gap{{Where: "c/l/s#j", Agent: "search", Why: "no sim_output_fallback: true"}}
	v = &validator{quiet: true}
	checkOffline(v, gaps, agentsvc.Health{})
	if v.fails != 1 {
		t.Errorf("an offline gap with no worker: fails = %d, want 1", v.fails)
	}
	v = &validator{quiet: true}
	checkOffline(v, gaps, agentsvc.Health{Alive: true})
	if v.failed() {
		t.Error("a reachable worker covers the gap")
	}
}

func TestCheckImage(t *testing.T) {
	found := func(string) (string, error) { return "/usr/bin/podman", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	no := func() bool { return false }
	have := func(string, string) bool { return true }
	lack := func(string, string) bool { return false }

	for _, tc := range []struct {
		name    string
		image   string
		look    func(string) (string, error)
		inside  func() bool
		inspect func(string, string) bool
		want    severity
	}{
		{"inside a container", "", missing, func() bool { return true }, lack, sevPass},
		{"no runtime", "img", missing, no, lack, sevFail},
		{"image not named", "", found, no, lack, sevWarn},
		{"image missing", "img", found, no, lack, sevFail},
		{"image present", "img", found, no, have, sevPass},
	} {
		v := &validator{quiet: true}
		checkImage(v, tc.image, tc.look, tc.inside, tc.inspect)
		if v.worst != tc.want {
			t.Errorf("%s: worst = %d, want %d", tc.name, v.worst, tc.want)
		}
	}
}

func TestRunDoctorRejectsUnknownMode(t *testing.T) {
	err := runDoctor(t.Context(), nil, Env{}, doctorOpts{mode: "mainframe"})
	if err == nil || !strings.Contains(err.Error(), "unknown --mode") {
		t.Errorf("err = %v", err)
	}
}
