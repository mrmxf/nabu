package content

import (
	"fmt"
	"strings"
)

// Quiz validation. Mirrors validateChoiceGroups: authoring mistakes surface on
// the slide's content status (and so in /_/ and `seed --strict`) instead of
// silently producing a quiz that cannot be answered.
//
// The severity split is deliberate. Something that makes the quiz UNANSWERABLE
// is an ERROR (no questions, a choice question with no options, a duplicate id
// that would collide in storage). Something merely questionable is a WARNING
// (no correct answer marked, an unknown reveal mode) — an author mid-draft
// should not be blocked by a half-written quiz.

// quizTypes is the closed set of question types the widget can render.
var quizTypes = map[string]bool{
	"choice":    true, // one correct option
	"multi":     true, // several correct options
	"truefalse": true, // a choice with two implied options
	"short":     true, // free text, not auto-marked
}

// quizScopes mirrors t_choice's vocabulary: who answers together.
var quizScopes = map[string]bool{"student": true, "group": true}

// quizReveals is when the answer + rationale become visible.
var quizReveals = map[string]bool{"on-answer": true, "on-request": true, "never": true}

// validateQuizGroups checks a slide's t_quiz frontmatter and returns the worst
// status found with a human-readable reason.
func validateQuizGroups(gs []QuizGroup) (ContentStatus, string) {
	worst := StatusOK
	var msg string
	fail := func(level ContentStatus, format string, a ...any) {
		worst = worseStatus(worst, level)
		msg = appendIssue(msg, fmt.Sprintf(format, a...))
	}

	seenGroup := map[string]bool{}
	for _, g := range gs {
		// The id is the placement reference AND the future storage key, so a blank
		// or duplicated one is not recoverable.
		if strings.TrimSpace(g.ID) == "" {
			fail(StatusErrors, "t_quiz: a group is missing its id")
			continue
		}
		if seenGroup[g.ID] {
			fail(StatusErrors, "t_quiz: duplicate group id %q", g.ID)
			continue
		}
		seenGroup[g.ID] = true

		if g.Scope != "" && !quizScopes[g.Scope] {
			fail(StatusWarnings, "t_quiz %q: scope %q must be student|group (defaulting to student)", g.ID, g.Scope)
		}
		if g.Reveal != "" && !quizReveals[g.Reveal] {
			fail(StatusWarnings, "t_quiz %q: reveal %q must be on-answer|on-request|never", g.ID, g.Reveal)
		}
		if len(g.Questions) == 0 {
			fail(StatusErrors, "t_quiz %q: no questions", g.ID)
			continue
		}

		seenQ := map[string]bool{}
		for i, q := range g.Questions {
			where := fmt.Sprintf("t_quiz %q question %d", g.ID, i+1)
			if strings.TrimSpace(q.ID) == "" {
				fail(StatusErrors, "%s: missing id", where)
				continue
			}
			where = fmt.Sprintf("t_quiz %q/%s", g.ID, q.ID)
			if seenQ[q.ID] {
				fail(StatusErrors, "%s: duplicate question id", where)
				continue
			}
			seenQ[q.ID] = true

			if strings.TrimSpace(q.Stem) == "" {
				fail(StatusErrors, "%s: missing stem (the question text)", where)
			}
			typ := q.Type
			if typ == "" {
				typ = "choice"
			}
			if !quizTypes[typ] {
				fail(StatusErrors, "%s: type %q must be choice|multi|truefalse|short", where, q.Type)
				continue
			}

			if typ == "short" {
				// free text: options would never be shown, so they are a mistake
				// worth naming rather than quietly dropping.
				if len(q.Options) > 0 {
					fail(StatusWarnings, "%s: a short question ignores its options", where)
				}
				continue
			}

			// truefalse supplies its own options when the author omits them.
			if len(q.Options) == 0 && typ != "truefalse" {
				fail(StatusErrors, "%s: no options — nothing to choose from", where)
				continue
			}

			seenOpt := map[string]bool{}
			correct := 0
			for _, o := range q.Options {
				if strings.TrimSpace(o.Key) == "" {
					fail(StatusErrors, "%s: an option is missing its key", where)
					continue
				}
				if seenOpt[o.Key] {
					fail(StatusErrors, "%s: duplicate option key %q", where, o.Key)
				}
				seenOpt[o.Key] = true
				if strings.TrimSpace(o.Label) == "" {
					fail(StatusWarnings, "%s: option %q has no label", where, o.Key)
				}
				if o.Correct {
					correct++
				}
			}
			// A question with no marked answer still renders; it just cannot teach
			// anything on reveal. Warn rather than block a draft.
			if correct == 0 && len(q.Options) > 0 {
				fail(StatusWarnings, "%s: no option marked correct", where)
			}
			if typ == "choice" && correct > 1 {
				fail(StatusWarnings, "%s: %d options marked correct — use type: multi", where, correct)
			}
		}
	}
	return worst, msg
}

// normalizeQuizGroups fills in the documented defaults after validation, so the
// client receives a complete config and never has to re-derive them. Mutates in
// place, like normalizeChoiceStyles.
func normalizeQuizGroups(gs []QuizGroup) {
	for i := range gs {
		g := &gs[i]
		if !quizScopes[g.Scope] {
			g.Scope = "student"
		}
		if !quizReveals[g.Reveal] {
			g.Reveal = "on-answer"
		}
		for j := range g.Questions {
			q := &g.Questions[j]
			if q.Type == "" {
				q.Type = "choice"
			}
			// truefalse is a choice with two implied options; materialise them here
			// so the client renders one thing and an author writes `correct` on a
			// one-line question.
			if q.Type == "truefalse" && len(q.Options) == 0 {
				trueIsCorrect := strings.EqualFold(strings.TrimSpace(q.Answer), "true")
				q.Options = []QuizOption{
					{Key: "true", Label: "True", Correct: trueIsCorrect},
					{Key: "false", Label: "False", Correct: !trueIsCorrect},
				}
			}
		}
	}
}

// quizIDs returns the declared group ids, for validating a {{< LMS t="quiz" >}}
// placement against the slide's frontmatter.
func quizIDs(gs []QuizGroup) []string {
	ids := make([]string, 0, len(gs))
	for _, g := range gs {
		ids = append(ids, g.ID)
	}
	return ids
}
