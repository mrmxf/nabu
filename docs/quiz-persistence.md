# Quiz persistence — the forward-compatibility contract

Nabu has a `quiz` widget. It is **formative only**: a student answers, sees
whether they were right, reads the rationale, and nothing is stored, scored or
sent anywhere. Answers die with the slide.

That is a deliberate first step, not an oversight. This note records the contract
the current implementation upholds so that adding persistence later is a code
change and **not a re-authoring of every lesson**. If you are the person adding
it, start here.

## What is already fixed

The authored data carries stable identity. Everything a stored answer needs to be
keyed on is written by the author today and validated at seed:

| Piece | Where it is authored | Enforced by |
|---|---|---|
| quiz id | `params.t_quiz[].id` | unique per slide, non-empty (ERROR otherwise) |
| question id | `params.t_quiz[].questions[].id` | unique per group, non-empty (ERROR otherwise) |
| who answers together | `params.t_quiz[].scope` — `student` \| `group` | closed set (WARNING + default otherwise) |
| the correct answer | `questions[].options[].correct` | parsed server-side, never sent as "the answer key" separate from the options |

So the storage key is already determined:

```
(session, student-or-group, quiz id, question id) -> answer
```

`answer` is an option key, a list of option keys (`multi`), or free text
(`short`) — the `QuizAnswer` type in `nabu/web/src/lib/types.ts`.

**Do not change these ids' meaning.** An author renaming a quiz id is renaming a
storage key; that is the same bargain `t_choice` ids already make, and it is why
both are documented as stable.

## What has to change, and where

Three places, in order:

1. **A collection.** Add `quiz_answers` (or extend `captures` — the derived-value
   snapshot collection is the same shape) keyed by the tuple above, with a
   migration modelled on `nabu/migrations/1751000005_choice_groups.go`.

2. **The host, not the component.** `LmsQuiz.svelte` is a *controlled component*:
   answers arrive as `value`, changes leave through `onAnswer`, and it never
   fetches. The state lives in its parent — `quizAnswers` in
   `nabu/web/src/lib/components/Slide.svelte`. Swap that plain object for:

   - `scope: "student"` → session-scoped storage, alongside the existing
     `work_state` writes;
   - `scope: "group"` → the group's shared Yjs document, exactly as `t_choice`
     group picks already work (`pbProvider` / `GroupDoc`, already threaded into
     `Slide.svelte`).

   If you find yourself adding a `fetch()` to `LmsQuiz.svelte`, the logic belongs
   in the parent instead.

3. **A teacher view.** The `captures` collection already snapshots derived values
   per `(session, key_id)` for teacher inspection; quiz answers are the same
   shape, so the dashboard pattern exists.

## What deliberately does not exist yet

- **No score.** Nothing sums correct answers. `isRight()` in `LmsQuiz.svelte`
  exists only to style a revealed answer.
- **No gradebook, no attempts, no pass mark.** Adding one is a product decision,
  not a missing feature.
- **No server-side marking.** `correct` is parsed server-side and shipped to the
  client with the options, because a formative self-check has nothing to protect.
  A *scored* quiz would need the correct answers withheld until submission — that
  is a real change to the seed payload, and the one place where the current
  design would need revisiting rather than extending.

## Authoring reference

The widget itself is documented for teachers in the built-in help:
`nabu/help/using-nabu/authoring-lessons/40-widgets.md`. Validation rules live in
`nabu/internal/content/quiz.go`, with the severity split (unanswerable → ERROR,
questionable → WARNING) explained at the top of that file.
