<script lang="ts">
	import { hydrateLms } from '$lib/lms/registry';
	import type { SlideRec, QuizAnswer } from '$lib/types';
	import type { GroupDoc } from '$lib/yjs/pbProvider';
	import AgentScreen from '$lib/components/AgentScreen.svelte';

	// One rendered slide: title, body (with inline LMS widgets hydrated — including
	// choice selectors placed by {{< LMS t="choice" >}}), plus the declarative
	// AgentScreen and notes. Shared by the session player and the read-only help
	// viewer. When `disabled` is set the try-it / write actions render inert
	// (guest/help preview); Next/Back live in the host.
	//
	// Choices: `onChoose(id, key)` reports a pick; the current selection is read
	// from the shared choiceValues store (so group picks live-update). `defaultKeyFor`
	// is lesson-wide so a {{< LMS t="print" from="…" >}} can resolve which field to
	// show for a choice defined on another slide.
	let {
		slide,
		disabled = false,
		onChoose,
		defaultKeyFor = () => 'label',
		noteValue = '',
		onNote,
		notesLabelHtml = '',
		groupTag = '',
		sessionId = '',
		lessonId = '',
		groupDoc = undefined
	}: {
		slide: SlideRec;
		disabled?: boolean;
		onChoose?: (id: string, key: string) => void;
		defaultKeyFor?: (from: string) => string;
		noteValue?: string;
		onNote?: (value: string) => void;
		notesLabelHtml?: string;
		groupTag?: string;
		sessionId?: string;
		lessonId?: string;
		groupDoc?: GroupDoc;
	} = $props();

	// the rendered slide body element, re-hydrated with LMS widgets on each change.
	let bodyEl = $state<HTMLElement | null>(null);

	// the group notes textarea. Notes are collaborative (a shared Y.Text), so
	// `noteValue` updates as other members type. We push those updates into the
	// element imperatively and ONLY when it isn't focused, so a remote edit never
	// yanks the local caret to the end mid-keystroke. Local edits flow out via
	// oninput; the incoming mirror of our own keystroke is skipped by the guard.
	let notesEl = $state<HTMLTextAreaElement | null>(null);
	$effect(() => {
		if (notesEl && document.activeElement !== notesEl && notesEl.value !== noteValue)
			notesEl.value = noteValue;
	});

	let disposeLms: (() => void) | undefined;
	// Self-check quiz answers, keyed quiz id -> question id -> answer.
	//
	// Deliberately local and deliberately ephemeral: a quiz is formative today, so
	// answers live here and are cleared when the slide changes. This is the ONE
	// place to change when answers are persisted — swap this object for a store
	// (session-scoped for scope="student", the group's Yjs doc for scope="group",
	// like choice picks) and LmsQuiz needs no edit at all, because it is a
	// controlled component. See docs/quiz-persistence.md.
	let quizAnswers = $state<Record<string, Record<string, QuizAnswer>>>({});
	$effect(() => {
		void slide?.id;
		quizAnswers = {};
	});

	$effect(() => {
		void slide?.id; // re-run when the slide changes
		disposeLms?.();
		if (bodyEl)
			disposeLms = hydrateLms(
				bodyEl,
				{
					groups: slide.t_choice ?? [],
					onSelect: (id, key) => onChoose?.(id, key),
					defaultKeyFor,
					disabled
				},
				{
					jobs: slide.t_agent ?? [],
					simOutput: slide.sim_output,
					groupTag,
					sessionId,
					lessonId,
					slide: slide.slug,
					disabled,
					groupDoc
				},
				{
					groups: slide.t_quiz ?? [],
					answers: quizAnswers,
					onAnswer: (quizId, questionId, answer) => {
						quizAnswers[quizId] = { ...(quizAnswers[quizId] ?? {}), [questionId]: answer };
					},
					disabled
				}
			);
		return () => disposeLms?.();
	});
</script>

