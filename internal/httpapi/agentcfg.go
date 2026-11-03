package httpapi

import (
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/content"
)

// This file is the server-side authority for an agent job's effective inputs.
// The client (LmsAgent) may propose values for group-editable config keys, but
// the server never trusts the client for anything else: it reloads the slide's
// authored t_agent config and its per-key policy, and rebuilds the input set so
// that a `fixed`/`hidden` key always keeps its authored value and a numeric
// `group_range` key is clamped to its bounds. See model.InputPolicy and the
// group collaboration plan.
//
// Note on tokens: `template` inputs (e.g. a search agent's query, "{{choice:agent-topic}}
// — …") are resolved on the client from the group's shared choices, which live
// in the group's Yjs document and are opaque to this Go server. The client sends
// the resolved string; as a guard, the server strips any token the client failed
// to resolve so a literal "{{…}}" never reaches the worker.

// tokenRe matches an unresolved {{choice:ID}} / {{field:ID}} token.
var tokenRe = regexp.MustCompile(`\{\{[^}]*\}\}`)

// authoredAgentJob loads the authored t_agent entry (defaults + policy) for a
// job on a slide. lesson is the lesson id (from the session); slideSlug and
// agentID come from the submit body. Returns nil if the slide or job is not
// found — the caller then falls back to the client-sent inputs (legacy path).
func authoredAgentJob(app core.App, lesson, slideSlug, agentID string) *content.AgentJob {
	rec, err := app.FindFirstRecordByFilter("slides",
		"lesson = {:l} && slug = {:s}",
		map[string]any{"l": lesson, "s": slideSlug})
	if err != nil || rec == nil {
		return nil
	}
	var jobs []content.AgentJob
	if err := rec.UnmarshalJSONField("t_agent", &jobs); err != nil {
		return nil
	}
	for i := range jobs {
		if jobs[i].ID == agentID {
			return &jobs[i]
		}
	}
	return nil
}

// effectiveInputs merges client-proposed values onto the authored inputs under
// the job's policy. Start from the authored inputs (so fixed/hidden keys keep
// their authored value and are still sent); overlay only group-editable keys;
// clamp ranges; strip any unresolved template token. Keys the client sends that
// are not group-editable are ignored.
func effectiveInputs(job *content.AgentJob, client map[string]any) map[string]any {
	out := make(map[string]any, len(job.Inputs))
	for k, v := range job.Inputs {
		out[k] = v
	}
	for key, pol := range job.ConfigPolicy {
		switch pol.Policy {
		case "group":
			if v, ok := client[key]; ok {
				// A `group` key with Options is a closed choice: accept the client's
				// value only when it's one of the allowed strings, else keep the
				// authored default (never trust an out-of-set value from the client).
				if len(pol.Options) > 0 {
					if s, isStr := v.(string); isStr && contains(pol.Options, s) {
						out[key] = v
					}
				} else {
					out[key] = v
				}
			}
		case "template":
			if v, ok := client[key]; ok {
				out[key] = v
			}
		case "group_range":
			if v, ok := client[key]; ok {
				out[key] = clampNumber(v, pol.Min, pol.Max)
			}
			// fixed, hidden, unknown: keep the authored value, ignore the client.
		}
	}
	// Any authored input that embeds a {{token}} (e.g. a `query` using
	// {{choice:agent-topic}}) is resolved client-side, because only the client can
	// read the group's Yjs choices. Trust the client's resolved value for such a
	// key even when its policy isn't declared `template`, so the topic/choice still
	// reaches the worker instead of being blanked by the guard below.
	for key, av := range job.Inputs {
		if s, ok := av.(string); ok && tokenRe.MatchString(s) {
			if v, ok := client[key]; ok {
				out[key] = v
			}
		}
	}
	// Controller inheritance: when the authored job opts in, forward the client's
	// `worker_prompts` map (each worker's own group-set additional_prompts, which
	// only the client can read from the group's Yjs doc). Sanitised to string
	// values so nothing but steering text reaches the worker. Not a config_policy
	// key, so it never renders as an editable field on the controller slide.
	if job.InheritWorkerPrompts {
		if wp, ok := client["worker_prompts"].(map[string]any); ok {
			clean := make(map[string]any, len(wp))
			for k, v := range wp {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					clean[k] = s
				}
			}
			if len(clean) > 0 {
				out["worker_prompts"] = clean
			}
		}
	}
	// guard: never ship a literal {{token}} to the worker (template inputs are
	// resolved client-side; this catches an unresolved one).
	for k, v := range out {
		if s, ok := v.(string); ok {
			out[k] = tokenRe.ReplaceAllString(s, "")
		}
	}
	return out
}

// clampNumber clamps a JSON number (float64) to [min, max] when the bounds are
// set. Non-numeric values are returned unchanged (a mistyped policy shouldn't
// drop a value silently).
func clampNumber(v any, min, max *float64) any {
	f, ok := v.(float64)
	if !ok {
		return v
	}
	if min != nil && f < *min {
		f = *min
	}
	if max != nil && f > *max {
		f = *max
	}
	return f
}
