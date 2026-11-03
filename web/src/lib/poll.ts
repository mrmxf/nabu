import { onDestroy } from 'svelte';

// One polling implementation for the whole app. Every live panel used to hand-roll
// `refresh(); setInterval(...); onDestroy(clearInterval)`, which meant each one had
// to remember the teardown and none of them stopped when the tab was hidden.
//
// A teacher switches between their slides and the dashboard constantly, so the
// visibility guard below is the largest single saving: a backgrounded dashboard
// costs nothing until it is looked at again.

export const LIVE_MS = 3000; // queue pressure — changes second to second
export const STEADY_MS = 5000; // worker health, slide positions
export const AMBIENT_MS = 10000; // landing-page notifications

// usePoll runs fn now, then every ms while the document is visible, and cleans up
// when the component unmounts. Call it during component initialisation (it
// registers onDestroy). Ticks that land while `document.hidden` are skipped, not
// queued — on becoming visible again it refreshes immediately so the first thing a
// teacher sees is current, not up to `ms` stale.
export function usePoll(fn: () => void | Promise<void>, ms: number): void {
	const run = () => void fn();

	run();
	const timer = setInterval(() => {
		if (!document.hidden) run();
	}, ms);

	const onVisible = () => {
		if (!document.hidden) run();
	};
	document.addEventListener('visibilitychange', onVisible);

	onDestroy(() => {
		clearInterval(timer);
		document.removeEventListener('visibilitychange', onVisible);
	});
}