<div class="card slide">
	<h2>{slide.title}</h2>
	{#key slide.id}
		<div class="body" bind:this={bodyEl}>{@html slide.body}</div>
	{/key}

	{#if slide.kind === 'agent' && !(slide.t_agent && slide.t_agent.length)}
		<!-- legacy simulated agent screen; slides using {{< LMS t="agent" >}} render
		     the real widget inline in the body instead. -->
		<AgentScreen agentKey={slide.agent_key} simOutput={slide.sim_output} {disabled} />
	{/if}

	{#if slide.kind === 'presentation' && !(slide.t_choice && slide.t_choice.length)}
		<p class="present-hint">Click <strong>Next</strong> when the presentation is finished.</p>
	{/if}

	{#if slide.notes_label}
		<label class="notes">
			<span class="notes-label">{@html notesLabelHtml || slide.notes_label}</span>
			<textarea
				bind:this={notesEl}
				rows="4"
				{disabled}
				oninput={(e) => onNote?.((e.currentTarget as HTMLTextAreaElement).value)}
				placeholder={disabled ? 'Sign in to save notes…' : 'Jot your answers here…'}
			></textarea>
		</label>
	{/if}
</div>

<style>
	.slide .body :global(table) {
		width: 100%;
	}
	.slide .body :global(blockquote) {
		border-left: 3px solid var(--hi2);
		padding-left: 0.8rem;
		color: var(--pi-dark);
	}
	/* Callout box (the paired {{< LMS t="callout" >}} container, emitted as
	   <aside class="lms-callout lms-callout-<style>"> in the slide body). Blue is
	   the named lapis-lazuli pigment at a fixed 60% lightness so it stays blue AND
	   readable in both light and dark themes. Centred on the page. */
	.slide .body :global(.lms-callout) {
		max-width: 42rem;
		margin: 1.5rem auto;
		padding: 0.85rem 1.25rem 1.1rem;
		border: 1px solid oklch(from var(--meso-lapis-lazuli-blue) 60% c h / 0.35);
		border-radius: 8px;
		/* rules sit every 1em; 2em line-height puts text on every other rule. */
		line-height: 2em;
	}
	.slide .body :global(.lms-callout .lms-callout-header) {
		font-weight: 700;
		line-height: 1.4;
		margin-bottom: 0.3rem;
		color: oklch(from var(--meso-lapis-lazuli-blue) 60% c h);
	}
	/* notepad — lightweight themed background with faint blue rules every 1em. */
	.slide .body :global(.lms-callout-notepad) {
		background-color: oklch(from var(--bg2) 96%  8%  h);
		background-image: repeating-linear-gradient(
			to bottom,
			transparent 0,
			transparent calc(1em - 1px),
			oklch(from var(--meso-lapis-lazuli-blue) 60% c h / 0.22) calc(1em - 1px),
			oklch(from var(--meso-lapis-lazuli-blue) 60% c h / 0.22) 1em
		);
	}

	/* prompt / cli-output — code panels whose `header` renders as a faux Apple Mac
	   System-1 title bar (pinstripes + hollow close box + centred title) sitting
	   above the content. Both drop the notepad ruling, use the mono font and clip
	   to rounded corners so the title bar spans edge to edge. */
	.slide .body :global(.lms-callout-prompt),
	.slide .body :global(.lms-callout-cli-output) {
		line-height: 1.5;
		overflow: hidden;
		font-family: var(--nabu-font-mono);
	}
	/* Shared title-bar frame. Colours come from each style's header rule below via
	   currentColor (pinstripes, close box) plus its own bar background. The header
	   uses negative margins to break out of the aside padding so the bar reaches
	   the panel edges, while the wrapped content keeps that padding. */
	.slide .body :global(.lms-callout-prompt .lms-callout-header),
	.slide .body :global(.lms-callout-cli-output .lms-callout-header) {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
		height: 1.5rem;
		margin: -0.85rem -1.25rem 0.9rem;
		padding: 0;
		font-weight: 700;
		/* six horizontal pinstripes in a centred band */
		background-repeat: no-repeat;
		background-position: center;
		background-size: 100% 11px;
		background-image: repeating-linear-gradient(
			to bottom, currentColor 0 1px, transparent 1px 2px);
	}
	/* hollow close box at the far left, out of flow so the title stays centred */
	.slide .body :global(.lms-callout-prompt .lms-callout-header)::before,
	.slide .body :global(.lms-callout-cli-output .lms-callout-header)::before {
		content: "";
		position: absolute;
		left: 0.5rem;
		width: 0.7rem;
		height: 0.7rem;
		border: 1px solid currentColor;
	}
	/* solid pill behind the title so it cuts through the pinstripes */
	.slide .body :global(.lms-callout-prompt .lms-callout-title),
	.slide .body :global(.lms-callout-cli-output .lms-callout-title) {
		padding: 0 0.6em;
		background: inherit;
	}

	/* prompt — AI system prompts: gainsboro panel, dark-green mono text, black
	   Mac window frame, light (white) title bar. */
	.slide .body :global(.lms-callout-prompt) {
		background-color: gainsboro;
		color: darkgreen;
		border: 1px solid black;
	}
	.slide .body :global(.lms-callout-prompt .lms-callout-header),
	.slide .body :global(.lms-callout-prompt .lms-callout-title) {
		color: black;
		background-color: white;
	}
	.slide .body :global(.lms-callout-prompt .lms-callout-header)::before {
		background-color: white;
	}

	/* cli-output — real/demo terminal output: thick dark-forest-green border,
	   light lotus-green panel, green mono text, dark (inverted) title bar. */
	.slide .body :global(.lms-callout-cli-output) {
		background-color: #dce9d0; /* lotus green */
		color: green;
		border: 3px solid #0b3d0b; /* dark forest green */
	}
	.slide .body :global(.lms-callout-cli-output .lms-callout-header),
	.slide .body :global(.lms-callout-cli-output .lms-callout-title) {
		color: #dce9d0; /* lotus green stripes/text on the dark bar */
		background-color: #0b3d0b; /* dark forest green */
	}
	.slide .body :global(.lms-callout-cli-output .lms-callout-header)::before {
		background-color: #0b3d0b;
	}
	/* Post-it note (the paired {{< LMS t="postit" >}} container, emitted as
	   <aside class="lms-postit lms-postit-<subtype>" style="--postit-bg:…">). The
	   author's `color` is the note background; the banner is 20% darker (l*0.8) and
	   the ink is a same-hue tint derived from --postit-bg, so no author-supplied
	   colour is ever un-styled. --postit-bg defaults to a classic sticky yellow.
	   Drag + minimise are wired in registry.ts.

	   Ink is a CONTRAST-FLOORED version of the original l = 1 − 0.5·bgl idea. That
	   formula collapses to zero contrast near bgl ≈ 2/3, and CSS can't repair it
	   smoothly (the relative-colour `l` keyword can't leave the oklch() call, and a
	   floor needs a discontinuity at that crossover). So instead the ink snaps to a
	   dark-tint (--pi-ink-lo) or light-tint (--pi-ink-hi) lightness depending on
	   which side of --pi-ink-t the background sits — chroma & hue kept, so it still
	   reads as "same colour, legible". The threshold is picked so the oklch-L gap is
	   ≥ ~0.4 on BOTH sides (light side 0.58−0.18, dark side 0.97−0.58). The `*1000`
	   makes the clamp a crisp step. */
	.slide .body :global(.lms-postit) {
		--postit-bg: oklch(0.92 0.13 95);
		--pi-ink-lo: 0.18; /* dark tinted ink, for light notes */
		--pi-ink-hi: 0.97; /* light tinted ink, for dark notes */
		--pi-ink-t: 0.58; /* bg-lightness threshold between the two */
		position: relative;
		max-width: 42rem;
		margin: 1.5rem auto;
		/* The note's in-flow footprint is ONLY the banner; the body is an absolute
		   overlay (below). So the note starts collapsed, expanding never reflows the
		   page, and dragging leaves at most a banner-sized gap behind. */
	}
	/* Banner: the drag handle + minimise toggle. 20% darker than the note; title
	   left, toggle right. Its ink uses the SAME contrast floor against the banner's
	   own (0.8·l) lightness, so the title stays legible even on mid-tone notes where
	   the darker banner would otherwise erode a dark ink's contrast. Rounded on all
	   corners when collapsed; only the top when the body is open beneath it. */
	.slide .body :global(.lms-postit-banner) {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.25rem 0.5rem 0.25rem 0.75rem;
		border-radius: 6px;
		background: oklch(from var(--postit-bg) calc(l * 0.8) c h);
		color: oklch(
			from var(--postit-bg)
			calc(
				var(--pi-ink-lo) +
				(var(--pi-ink-hi) - var(--pi-ink-lo)) * clamp(0, (var(--pi-ink-t) - 0.8 * l) * 1000, 1)
			)
			c h
		);
		font-weight: 700;
		cursor: grab;
		user-select: none;
		touch-action: none; /* let the pointer drag handler own the gesture */
		box-shadow: 0 6px 14px oklch(0 0 0 / 0.25);
	}
	.slide .body :global(.lms-postit.is-open .lms-postit-banner) {
		border-radius: 6px 6px 0 0;
	}
	.slide .body :global(.lms-postit-banner.is-dragging) {
		cursor: grabbing;
	}
	/* Drag-affordance glyph at the banner's left edge; decorative, dimmed. */
	.slide .body :global(.lms-postit-grip) {
		flex: none;
		opacity: 0.6;
		font-size: 0.85em;
	}
	.slide .body :global(.lms-postit-title) {
		flex: 1;
		min-width: 0;
	}
	/* Toggle button — a small square glyph: '+' collapsed, '–' open. */
	.slide .body :global(.lms-postit-min) {
		flex: none;
		width: 1.2rem;
		height: 1.2rem;
		padding: 0;
		border: 1px solid currentColor;
		border-radius: 3px;
		background: transparent;
		color: inherit;
		font: inherit;
		line-height: 1;
		cursor: pointer;
	}
	.slide .body :global(.lms-postit-min)::before {
		content: '+';
	}
	.slide .body :global(.lms-postit.is-open .lms-postit-min)::before {
		content: '–'; /* en-dash */
	}
	/* Body: the wrapped markdown, wrapped client-side (registry.ts) so it can be an
	   absolute overlay hanging off the banner. Hidden until open; when open it floats
	   below the banner WITHOUT taking flow space, so the surrounding page never
	   shifts. It carries the note's own background, padding, ink and shadow. */
	.slide .body :global(.lms-postit-body) {
		display: none;
		position: absolute;
		top: 100%;
		left: 0;
		right: 0;
		z-index: 5;
		padding: 0.85rem 1.25rem 1.1rem;
		border-radius: 0 0 6px 6px;
		background: var(--postit-bg);
		color: oklch(
			from var(--postit-bg)
			calc(
				var(--pi-ink-lo) +
				(var(--pi-ink-hi) - var(--pi-ink-lo)) * clamp(0, (var(--pi-ink-t) - l) * 1000, 1)
			)
			c h
		);
		box-shadow: 0 6px 14px oklch(0 0 0 / 0.25);
		line-height: 1.5;
	}
	.slide .body :global(.lms-postit.is-open .lms-postit-body) {
		display: block;
	}
	/* subtype `list` — tighten a bulleted/numbered list to the note. */
	.slide .body :global(.lms-postit-list :is(ul, ol)) {
		margin: 0;
		padding-left: 1.3rem;
	}
	.slide .body :global(.lms-postit-list li) {
		margin: 0.15rem 0;
	}
	/* nested ordered lists number 1/2/3 → a/b/c → i/ii/iii by depth. */
	.slide .body :global(.lms-postit-list ol) {
		list-style-type: decimal;
	}
	.slide .body :global(.lms-postit-list ol ol) {
		list-style-type: lower-alpha;
	}
	.slide .body :global(.lms-postit-list ol ol ol) {
		list-style-type: lower-roman;
	}
	/* subtype `code` — let a fenced code block bleed to the note edges and drop the
	   note's own text colour so the code keeps its normal contrast. */
	.slide .body :global(.lms-postit-code pre) {
		margin: 0 -1.25rem -1.1rem;
		border-radius: 0 0 6px 6px;
		padding: 0.75rem 1.25rem;
		color: initial;
	}

	.present-hint {
		margin-top: 1rem;
		padding: 0.6rem 0.9rem;
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
		border: 1px dashed oklch(from var(--hi2) 85% calc(c * 0.4) h);
		border-radius: 8px;
		color: var(--pi-body);
	}
	.notes {
		display: block;
		font-weight: 600;
		margin-top: 1rem;
	}
	.notes-label {
		display: block;
		margin-bottom: 0.3rem;
	}
	.notes-label :global(small) {
		color: var(--pi-meta);
	}
	.notes textarea {
		width: 100%;
		font-weight: 400;
	}

	/* ── Presentational widgets ────────────────────────────────────────────────
	   Baseline styling for the server-rendered container widgets (define, steps,
	   step, grid, col, proscons, vocab, toolcard, banner, itemtype, video — see
	   contentwidgets.go). These are the DEFAULTS: a course re-themes them from
	   its _course.css, which loads after this component's styles.

	   They are written against the TONE vocabulary, never against a colour. A
	   widget emits `lms-tone-warn`; the rule below reads `--lms-tone`, and each
	   tone class sets `--lms-tone` to a semantic custom property. A course that
	   defines --warn/--ok/--info/… restyles every widget at once. The fallbacks
	   keep everything legible when no course stylesheet is loaded at all. */
	.slide .body :global(.lms-tone-define) {
		--lms-tone: var(--define, var(--hi4));
	}
	.slide .body :global(.lms-tone-ok) {
		--lms-tone: var(--ok, var(--hi4));
	}
	.slide .body :global(.lms-tone-warn) {
		--lms-tone: var(--warn, var(--hi3));
	}
	.slide .body :global(.lms-tone-info) {
		--lms-tone: var(--info, var(--hi2));
	}
	.slide .body :global(.lms-tone-attention) {
		--lms-tone: var(--attention, var(--hi1));
	}
	.slide .body :global(.lms-tone-step) {
		--lms-tone: var(--step, var(--hi3));
	}
	.slide .body :global(.lms-tone-neutral) {
		--lms-tone: var(--pi-meta);
	}

	/* define — the workhorse box. The tint is derived from the tone rather than
	   authored, so it tracks the theme (and dark mode) automatically. */
	.slide .body :global(.lms-define),
	.slide .body :global(.lms-col),
	.slide .body :global(.lms-step),
	.slide .body :global(.lms-vocab),
	.slide .body :global(.lms-toolcard) {
		--lms-tone: var(--define, var(--hi4));
		border-left: 5px solid var(--lms-tone);
		background: oklch(from var(--lms-tone) l c h / 0.09);
		border-radius: 0 8px 8px 0;
		padding: 1rem 1.25rem;
		margin: 1.5rem 0;
	}
	.slide .body :global(.lms-define-header),
	.slide .body :global(.lms-col-header),
	.slide .body :global(.lms-step-header),
	.slide .body :global(.lms-toolcard-header) {
		font-weight: 700;
		color: var(--lms-tone);
		margin-bottom: 0.4rem;
	}
	/* first/last child margins collapse awkwardly inside a padded box */
	.slide .body :global(.lms-define > :last-child),
	.slide .body :global(.lms-col > :last-child),
	.slide .body :global(.lms-step > :last-child) {
		margin-bottom: 0;
	}

	/* steps — numbering lives here, not in the markup, so inserting a step
	   renumbers the rest with no authoring change. */
	.slide .body :global(.lms-steps) {
		counter-reset: lms-step;
		display: block;
		margin: 1.5rem 0;
	}
	.slide .body :global(.lms-step) {
		counter-increment: lms-step;
		position: relative;
		padding-left: 3.25rem;
	}
	.slide .body :global(.lms-step)::before {
		content: counter(lms-step);
		position: absolute;
		left: 0.9rem;
		top: 0.9rem;
		width: 1.8rem;
		height: 1.8rem;
		border-radius: 50%;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		font-size: 0.9rem;
		font-weight: 700;
		background: var(--lms-tone);
		color: var(--pi-body-bg, #fff);
	}

	/* grid / proscons — auto-fit so columns stack on a narrow screen with no
	   authored breakpoint. */
	.slide .body :global(.lms-grid) {
		display: grid;
		gap: 1rem;
		margin: 1.5rem 0;
		grid-template-columns: repeat(
			auto-fit,
			minmax(min(var(--lms-grid-min, 18rem), 100%), 1fr)
		);
	}
	.slide .body :global(.lms-grid > .lms-col) {
		margin: 0;
	}

	/* vocab — term and part of speech on one line, example set apart. */
	.slide .body :global(.lms-vocab-term) {
		font-weight: 700;
		font-size: 1.15em;
		color: var(--lms-tone);
	}
	.slide .body :global(.lms-vocab-pos) {
		margin-left: 0.6rem;
		font-size: 0.85em;
		font-style: italic;
		color: var(--pi-meta);
	}
	.slide .body :global(.lms-vocab-example) {
		margin-top: 0.6rem;
		padding-left: 0.8rem;
		border-left: 2px solid var(--lms-tone);
		font-style: italic;
		color: var(--pi-meta);
	}

	/* toolcard */
	.slide .body :global(.lms-toolcard-header) {
		display: flex;
		align-items: baseline;
		gap: 0.6rem;
		flex-wrap: wrap;
	}
	.slide .body :global(.lms-toolcard-cost) {
		font-size: 0.8em;
		font-weight: 400;
		padding: 0.1rem 0.5rem;
		border-radius: 999px;
		background: oklch(from var(--lms-tone) l c h / 0.18);
		color: var(--pi-dark);
	}
	.slide .body :global(.lms-toolcard-focus) {
		margin: 0.2rem 0 0.5rem;
		color: var(--pi-meta);
		font-style: italic;
	}

	/* banner — a section divider across a long deck. */
	.slide .body :global(.lms-banner) {
		margin: 3rem 0 1.5rem;
		padding: 0.8rem 1.4rem;
		border-radius: 6px;
		background: linear-gradient(
			90deg,
			var(--lms-banner-from, var(--hi2)),
			var(--lms-banner-to, var(--hi4))
		);
		color: #fff;
		font-size: 1.4rem;
		font-weight: 700;
	}

	/* itemtype — replaces an LMS export's banner images. */
	.slide .body :global(.lms-itemtype) {
		display: inline-block;
		padding: 0.1rem 0.6rem;
		border-radius: 999px;
		font-size: 0.75em;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		background: var(--bg2);
		color: var(--pi-dark);
		border: 1px solid var(--hi2);
	}

	/* video — aspect-ratio box so the iframe scales without JS. */
	.slide .body :global(.lms-video) {
		position: relative;
		margin: 1.5rem 0;
		aspect-ratio: var(--lms-video-ratio, 16 / 9);
		max-width: 100%;
	}
	.slide .body :global(.lms-video iframe) {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		border: 0;
		border-radius: 8px;
	}

	/* Semantic callout styles (intro/note/tip/tips/warning/discuss) added to the
	   existing chrome styles above. Same tone approach: colour comes from the
	   course stylesheet when there is one, from nabu's tokens when there isn't. */
	.slide .body :global(.lms-callout-intro) {
		--lms-tone: var(--define, var(--hi4));
	}
	.slide .body :global(.lms-callout-note) {
		--lms-tone: var(--info, var(--hi2));
	}
	.slide .body :global(.lms-callout-tip),
	.slide .body :global(.lms-callout-tips) {
		--lms-tone: var(--info, var(--hi2));
	}
	.slide .body :global(.lms-callout-warning) {
		--lms-tone: var(--warn, var(--hi3));
	}
	.slide .body :global(.lms-callout-discuss) {
		--lms-tone: var(--attention, var(--hi1));
	}
	.slide .body :global(.lms-callout-intro),
	.slide .body :global(.lms-callout-note),
	.slide .body :global(.lms-callout-tip),
	.slide .body :global(.lms-callout-tips),
	.slide .body :global(.lms-callout-warning),
	.slide .body :global(.lms-callout-discuss) {
		max-width: none;
		margin: 1.5rem 0;
		line-height: inherit;
		border: none;
		border-left: 5px solid var(--lms-tone);
		border-radius: 0 8px 8px 0;
		background: oklch(from var(--lms-tone) l c h / 0.09);
	}
	.slide .body :global(.lms-callout-intro .lms-callout-header),
	.slide .body :global(.lms-callout-note .lms-callout-header),
	.slide .body :global(.lms-callout-tip .lms-callout-header),
	.slide .body :global(.lms-callout-tips .lms-callout-header),
	.slide .body :global(.lms-callout-warning .lms-callout-header),
	.slide .body :global(.lms-callout-discuss .lms-callout-header) {
		color: var(--lms-tone);
	}

	/* In-lesson cross-reference links (data-nabu-slide) look like links but jump
	   the deck rather than navigating; see onBodyClick in the run page. */
	.slide .body :global(a[data-nabu-slide]) {
		cursor: pointer;
		text-decoration-style: dotted;
	}
</style>
