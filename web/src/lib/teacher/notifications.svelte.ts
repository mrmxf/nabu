import { pb } from '$lib/pocketbase';
import { getAgentHealth } from '$lib/lms/health';
import { handleAuthError } from '$lib/stores/auth.svelte';

// Things a teacher needs to know about RIGHT NOW, without going looking.
//
// Every source here is derived from something that already exists — no new
// endpoint, no new collection. That is deliberate: an alerting panel is only worth
// having if it is fed by real signals, and the cheapest real signals are the ones
// the app already reads.
//
// The ordering matters more than it looks. "The worker is down" is the failure that
// actually strands a lesson: students press the button, jobs queue forever, and
// nothing on screen explains why. It sorts first and it is the reason this panel
// exists.

export type Severity = 'error' | 'warning' | 'info';

export interface Notice {
	id: string; // stable across refreshes, so a re-render doesn't flicker
	severity: Severity;
	title: string;
	detail: string;
	action?: { label: string; run: () => Promise<void> };
}

const RANK: Record<Severity, number> = { error: 0, warning: 1, info: 2 };

interface State {
	items: Notice[];
	loading: boolean;
	error: string;
	checkedAt: Date | null;
}

export const notifications = $state<State>({
	items: [],
	loading: true,
	error: '',
	checkedAt: null
});

// refresh rebuilds the whole list from scratch each tick. Cheap (three reads, two
// of which are already cached) and far easier to reason about than reconciling
// individual notices in and out.
export async function refreshNotifications(): Promise<void> {
	const found: Notice[] = [];
	try {
		const [health, groups, jobs] = await Promise.all([
			getAgentHealth(true),
			pb.collection('groups').getFullList({ sort: 'tag' }),
			pb.collection('jobs').getFullList({
				filter: "status = 'queued' || status = 'running'",
				expand: 'group'
			})
		]);

		// 1. worker down — the one that strands a lesson.
		if (!health.alive) {
			found.push({
				id: 'worker-down',
				severity: 'error',
				title: 'The agent server is not responding',
				detail:
					`${health.reason ?? 'unreachable'} — students can still press the button, ` +
					`but their jobs will queue and never run.`
			});
		}

		// 2. paused groups — usually deliberate, but easy to forget you did it.
		for (const g of groups) {
			if (!g.paused) continue;
			found.push({
				id: `paused-${g.id}`,
				severity: 'warning',
				title: `Group ${g.tag} is paused`,
				detail: 'Students in this group cannot submit new jobs until you resume it.',
				action: {
					label: 'Resume',
					run: async () => {
						await pb.collection('groups').update(g.id, { paused: false });
						await refreshNotifications();
					}
				}
			});
		}

		// 3. queue full — same query QueueHistogram runs, bucketed the same way.
		const depth = new Map<string, { tag: string; n: number; max: number }>();
		for (const j of jobs) {
			const g = (j as { expand?: { group?: { tag?: string; max_depth?: number } } }).expand?.group;
			const tag = g?.tag ?? (j.group as string) ?? 'unknown';
			const row = depth.get(tag) ?? { tag, n: 0, max: g?.max_depth ?? 0 };
			row.n += 1;
			depth.set(tag, row);
		}
		for (const r of depth.values()) {
			if (r.max <= 0 || r.n < r.max) continue;
			found.push({
				id: `full-${r.tag}`,
				severity: 'warning',
				title: `Group ${r.tag} has a full queue`,
				detail: `${r.n} of ${r.max} slots in use — further submissions are refused until one finishes.`
			});
		}

		notifications.items = found.sort(
			(a, b) => RANK[a.severity] - RANK[b.severity] || a.title.localeCompare(b.title)
		);
		notifications.error = '';
		notifications.checkedAt = new Date();
	} catch (err) {
		if (handleAuthError(err)) return;
		// A failure to CHECK is itself worth surfacing — silently showing "No
		// notifications" when the check never ran would be the same class of quiet
		// failure this panel exists to prevent.
		notifications.error = err instanceof Error ? err.message : 'Could not check for problems';
	} finally {
		notifications.loading = false;
	}
}
