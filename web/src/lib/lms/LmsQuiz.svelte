<script lang="ts">
	import type { QuizGroup, QuizAnswer } from '$lib/types';

	// Self-check quiz, mounted by the LMS hydrator from {{< LMS t="quiz" id="…" >}}.
	// The questions come from the slide's t_quiz frontmatter (resolved by id in
	// registry.ts), not from this component's props payload.
	//
	// ── This is a CONTROLLED component, on purpose ────────────────────────────
	//
	// Answers come in via `value` and go out via `onAnswer`. It holds no state of
	// its own and never talks to the network. Today the parent keeps `value` in a
	// plain object that dies with the slide, so a quiz is formative: answer, see
	// if you were right, read why.
	//
	// When persistence arrives, NOTHING HERE CHANGES. The parent swaps its local
	// object for a store (session-scoped for scope="student", the group's shared
	// Yjs doc for scope="group", exactly as t_choice group picks already work) and
	// starts writing answers keyed (session, student-or-group, quiz.id,
	// question.id). Those ids are authored, stable, and already in the data below.
	// Keep it that way: if you find yourself adding a fetch() to this file, the
	// logic belongs in the parent instead. See docs/quiz-persistence.md.
	let {
		quiz,
		value = {},
		onAnswer,
		disabled = false
	}: {
		quiz: QuizGroup;
		value?: Record<string, QuizAnswer>;
		onAnswer?: (questionId: string, answer: QuizAnswer) => void;
		disabled?: boolean;
	} = $props();

	// A question is "committed" once the student has actually answered it. Reveal
	// is per question, not per quiz, so answering Q1 doesn't spoil Q3.
	function answered(qid: string): boolean {
		const a = value[qid];
		if (a === undefined) return false;
		return Array.isArray(a) ? a.length > 0 : String(a).trim() !== '';
	}

	// `never` collects without revealing (a teacher debriefs live); `on-request`
	// hides the answer behind a control; `on-answer` shows it immediately.
	let requested = $state<Record<string, boolean>>({});
	function revealed(qid: string): boolean {
		if (quiz.reveal === 'never') return false;
		if (quiz.reveal === 'on-request') return !!requested[qid];
		return answered(qid);
	}

	function pick(qid: string, key: string) {
		if (disabled) return;
		onAnswer?.(qid, key);
	}

	function toggle(qid: string, key: string) {
		if (disabled) return;
		const cur = Array.isArray(value[qid]) ? (value[qid] as string[]) : [];
		onAnswer?.(qid, cur.includes(key) ? cur.filter((k) => k !== key) : [...cur, key]);
	}

	function isSelected(qid: string, key: string): boolean {
		const a = value[qid];
		return Array.isArray(a) ? a.includes(key) : a === key;
	}

	// Whether the student's answer matches what the author marked correct. Used
	// only to style a revealed answer — there is no score anywhere.
	function isRight(qid: string): boolean {
		const q = quiz.questions.find((x) => x.id === qid);
		if (!q || !q.options?.length) return false;
		const correct = q.options.filter((o) => o.correct).map((o) => o.key);
		const a = value[qid];
		const given = Array.isArray(a) ? a : a === undefined ? [] : [String(a)];
		return (
			correct.length === given.length && correct.every((k) => given.includes(k))
		);
	}
</script>

<section class="lms-quiz" aria-label={quiz.legend || 'Check your understanding'}>
	{#if quiz.legend}
		<h3 class="lms-quiz-legend">{quiz.legend}</h3>
	{/if}

	{#each quiz.questions as q (q.id)}
		<fieldset class="lms-quiz-q" class:is-revealed={revealed(q.id)}>
			<legend class="lms-quiz-stem">{q.stem}</legend>

			{#if q.type === 'short'}
				<input
					type="text"
					class="lms-quiz-short"
					{disabled}
					value={typeof value[q.id] === 'string' ? (value[q.id] as string) : ''}
					oninput={(e) => onAnswer?.(q.id, (e.currentTarget as HTMLInputElement).value)}
					placeholder="Your answer"
				/>
			{:else}
				<ul class="lms-quiz-options">
					{#each q.options ?? [] as o (o.key)}
						<li>
							<label
								class="lms-quiz-option"
								class:is-picked={isSelected(q.id, o.key)}
								class:is-correct={revealed(q.id) && o.correct}
								class:is-wrong={revealed(q.id) && isSelected(q.id, o.key) && !o.correct}
							>
								<input
									type={q.type === 'multi' ? 'checkbox' : 'radio'}
									name="{quiz.id}-{q.id}"
									value={o.key}
									{disabled}
									checked={isSelected(q.id, o.key)}
									onchange={() => (q.type === 'multi' ? toggle(q.id, o.key) : pick(q.id, o.key))}
								/>
								<span>{o.label}</span>
							</label>
						</li>
					{/each}
				</ul>
			{/if}

			{#if quiz.reveal === 'on-request' && !requested[q.id]}
				<button
					type="button"
					class="pseudo lms-quiz-reveal"
					disabled={disabled || !answered(q.id)}
					onclick={() => (requested[q.id] = true)}
				>
					Show answer
				</button>
			{/if}

			{#if revealed(q.id)}
				<p class="lms-quiz-verdict" class:is-right={isRight(q.id)}>
					{#if q.type === 'short'}
						{#if q.answer}Suggested answer: {q.answer}{/if}
					{:else if isRight(q.id)}
						Correct
					{:else}
						Not quite
					{/if}
				</p>
				{#if q.rationale}
					<p class="lms-quiz-rationale">{q.rationale}</p>
				{/if}
			{/if}
		</fieldset>
	{/each}
</section>

<style>
	/* Styled from the tone vocabulary so a course theme reaches the quiz too —
	   see the presentational widget notes in Slide.svelte. */
	.lms-quiz {
		margin: 1.5rem 0;
	}
	.lms-quiz-legend {
		margin: 0 0 0.8rem;
		font-size: 1.1rem;
	}
	.lms-quiz-q {
		border: none;
		border-left: 5px solid var(--info, var(--hi2));
		background: oklch(from var(--info, var(--hi2)) l c h / 0.07);
		border-radius: 0 8px 8px 0;
		padding: 0.9rem 1.2rem;
		margin: 0 0 1rem;
	}
	.lms-quiz-stem {
		font-weight: 700;
		padding: 0;
		margin-bottom: 0.5rem;
	}
	.lms-quiz-options {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.lms-quiz-options li {
		margin: 0.25rem 0;
	}
	.lms-quiz-option {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		padding: 0.3rem 0.5rem;
		border-radius: 6px;
		cursor: pointer;
	}
	.lms-quiz-option input {
		margin: 0;
	}
	/* Revealed state. Correct is always marked, so a student who picked wrong
	   still learns the right answer rather than just being told "no". */
	.lms-quiz-option.is-correct {
		background: oklch(from var(--ok, var(--hi4)) l c h / 0.18);
	}
	.lms-quiz-option.is-wrong {
		background: oklch(from var(--attention, var(--hi1)) l c h / 0.15);
		text-decoration: line-through;
	}
	.lms-quiz-short {
		width: 100%;
		max-width: 28rem;
	}
	.lms-quiz-reveal {
		margin-top: 0.4rem;
		font-size: 0.9em;
	}
	.lms-quiz-verdict {
		margin: 0.6rem 0 0;
		font-weight: 700;
		color: var(--attention, var(--hi1));
	}
	.lms-quiz-verdict.is-right {
		color: var(--ok, var(--hi4));
	}
	.lms-quiz-rationale {
		margin: 0.3rem 0 0;
		color: var(--pi-meta);
	}
</style>
