package admin

import (
	"testing"

	"github.com/mrmxf/nabu/internal/content"
)

func TestDeriveNeeds(t *testing.T) {
	courses := []content.Course{
		{Lessons: []content.Lesson{
			{Bucket: "beta"}, // bucket only
			{Bucket: "alpha", Slides: []content.Slide{{Kind: "agent"}}}, // bucket + agent slide
			{Bucket: "beta"}, // duplicate bucket
			{Slides: []content.Slide{{AgentJobs: []content.AgentJob{{ID: "j1"}}}}}, // agent via job
			{}, // nothing
		}},
	}
	n := deriveNeeds(courses)

	// Distinct + sorted.
	if got, want := len(n.buckets), 2; got != want {
		t.Fatalf("buckets=%v, want %d distinct", n.buckets, want)
	}
	if n.buckets[0] != "alpha" || n.buckets[1] != "beta" {
		t.Fatalf("buckets not sorted/distinct: %v", n.buckets)
	}
	if !n.agents {
		t.Fatal("agents should be true (agent slide + agent job present)")
	}
}

func TestDeriveNeedsNoAgentsNoBuckets(t *testing.T) {
	courses := []content.Course{{Lessons: []content.Lesson{
		{Slides: []content.Slide{{Kind: "presentation"}}},
	}}}
	n := deriveNeeds(courses)
	if len(n.buckets) != 0 {
		t.Fatalf("expected no buckets, got %v", n.buckets)
	}
	if n.agents {
		t.Fatal("expected agents=false")
	}
}

func TestEnvSet(t *testing.T) {
	if envSet("") {
		t.Fatal("empty name must be false")
	}
	t.Setenv("NABU_TEST_ENV_PRESENT", "value")
	if !envSet("NABU_TEST_ENV_PRESENT") {
		t.Fatal("set var must be true")
	}
	t.Setenv("NABU_TEST_ENV_EMPTY", "")
	if envSet("NABU_TEST_ENV_EMPTY") {
		t.Fatal("empty-value var must be false")
	}
	if envSet("NABU_TEST_ENV_ABSENT") {
		t.Fatal("absent var must be false")
	}
}

func TestMaskSecret(t *testing.T) {
	// Never reveals the value — only presence.
	if got := maskSecret("hunter2"); got != "«set»" {
		t.Fatalf("maskSecret(non-empty)=%q, want «set»", got)
	}
	if got := maskSecret(""); got != "«empty»" {
		t.Fatalf("maskSecret(empty)=%q, want «empty»", got)
	}
	if got := maskSecret("s3-secret-key"); got == "s3-secret-key" {
		t.Fatal("maskSecret leaked the value")
	}
}

func TestEnvValue(t *testing.T) {
	if envValue("") != "" {
		t.Fatal("empty name must yield empty value")
	}
	t.Setenv("NABU_TEST_SETTINGS_PW", "topsecret")
	if envValue("NABU_TEST_SETTINGS_PW") != "topsecret" {
		t.Fatal("envValue should read the named var")
	}
	if envValue("NABU_TEST_SETTINGS_ABSENT") != "" {
		t.Fatal("absent var must yield empty")
	}
}

func TestNameOr(t *testing.T) {
	if nameOr("", "fallback") != "fallback" {
		t.Fatal("empty name must fall back")
	}
	if nameOr("real", "fallback") != "real" {
		t.Fatal("non-empty name must win")
	}
}

func TestValidatorWorstTracking(t *testing.T) {
	v := &validator{quiet: true} // quiet so the test doesn't emit log noise
	if v.failed() {
		t.Fatal("fresh validator must not be failed")
	}
	v.record(sevPass, "a")
	v.record(sevWarn, "b")
	if v.failed() {
		t.Fatal("warn alone must not fail")
	}
	v.record(sevFail, "c")
	v.record(sevPass, "d") // a later pass must not clear the failure
	if !v.failed() {
		t.Fatal("a FAIL must make failed() true and stick")
	}
}
