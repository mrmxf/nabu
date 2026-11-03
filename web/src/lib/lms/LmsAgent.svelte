<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { pb } from '$lib/pocketbase';
	import { getAgentHealth } from './health';
	import { choiceValues } from '$lib/stores/decisions.svelte';
	import { imagesEnabled } from '$lib/stores/settings.svelte';
	import type { AgentJob } from '$lib/types';
	import { Text as YText } from 'yjs';
	import type { Map as YMap } from 'yjs';
	import { applyTextDiff, configText, type GroupDoc } from '$lib/yjs/pbProvider';

	// Real agent-job widget mounted by hydrateLms from a {{< LMS t="agent" id="…" >}}
	// shortcode. The hydrator resolves the id to the slide's t_agent config and
	// passes it here with the slide's sim_output (the offline fallback) and the
	// student's group tag.
	//
	// State machine (see docs/phase3-tracking-spec.md A2):
	//   idle -> dispatching -> queued -> running -> done
	//                       \-> failed | queue_full
	//   server_offline is entered up-front when the health probe is red.
	//
	// Dispatch posts to /api/job/submit and then follows the job over PocketBase
	// realtime. That endpoint is the next slice; until it exists a submit falls
	// back to the authored sample (when sim_output_fallback is set), so the widget
	// is useful now and forward-compatible later.
	let {
		job,
		simOutput = '',
		groupTag = '',
		sessionId = '',
		slide = '',
		configMap = undefined,
		groupDoc = undefined,
		disabled = false
	}: {
		job: AgentJob;
		simOutput?: string;
		groupTag?: string;
		sessionId?: string;
		slide?: string;
		// the group's shared config overrides for THIS agent (a Y.Map). Present in
		// the live player; undefined in the read-only help viewer (config is then
		// shown but not editable).
		configMap?: YMap<unknown>;
		// the whole group doc, used only by a controller job with
		// inherit_worker_prompts to read the OTHER agents' group-set additional_prompts
		// at run time. Undefined in the help viewer / groupless play.
		groupDoc?: GroupDoc;
		disabled?: boolean;
	} = $props();

	type Status =
		| 'loading'
		| 'idle'
		// no_controller is NOT a fault: the server is fine, the group just hasn't
		// picked its controller yet. Kept distinct from server_offline so a normal
		// pre-choice slide never claims the agent server is down.
		| 'no_controller'
		| 'server_offline'
		| 'dispatching'
		| 'queued'
		| 'running'
		| 'done'
		| 'failed'
		| 'queue_full'
		| 'images_exhausted';

	let status = $state<Status>('loading');
	let message = $state('');
	let queuePos = $state(0);
	let queueDepth = $state(0);
	let sample = $state(false); // true when the shown result is the offline sample
	let resultHtml = $state(''); // the real job result (server-rendered) on done
	let jobId = $state('');
	let unsub: (() => void) | undefined;

	// Copy-to-next: the rendered output element and a short "copied" acknowledgement.
	// copiedOnce stops the throbbing nudge once the student has taken the output.
	let outputEl = $state<HTMLDivElement | undefined>();
	let copied = $state(false);
	let copiedOnce = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | undefined;
	async function copyOutput() {
		const text = (outputEl?.innerText ?? '').trim();
		if (!text) return;
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
			copiedOnce = true;
			clearTimeout(copyTimer);
			copyTimer = setTimeout(() => (copied = false), 2000);
		} catch {
			// clipboard blocked (permissions/insecure context); leave the button as-is.
		}
	}

	// Image budget ("make an image" mode). The group's remaining images live
	// on the server-authoritative groups record; we read it once and then follow it
	// over realtime so decrements (a teammate's image) and teacher overrides show
	// live. null until loaded / not an image agent.
	let imagesRemaining = $state<number | null>(null);
	let groupUnsub: (() => void) | undefined;
	// True when this agent run makes an image, so it draws on the image budget.
	// `format` is authored fixed, so the authored value is authoritative here.
	const isImageJob = $derived(String(job.inputs?.format ?? '') === 'image');
	// Whether the image back-end is exposed (app_config `images_enabled`). An image
	// job renders nothing while this is false — see the template guard.
	const imgEnabled = $derived(imagesEnabled());
	// Out of images: block the run and explain why (image agents only).
	const outOfImages = $derived(
		isImageJob && imagesRemaining !== null && imagesRemaining <= 0
	);

	// Authored per job (t_agent `icon`); a robot when the course sets none.
	const avatar = $derived(job.icon || '🤖');

	// The group's live config overrides for this agent, mirrored from the shared
	// Y.Map so a teammate's slider move updates everyone. Only group-editable keys
	// are ever written here; authored defaults are read from job.inputs.
	let overrides = $state<Record<string, unknown>>({});
	let cfgObserver: (() => void) | undefined;
	function syncOverrides() {
		if (!configMap) return;
		const next: Record<string, unknown> = {};
		// A merge:'crdt' key holds a Y.Text child; mirror its string so the textarea
		// and effectiveInputs() see a plain value, never the Y.Text object itself.
		for (const [k, v] of configMap.entries()) next[k] = v instanceof YText ? v.toString() : v;
		overrides = next;
	}
	onMount(() => {
		if (!configMap) return;
		syncOverrides();
		cfgObserver = () => syncOverrides();
		// observeDeep, not observe: a teammate typing into a crdt key mutates a nested
		// Y.Text, which a shallow observe on the config map would never report.
		configMap.observeDeep(cfgObserver);
	});

	// policyOf returns a key's access rule (default 'fixed' when unspecified).
	const policyOf = (key: string) => job.config_policy?.[key]?.policy ?? 'fixed';

	// mergeOf returns a key's collaborative write mode: 'crdt' merges concurrent
	// edits per-character (free-form prose), 'atomic' (default) is last-writer-wins
	// on the whole value (paths, numbers, choices).
	const mergeOf = (key: string) => job.config_policy?.[key]?.merge ?? 'atomic';

	// resolveTokens fills {{choice:ID}} / {{field:ID}} from the group's shared
	// choices so a template input (e.g. a search agent's query) shows/sends real text
	// instead of the literal token.
	function resolveTokens(s: string): string {
		return s.replace(/\{\{\s*(?:choice|field):([^}]+?)\s*\}\}/g, (_, id) => {
			const v = choiceValues[id.trim()];
			return v ? (v.label ?? v.slug ?? v.key ?? '') : '';
		});
	}

	// resolveAgentToken resolves a {{choice:ID}} in the agent field to the option's
	// KEY (agent/tool names are keys like pip|orca|buzz, not human labels). A plain
	// agent name (no token) passes through unchanged. Used for the controller slide,
	// whose agent is the group's chosen controller.
	function resolveAgentToken(s: string): string {
		return s.replace(/\{\{\s*(?:choice|field):([^}]+?)\s*\}\}/g, (_, id) => {
			const v = choiceValues[id.trim()];
			return v ? (v.key ?? v.slug ?? v.label ?? '') : '';
		});
	}

	// dispatchAgent is the REAL agent this run targets: for the controller slide,
	// job.agent is a {{choice:…}} token, so this resolves to the group's chosen
	// controller key (pip|orca|buzz). Reactive, so it settles when the group's
	// choice loads (which can arrive after mount). Empty until the choice is made.
	const dispatchAgent = $derived(resolveAgentToken(job.agent));

	// activeProfile is the chosen variant's profile (e.g. the pip|orca|buzz
	// controller the group picked), keyed by the resolved agent name. Empty until
	// the choice is made or when the agent has no profiles. Supplies per-variant
	// defaults for group-editable inputs and the `system` text the agentdef widget
	// shows — see model.AgentJob.Profiles.
	const activeProfile = $derived<Record<string, string>>(job.profiles?.[dispatchAgent] ?? {});

	// defaultFor is a key's authored default: the active profile's value when it
	// supplies one, else the job's inputs default. This is what a group-editable
	// key falls back to until the group edits it ("group memory empty").
	const defaultFor = (key: string): unknown =>
		key in activeProfile ? activeProfile[key] : job.inputs?.[key];

	// currentValue is the effective value shown/sent for a key: the group's
	// override when set, else the authored default (profile-aware).
	const currentValue = (key: string): unknown =>
		key in overrides ? overrides[key] : defaultFor(key);

	// editableKeys: authored inputs plus any group-only key declared in the policy
	// (e.g. the new additional_prompts), minus hidden keys, in a stable order.
	const editableKeys = $derived.by(() => {
		const keys = Object.keys(job.inputs ?? {});
		for (const k of Object.keys(job.config_policy ?? {})) if (!keys.includes(k)) keys.push(k);
		return keys.filter((k) => policyOf(k) !== 'hidden');
	});

	// setOverride writes a group-editable value into the shared map (or, offline,
	// a local-only mirror) so it survives until run and replicates to the group.
	function setOverride(key: string, value: unknown) {
		if (!configMap) {
			overrides = { ...overrides, [key]: value }; // groupless/offline: local mirror
			return;
		}
		if (mergeOf(key) === 'crdt') {
			// free-form prose: diff the new value into the shared Y.Text so two typists
			// merge instead of clobbering. Seed a fresh Y.Text with the authored default
			// (profile-aware) so every member shares the same base text.
			applyTextDiff(configText(configMap, key, String(defaultFor(key) ?? '')), String(value));
		} else {
			configMap.set(key, value); // atomic value: whole-value last-writer-wins
		}
	}

	// chipText renders a read-only (fixed) input compactly: arrays as a count.
	function chipText(key: string): string {
		const v = currentValue(key);
		if (Array.isArray(v)) return `${key}: ${v.length}`;
		if (typeof v === 'string' && v.includes('{{')) return `${key}: ${resolveTokens(v)}`;
		return `${key}: ${v}`;
	}

	// Server health, fetched once on mount.
	let health = $state<{ alive: boolean; agents: string[] } | null>(null);
	// Once the student runs (or reveals a sample), availability stops governing
	// status so the effect below can't clobber a job's state.
	let engaged = $state(false);
	onMount(async () => {
		health = await getAgentHealth();
	});

	// For an image agent, load the group's remaining-image budget and follow it live.
	onMount(async () => {
		if (!isImageJob || !sessionId) return;
		try {
			const state = await pb.send<{ group_id: string; images_remaining: number }>(
				`/api/group/state?session_id=${encodeURIComponent(sessionId)}`,
				{ method: 'GET' }
			);
			imagesRemaining = state.images_remaining;
			groupUnsub = await pb.collection('groups').subscribe(state.group_id, (e) => {
				const n = (e.record as unknown as { images_remaining?: number }).images_remaining;
				if (typeof n === 'number') imagesRemaining = n;
			});
		} catch {
			// budget display is best-effort; the server still enforces it at submit.
		}
	});

	// Pre-run availability, reactive so it settles once the group's controller
	// choice loads (dispatchAgent can resolve after mount). Uses the RESOLVED agent
	// name, not the literal token, and reports a clear "pick your controller" hint
	// when the choice hasn't been made yet.
	$effect(() => {
		if (engaged) return;
		const h = health;
		const a = dispatchAgent;
		if (!h) return; // still probing → stay 'loading'
		if (!h.alive) {
			status = 'server_offline';
			message = '';
		} else if (!a) {
			// Health is green — there is simply no controller chosen yet.
			status = 'no_controller';
			message = '';
		} else if (!h.agents.includes(a)) {
			status = 'server_offline';
			message = `agent "${a}" is not loaded on the server`;
		} else {
			status = 'idle';
			message = '';
		}
	});

	// JobRec is the subset of the jobs record the widget reacts to.
	interface JobRec {
		status: string;
		queue_pos?: number;
		result_html?: string;
	}

	// effectiveInputs is what the client proposes for this run: authored defaults
	// with the group's editable overrides applied and template tokens resolved.
	// The server re-enforces the same policy (clamps ranges, ignores fixed/hidden),
	// so this is advisory — but it keeps the request honest and self-describing.
	function effectiveInputs(): Record<string, unknown> {
		const out: Record<string, unknown> = { ...(job.inputs ?? {}) };
		for (const key of editableKeys) {
			const policy = policyOf(key);
			if (policy === 'group' || policy === 'group_range') {
				// send the effective value (override, else the profile-aware default) so a
				// per-variant default reaches the server even when the group hasn't edited
				// it and the key isn't in job.inputs.
				out[key] = currentValue(key);
			} else if (policy === 'template') {
				out[key] = resolveTokens(String(currentValue(key) ?? ''));
			}
		}
		// Resolve {{choice:…}}/{{field:…}} tokens in ANY string input — even those not
		// declared `template` (e.g. a `query` that embeds {{choice:agent-topic}}), so
		// the resolved value, not the literal token, is what reaches the worker.
		for (const [key, val] of Object.entries(out)) {
			if (typeof val === 'string' && val.includes('{{')) out[key] = resolveTokens(val);
		}
		return out;
	}

	// gatherWorkerPrompts reads each worker's own group-set additional_prompts from
	// the shared group doc, so a controller run (inherit_worker_prompts) steers the
	// workers exactly as a manual run would. Keyed by worker name (e.g. search/sort/
	// present), taken from the controller's `workers` input; empties are skipped. Only
	// the client can read these — they live in the group's Yjs doc, not the DB — so
	// the server trusts the resulting `worker_prompts` map (see effectiveInputs).
	function gatherWorkerPrompts(): Record<string, string> {
		const out: Record<string, string> = {};
		if (!groupDoc) return out;
		const workers = String(job.inputs?.workers ?? '')
			.split(',')
			.map((w) => w.trim())
			.filter(Boolean);
		for (const name of workers) {
			const v = groupDoc.agentConfigs.get(name)?.get('additional_prompts');
			const text = v instanceof YText ? v.toString() : typeof v === 'string' ? v : '';
			if (text.trim()) out[name] = text;
		}
		return out;
	}

	async function run() {
		engaged = true;
		sample = false;
		resultHtml = '';
		status = 'dispatching';
		message = '';
		try {
			const inputs = effectiveInputs();
			if (job.inherit_worker_prompts) {
				const wp = gatherWorkerPrompts();
				if (Object.keys(wp).length) inputs.worker_prompts = wp;
			}
			const res = await pb.send<{ job: { id: string } & JobRec }>('/api/job/submit', {
				method: 'POST',
				body: {
					session_id: sessionId,
					slide,
					agent_id: job.id,
					agent: dispatchAgent,
					inputs,
					timeout: job.timeout ?? ''
				}
			});
			jobId = res.job.id;
			applyJob(res.job);
			follow(jobId);
		} catch (err: unknown) {
			handleSubmitError(err);
		}
	}

	// applyJob maps a server job record onto the widget state.
	function applyJob(rec: JobRec) {
		if (rec.queue_pos !== undefined) queuePos = rec.queue_pos;
		if (rec.status === 'queued') status = 'queued';
		else if (rec.status === 'running') status = 'running';
		else if (rec.status === 'done') {
			resultHtml = rec.result_html ?? '';
			sample = false;
			status = 'done';
		} else if (rec.status === 'failed') {
			status = 'failed';
			message = message || 'the job failed';
		}
	}

	// follow subscribes to the job row for live status, then reconciles once in
	// case the (fast) job already finished before the subscription landed.
	async function follow(id: string) {
		try {
			unsub = await pb.collection('jobs').subscribe(id, (e) => {
				applyJob(e.record as unknown as JobRec);
				if (e.record.status === 'done' || e.record.status === 'failed') unsub?.();
			});
			const rec = await pb.collection('jobs').getOne(id);
			applyJob(rec as unknown as JobRec);
		} catch {
			// realtime unavailable; the submit-response state stands.
		}
	}

	// handleSubmitError distinguishes the two designed rejections (429 queue_full,
	// 503 server_unreachable) from "endpoint not built yet", which falls back to
	// the authored sample when the slide allows it.
	function handleSubmitError(err: unknown) {
		const status429 = (err as { status?: number })?.status;
		const data = (
			err as {
				data?: { group_active?: number; group_max?: number; error?: string; images_remaining?: number };
			}
		)?.data;
		if (status429 === 429 && data?.error === 'images_exhausted') {
			imagesRemaining = data?.images_remaining ?? 0;
			status = 'images_exhausted';
			return;
		}
		if (status429 === 429) {
			queuePos = data?.group_active ?? 0;
			queueDepth = data?.group_max ?? 0;
			status = 'queue_full';
			return;
		}
		if (status429 === 503) {
			status = 'server_offline';
			return;
		}
		// endpoint not available yet (404/501) — degrade gracefully.
		if (job.sim_output_fallback && simOutput) {
			reveal(true);
		} else {
			status = 'failed';
			message = 'job dispatch is not wired up yet';
		}
	}

	// reveal shows the result panel; sampleFlag marks it as the offline sample.
	function reveal(sampleFlag: boolean) {
		engaged = true;
		sample = sampleFlag;
		status = 'done';
	}

	function showSample() {
		reveal(true);
	}

	function reset() {
		unsub?.();
		engaged = false; // let availability govern the pre-run state again
		status = 'idle';
		message = '';
		sample = false;
		resultHtml = '';
	}

	onDestroy(() => {
		unsub?.();
		groupUnsub?.();
		clearTimeout(copyTimer);
		if (configMap && cfgObserver) configMap.unobserveDeep(cfgObserver);
	});
