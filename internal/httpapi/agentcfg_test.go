package httpapi

import (
	"testing"

	"github.com/mrmxf/nabu/internal/content"
)

func f64(v float64) *float64 { return &v }

// TestEffectiveInputs: the server rebuilds inputs from the authored config under
// policy — fixed/hidden keep the authored value (client ignored), group overrides
// apply, group_range clamps, and unresolved template tokens are stripped.
func TestEffectiveInputs(t *testing.T) {
	job := &content.AgentJob{
		ID:    "hunter",
		Agent: "web_search",
		Inputs: map[string]any{
			"query":         "{{choice:agent-topic}} — key facts",
			"max_results":   float64(5),
			"recency_days":  float64(365),
			"region":        "gb",
			"cite_sources":  true,
			"domains_allow": []any{"reuters.com", "bbc.co.uk"},
		},
		ConfigPolicy: map[string]content.InputPolicy{
			"query":              {Policy: "template"},
			"max_results":        {Policy: "group_range", Min: f64(1), Max: f64(10)},
			"recency_days":       {Policy: "group_range", Min: f64(7), Max: f64(3650)},
			"cite_sources":       {Policy: "group"},
			"additional_prompts": {Policy: "group"},
			"region":             {Policy: "fixed"},
			"domains_allow":      {Policy: "hidden"},
		},
	}

	client := map[string]any{
		"query":              "dinosaurs — key facts", // client resolved the token
		"max_results":        float64(99),             // over max → clamp to 10
		"recency_days":       float64(3),              // under min → clamp to 7
		"cite_sources":       false,                   // group override
		"additional_prompts": "focus on UK sources",   // new group key
		"region":             "us",                    // fixed → must be IGNORED
		"domains_allow":      []any{"evil.example"},    // hidden → must be IGNORED
	}

	out := effectiveInputs(job, client)

	if out["query"] != "dinosaurs — key facts" {
		t.Errorf("query = %v, want resolved client value", out["query"])
	}
	if out["max_results"] != float64(10) {
		t.Errorf("max_results = %v, want 10 (clamped)", out["max_results"])
	}
	if out["recency_days"] != float64(7) {
		t.Errorf("recency_days = %v, want 7 (clamped)", out["recency_days"])
	}
	if out["cite_sources"] != false {
		t.Errorf("cite_sources = %v, want false (group override)", out["cite_sources"])
	}
	if out["additional_prompts"] != "focus on UK sources" {
		t.Errorf("additional_prompts = %v, want the group value", out["additional_prompts"])
	}
	if out["region"] != "gb" {
		t.Errorf("region = %v, want gb (fixed; client ignored)", out["region"])
	}
	if got, ok := out["domains_allow"].([]any); !ok || len(got) != 2 {
		t.Errorf("domains_allow = %v, want the authored 2-item allowlist (hidden; client ignored)", out["domains_allow"])
	}
}

// TestEffectiveInputsStripsUnresolvedToken: if the client fails to resolve a
// template token, the server strips it so no literal {{…}} reaches the worker.
func TestEffectiveInputsStripsUnresolvedToken(t *testing.T) {
	job := &content.AgentJob{
		Inputs:       map[string]any{"query": "{{choice:agent-topic}} — facts"},
		ConfigPolicy: map[string]content.InputPolicy{"query": {Policy: "template"}},
	}
	out := effectiveInputs(job, map[string]any{}) // client sent nothing
	if got := out["query"].(string); got != " — facts" {
		t.Errorf("query = %q, want the token stripped", got)
	}
}

// TestEffectiveInputsGroupOptions: a `group` key with Options is a closed choice —
// an allowed value is accepted, anything else keeps the authored default.
func TestEffectiveInputsGroupOptions(t *testing.T) {
	job := &content.AgentJob{
		Inputs: map[string]any{"orientation": "landscape"},
		ConfigPolicy: map[string]content.InputPolicy{
			"orientation": {Policy: "group", Options: []string{"landscape", "portrait"}},
		},
	}
	// allowed value → applied.
	if out := effectiveInputs(job, map[string]any{"orientation": "portrait"}); out["orientation"] != "portrait" {
		t.Errorf("orientation = %v, want portrait (allowed option)", out["orientation"])
	}
	// out-of-set value → authored default kept.
	if out := effectiveInputs(job, map[string]any{"orientation": "square"}); out["orientation"] != "landscape" {
		t.Errorf("orientation = %v, want landscape (bad option ignored)", out["orientation"])
	}
	// non-string → authored default kept.
	if out := effectiveInputs(job, map[string]any{"orientation": 3}); out["orientation"] != "landscape" {
		t.Errorf("orientation = %v, want landscape (non-string ignored)", out["orientation"])
	}
}

// TestIsImageJob: only pixie in format=image draws on the image budget.
func TestIsImageJob(t *testing.T) {
	cases := []struct {
		agent  string
		inputs map[string]any
		want   bool
	}{
		// The authored format decides, whatever the agent is called (the SPA
		// applies the same rule).
		{"painter", map[string]any{"format": "image"}, true},
		{"painter", map[string]any{"format": "script"}, false},
		{"painter", map[string]any{}, false},
		{"search", map[string]any{"format": "image"}, true},
		{"search", map[string]any{"format": 3}, false},
	}
	for _, c := range cases {
		if got := isImageJob(c.agent, c.inputs); got != c.want {
			t.Errorf("isImageJob(%q, %v) = %v, want %v", c.agent, c.inputs, got, c.want)
		}
	}
}

// TestClampNumber: bounds are applied only when set; non-numbers pass through.
func TestClampNumber(t *testing.T) {
	if got := clampNumber(float64(99), f64(1), f64(10)); got != float64(10) {
		t.Errorf("clamp high = %v, want 10", got)
	}
	if got := clampNumber(float64(0), f64(1), f64(10)); got != float64(1) {
		t.Errorf("clamp low = %v, want 1", got)
	}
	if got := clampNumber(float64(5), nil, nil); got != float64(5) {
		t.Errorf("no bounds = %v, want 5", got)
	}
	if got := clampNumber("nope", f64(1), f64(10)); got != "nope" {
		t.Errorf("non-number = %v, want unchanged", got)
	}
}
