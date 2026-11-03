package content

import (
	"strings"
	"testing"
)

func quiz(qs ...Question) []QuizGroup {
	return []QuizGroup{{ID: "g1", Questions: qs}}
}

// Things that make a quiz UNANSWERABLE are errors; things merely questionable
// are warnings, so a half-written draft isn't blocked.
func TestQuizValidationSeverity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		gs    []QuizGroup
		want  ContentStatus
		match string
	}{
		{
			name: "well formed",
			gs: quiz(Question{ID: "q1", Type: "choice", Stem: "What?", Options: []QuizOption{
				{Key: "a", Label: "Yes", Correct: true}, {Key: "b", Label: "No"},
			}}),
			want: StatusOK,
		},
		{
			name:  "group with no id",
			gs:    []QuizGroup{{Questions: []Question{{ID: "q1", Stem: "x"}}}},
			want:  StatusErrors,
			match: "missing its id",
		},
		{
			name: "duplicate group id",
			gs: []QuizGroup{
				{ID: "dup", Questions: []Question{{ID: "q1", Type: "short", Stem: "x"}}},
				{ID: "dup", Questions: []Question{{ID: "q1", Type: "short", Stem: "y"}}},
			},
			want:  StatusErrors,
			match: "duplicate group id",
		},
		{
			name:  "no questions",
			gs:    []QuizGroup{{ID: "g1"}},
			want:  StatusErrors,
			match: "no questions",
		},
		{
			name:  "duplicate question id",
			gs:    quiz(Question{ID: "q1", Type: "short", Stem: "a"}, Question{ID: "q1", Type: "short", Stem: "b"}),
			want:  StatusErrors,
			match: "duplicate question id",
		},
		{
			name:  "choice with no options",
			gs:    quiz(Question{ID: "q1", Type: "choice", Stem: "What?"}),
			want:  StatusErrors,
			match: "nothing to choose from",
		},
		{
			name:  "missing stem",
			gs:    quiz(Question{ID: "q1", Type: "short"}),
			want:  StatusErrors,
			match: "missing stem",
		},
		{
			name:  "unknown type",
			gs:    quiz(Question{ID: "q1", Type: "essay", Stem: "Discuss"}),
			want:  StatusErrors,
			match: "must be choice|multi|truefalse|short",
		},
		{
			name: "nothing marked correct is only a warning",
			gs: quiz(Question{ID: "q1", Type: "choice", Stem: "What?", Options: []QuizOption{
				{Key: "a", Label: "Yes"}, {Key: "b", Label: "No"},
			}}),
			want:  StatusWarnings,
			match: "no option marked correct",
		},
		{
			name: "several correct on a single-answer question",
			gs: quiz(Question{ID: "q1", Type: "choice", Stem: "What?", Options: []QuizOption{
				{Key: "a", Label: "Yes", Correct: true}, {Key: "b", Label: "No", Correct: true},
			}}),
			want:  StatusWarnings,
			match: "use type: multi",
		},
		{
			name:  "bad scope warns and does not block",
			gs:    []QuizGroup{{ID: "g1", Scope: "course", Questions: []Question{{ID: "q1", Type: "short", Stem: "x"}}}},
			want:  StatusWarnings,
			match: "must be student|group",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, msg := validateQuizGroups(tc.gs)
			if got != tc.want {
				t.Errorf("status = %s, want %s (%s)", got, tc.want, msg)
			}
			if tc.match != "" && !strings.Contains(msg, tc.match) {
				t.Errorf("message %q should contain %q", msg, tc.match)
			}
		})
	}
}

// Defaults are filled in after validation, so an author who writes the minimum
// still gets a complete config on the client.
func TestQuizNormalisesDefaults(t *testing.T) {
	gs := quiz(Question{ID: "q1", Stem: "What?", Options: []QuizOption{{Key: "a", Label: "Yes", Correct: true}}})
	normalizeQuizGroups(gs)
	if gs[0].Scope != "student" {
		t.Errorf("scope = %q, want student", gs[0].Scope)
	}
	if gs[0].Reveal != "on-answer" {
		t.Errorf("reveal = %q, want on-answer", gs[0].Reveal)
	}
	if gs[0].Questions[0].Type != "choice" {
		t.Errorf("type = %q, want choice", gs[0].Questions[0].Type)
	}
}

// truefalse is a choice with two implied options: the author writes one line and
// the client renders the same shape as every other choice question.
func TestQuizTrueFalseMaterialisesOptions(t *testing.T) {
	gs := quiz(Question{ID: "q1", Type: "truefalse", Stem: "Workflows have one trigger", Answer: "false"})
	normalizeQuizGroups(gs)
	opts := gs[0].Questions[0].Options
	if len(opts) != 2 {
		t.Fatalf("options = %d, want 2", len(opts))
	}
	if opts[0].Key != "true" || opts[1].Key != "false" {
		t.Errorf("unexpected option keys: %+v", opts)
	}
	if opts[0].Correct || !opts[1].Correct {
		t.Errorf("answer=false should mark the false option correct: %+v", opts)
	}
}

// An author-supplied truefalse with explicit options is left alone.
func TestQuizTrueFalseKeepsAuthoredOptions(t *testing.T) {
	gs := quiz(Question{ID: "q1", Type: "truefalse", Stem: "x", Options: []QuizOption{
		{Key: "yes", Label: "Yep", Correct: true}, {Key: "no", Label: "Nope"},
	}})
	normalizeQuizGroups(gs)
	if got := gs[0].Questions[0].Options[0].Key; got != "yes" {
		t.Errorf("authored options were overwritten: %q", got)
	}
}

// A quiz widget pointing at an id the slide doesn't declare renders inert; the
// author is warned rather than the slide breaking.
func TestQuizWidgetRefValidated(t *testing.T) {
	_, issues, err := preprocessLMS(`{{< LMS t="quiz" id="nope" >}}`, nil, []string{"real"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Level != StatusWarnings {
		t.Fatalf("want one warning, got %+v", issues)
	}
	if !strings.Contains(issues[0].Msg, "t_quiz") {
		t.Errorf("warning should name the frontmatter key, got %q", issues[0].Msg)
	}

	// a declared id is clean
	_, issues, err = preprocessLMS(`{{< LMS t="quiz" id="real" >}}`, nil, []string{"real"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("declared id should be clean, got %+v", issues)
	}
}

// The quiz is hydrated client-side (it is interactive), unlike the purely
// presentational widgets — so it must still emit an <lms> element.
func TestQuizEmitsHydrationElement(t *testing.T) {
	out, _, err := preprocessLMS(`{{< LMS t="quiz" id="g1" >}}`, nil, []string{"g1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<lms ") {
		t.Errorf("quiz should emit an <lms> element to hydrate:\n%s", out)
	}
}
