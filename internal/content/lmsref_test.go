package content

import (
	"strings"
	"testing"
)

// TestRenderLmsRefs checks that lms:// references in a slide's markdown are
// rewritten to resolved URLs, and that an unresolved reference is left as authored
// and stamped as a warning.
func TestRenderLmsRefs(t *testing.T) {
	r := NewGoldmarkRenderer()
	ctx := RenderContext{
		// resolve only lms://resources/*; anything else returns ok=false.
		LmsResolve: func(ref string) (string, bool) {
			if strings.HasPrefix(ref, "lms://resources/") {
				name := strings.TrimPrefix(ref, "lms://resources/")
				return "https://cdn.example/bucket/prefix/resources/" + name, true
			}
			return "", false
		},
	}

	src := "![clip](lms://resources/demo.mp4)\n\n[notes](lms://group/s7g1/x.md)\n"
	out, issues, err := r.Render(src, ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(out, "https://cdn.example/bucket/prefix/resources/demo.mp4") {
		t.Errorf("resolved resource URL missing from output:\n%s", out)
	}
	if strings.Contains(out, `src="lms://resources/demo.mp4"`) {
		t.Errorf("resource ref was not rewritten:\n%s", out)
	}
	// The group ref can't resolve statically: left as authored + a warning.
	if !strings.Contains(out, "lms://group/s7g1/x.md") {
		t.Errorf("unresolved group ref should be left as authored:\n%s", out)
	}
	var warned bool
	for _, is := range issues {
		if strings.Contains(is.Msg, "lms://group/s7g1/x.md") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a warning for the unresolved group ref, got %+v", issues)
	}
}