</script>

<!-- Image back-end disabled (app_config `images_enabled`): render nothing for an
     image job. The student sequencer already drops whole image slides; this guards
     the other players (teacher page + help viewer) so no "make an image" widget
     leaks through, and the job API refuses the submit regardless. -->
{#if isImageJob && !imgEnabled}
	<!-- image generation is turned off -->
{:else}
<div class="agent">
	<div class="head">
		<span class="avatar">{avatar}</span>
		<strong>{job.title || job.agent}</strong>
		{#if groupTag}<span class="tag">{groupTag}</span>{/if}
		{#if isImageJob && imagesRemaining !== null}
			<span class="images" class:none={imagesRemaining <= 0} title="images your group can still make"
				>🖼️ {imagesRemaining} left</span
			>
		{/if}
		<span class="spacer"></span>
		{#if status === 'idle'}
			<button onclick={run} disabled={disabled || outOfImages}>Run</button>
		{:else if status === 'dispatching' || status === 'queued' || status === 'running'}
			<button disabled>Working…</button>
		{:else if status === 'done' || status === 'failed'}
			<button class="pseudo" onclick={reset}>Run again</button>
		{/if}
	</div>

	{#if editableKeys.length}
		<div class="config">
			{#each editableKeys as key (key)}
				{@const policy = policyOf(key)}
				{#if policy === 'group_range'}
					{@const p = job.config_policy?.[key]}
					<label class="field">
						<span class="k">{key}</span>
						<input
							type="number"
							min={p?.min}
							max={p?.max}
							value={Number(currentValue(key) ?? 0)}
							{disabled}
							oninput={(e) => setOverride(key, Number((e.currentTarget as HTMLInputElement).value))}
						/>
						{#if p?.min !== undefined || p?.max !== undefined}
							<span class="range">({p?.min ?? '–'}–{p?.max ?? '–'})</span>
						{/if}
					</label>
				{:else if policy === 'group' && job.config_policy?.[key]?.options?.length}
					{@const opts = job.config_policy?.[key]?.options ?? []}
					<label class="field">
						<span class="k">{key}</span>
						<select
							value={String(currentValue(key) ?? '')}
							{disabled}
							onchange={(e) => setOverride(key, (e.currentTarget as HTMLSelectElement).value)}
						>
							{#each opts as opt (opt)}
								<option value={opt}>{opt}</option>
							{/each}
						</select>
					</label>
				{:else if policy === 'group' && typeof (currentValue(key) ?? job.inputs?.[key]) === 'boolean'}
					<label class="field check">
						<input
							type="checkbox"
							checked={Boolean(currentValue(key))}
							{disabled}
							onchange={(e) => setOverride(key, (e.currentTarget as HTMLInputElement).checked)}
						/>
						<span class="k">{key}</span>
					</label>
				{:else if policy === 'group'}
					<label class="field wide">
						<span class="k">{key}</span>
						<textarea
							rows="2"
							value={String(currentValue(key) ?? '')}
							{disabled}
							oninput={(e) => setOverride(key, (e.currentTarget as HTMLTextAreaElement).value)}
						></textarea>
					</label>
				{:else if policy === 'template'}
					<div class="field wide">
						<span class="k">{key}</span>
						<span class="preview">{resolveTokens(String(currentValue(key) ?? ''))}</span>
					</div>
				{:else}
					<span class="chip">{chipText(key)}</span>
				{/if}
			{/each}
		</div>
	{/if}

	{#if status === 'loading'}
		<p class="muted hint">Checking the agent server…</p>
	{:else if status === 'no_controller'}
		<!-- Nothing is wrong here — this is the normal state of the controller slide
		     before the group has chosen. Deliberately not the red "off" banner. -->
		<div class="banner info">No controller selected — pick one above, then run.</div>
	{:else if status === 'server_offline'}
		<div class="banner off">
			⚠ The agent server is offline{message ? ` — ${message}` : ''}.
			{#if job.sim_output_fallback && simOutput}
				<button class="pseudo small" onclick={showSample}>See a sample result</button>
			{/if}
		</div>
	{:else if status === 'queue_full'}
		<div class="banner warn">
			Your group's queue is full ({queuePos}/{queueDepth}). Wait for a job to finish, then try
			again.
		</div>
	{:else if status === 'images_exhausted'}
		<div class="banner warn">
			Your group has used all its images. Ask your teacher to grant more.
		</div>
	{:else if status === 'dispatching'}
		<div class="running"><Dots /> <span>Submitting…</span></div>
	{:else if status === 'queued'}
		<div class="running"><Dots /> <span>In your group's queue — position {queuePos}.</span></div>
	{:else if status === 'running'}
		<div class="running"><Dots /> <span>{dispatchAgent || job.agent} is working…</span></div>
	{:else if status === 'failed'}
		<div class="banner off">Job failed{message ? ` — ${message}` : ''}.</div>
	{:else if status === 'done'}
		{#if sample}<p class="sample-note">Sample result (server offline).</p>{/if}
		<div class="output" bind:this={outputEl}>{@html sample ? simOutput : resultHtml}</div>
		{#if job.copy_next}
			<div class="copy-nudge" class:taken={copiedOnce}>
				<button class="copy-btn" onclick={copyOutput}>
					{copied ? '✓ Copied — paste it into the next agent' : '📋 Copy this output'}
				</button>
			</div>
		{/if}
	{:else if status === 'idle'}
		{#if outOfImages}
			<p class="muted hint">
				Your group has used all its images. Ask your teacher to grant more.
			</p>
		{:else}
			<p class="muted hint">Press <em>Run</em> to send this job to the server.</p>
		{/if}
	{/if}
</div>
{/if}

{#snippet Dots()}
	<span class="dots"><span class="d"></span><span class="d"></span><span class="d"></span></span>
{/snippet}

<style>
	.agent {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		background: oklch(from var(--hi2) 97% calc(c * 0.3) h);
		padding: 0.8rem 1rem;
		margin-top: 1rem;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.avatar {
		font-size: 1.3rem;
	}
	.spacer {
		flex: 1;
	}
	.tag {
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
		background: var(--ink-cuneiform);
		color: #fff;
		border-radius: 4px;
		padding: 0.05rem 0.4rem;
		font-size: 0.8rem;
	}
	.images {
		font-size: 0.8rem;
		color: var(--pi-meta);
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
	}
	.images.none {
		color: var(--hi1);
	}
	.config {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
		margin-top: 0.6rem;
	}
	.chip {
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		color: var(--pi-meta);
		border-radius: 4px;
		padding: 0.05rem 0.45rem;
		font-size: 0.8rem;
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
	}
	/* group-editable config controls. A `.field` is a labelled input the whole
	   group co-edits; a `.chip` above stays for fixed (read-only) values. */
	.field {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.8rem;
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
		background: oklch(from var(--hi2) 98% calc(c * 0.3) h);
		border: 1px solid oklch(from var(--hi2) 85% calc(c * 0.35) h);
		border-radius: 4px;
		padding: 0.1rem 0.45rem;
	}
	.field.wide {
		display: flex;
		flex-direction: column;
		align-items: stretch;
		gap: 0.2rem;
		flex-basis: 100%;
	}
	.field .k {
		color: var(--pi-meta);
	}
	.field input[type='number'] {
		width: 4.5rem;
		margin: 0;
	}
	.field.check input {
		margin: 0;
	}
	.field textarea {
		width: 100%;
		font-family: inherit;
		font-size: 0.8rem;
	}
	.field .range {
		color: var(--pi-meta);
		opacity: 0.7;
	}
	.field .preview {
		font-family: var(--nabu-font, inherit);
		color: var(--pi-body);
	}
	.banner {
		margin-top: 0.7rem;
		padding: 0.55rem 0.8rem;
		border-radius: 6px;
		font-size: 0.9rem;
	}
	.banner.off {
		background: oklch(from var(--hi1) 96% calc(c * 0.4) h);
		color: var(--hi1);
	}
	.banner.warn {
		background: oklch(from var(--hi3) 95% calc(c * 0.5) h);
		color: oklch(from var(--hi3) 42% c h);
	}
	/* neutral, instructional — an expected step, not a fault (see no_controller) */
	.banner.info {
		background: oklch(from var(--hi2) 95% calc(c * 0.35) h);
		color: var(--pi-body);
	}
	.running {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.7rem 0.2rem;
		color: var(--pi-meta);
		font-style: italic;
	}
	.dots {
		display: inline-flex;
		gap: 0.25rem;
	}
	.dots .d {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--hi2);
		animation: pulse 1s infinite ease-in-out;
	}
	.dots .d:nth-child(2) {
		animation-delay: 0.15s;
	}
	.dots .d:nth-child(3) {
		animation-delay: 0.3s;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
			transform: translateY(0);
		}
		50% {
			opacity: 1;
			transform: translateY(-2px);
		}
	}
	.output {
		margin-top: 0.6rem;
		animation: fade 0.4s ease;
	}
	.output :global(table) {
		width: 100%;
	}
	@keyframes fade {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
	/* Copy-to-next nudge: a tight box hugging the button whose border throbs from
	   red to transparent on a 2s cycle to draw the eye. The throb stops (`.taken`)
	   once the student has copied, so it isn't a permanent distraction. */
	.copy-nudge {
		display: inline-block;
		margin-top: 0.7rem;
		padding: 3px;
		border: 2px solid transparent;
		border-radius: 8px;
		animation: throb 2s ease-in-out infinite;
	}
	.copy-nudge.taken {
		animation: none;
		border-color: transparent;
	}
	.copy-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		margin: 0;
	}
	@keyframes throb {
		0%,
		100% {
			border-color: rgba(220, 0, 0, 0);
			box-shadow: 0 0 0 0 rgba(220, 0, 0, 0);
		}
		50% {
			border-color: rgba(220, 0, 0, 1);
			box-shadow: 0 0 6px 1px rgba(220, 0, 0, 0.55);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.copy-nudge {
			animation: none;
			border-color: rgba(220, 0, 0, 0.85);
		}
	}
	.sample-note {
		margin: 0.6rem 0 0;
		font-size: 0.82rem;
		color: var(--pi-meta);
	}
	.hint {
		margin: 0.6rem 0 0;
	}
	.small {
		margin-left: 0.4rem;
		font-size: 0.85rem;
	}
</style>
