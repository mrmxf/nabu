import * as Y from 'yjs';
import { toBase64, fromBase64 } from 'lib0/buffer';
import { pb } from '$lib/pocketbase';
import type { ResolvedValue } from '$lib/types';

// A minimal Yjs sync provider backed by PocketBase, not a websocket server.
// Every group working a lesson shares one Y.Doc (its "room" = `${lesson}:${group}`,
// derived server-side from the session). The document holds the group's
// collaborative state: notes (Y.Text per slide), choices, and group-editable
// agent configs.
//
// Transport reuses the same pipe the `decisions` store uses:
//   • local edit    → doc 'update' event → POST /api/ydoc/push (base64 update)
//   • the new row   → PocketBase realtime → every member applies it (Y.applyUpdate)
//   • on load       → GET /api/ydoc/pull → apply the whole log
// Yjs merges are idempotent and order-independent, so re-seeing our own pushed
// row (echo) is a harmless no-op; we still tag remote applies with a sentinel
// origin so our update handler never re-pushes them (no feedback loop).

// REMOTE marks updates that arrived from the server, so the local update handler
// knows not to push them back.
const REMOTE = Symbol('pb-remote');

// warnSyncBroken flags a failed sync call once (throttled) so a dead/old backend
// shows up in the console instead of looking like silently-broken collaboration.
// The most common cause is a server binary that predates the /api/ydoc endpoints.
let syncWarned = false;
function warnSyncBroken(op: 'push' | 'pull', detail: unknown): void {
	if (syncWarned) return;
	syncWarned = true;
	console.warn(
		`[nabu] group sync ${op} failed (${String(
			detail
		)}). Group notes/choices/agent-config will NOT sync. If this is a 404, the ` +
			`running server is missing /api/ydoc — rebuild & restart nabu (./nabu/scripts/dev.sh).`
	);
}

export interface GroupDoc {
	doc: Y.Doc;
	/** slideId → shared rich text; group members co-edit the same notes. */
	notes: Y.Map<Y.Text>;
	/** choiceId → resolved value {key,label,slug,scope}; the group's shared picks. */
	choices: Y.Map<ResolvedValue>;
	/** agentId → Y.Map of group-editable config overrides. */
	agentConfigs: Y.Map<Y.Map<unknown>>;
	/** resolves once the initial pull + merge has completed. */
	ready: Promise<void>;
	/** tear down realtime + local handlers; call on unmount. */
	destroy: () => void;
}

// noteText returns (creating if needed) the shared Y.Text for a slide's notes.
export function noteText(g: GroupDoc, slideId: string): Y.Text {
	let t = g.notes.get(slideId);
	if (!t) {
		t = new Y.Text();
		g.notes.set(slideId, t);
	}
	return t;
}

// applyTextDiff mutates a Y.Text to match `next`, as a minimal delete+insert in
// the changed middle (common prefix/suffix preserved). This is the standard
// "naive" plain-<textarea> ↔ CRDT binding: it merges concurrent edits made at
// different positions correctly, which a whole-string replace would not.
export function applyTextDiff(ytext: Y.Text, next: string): void {
	const cur = ytext.toString();
	if (cur === next) return;
	let start = 0;
	const minLen = Math.min(cur.length, next.length);
	while (start < minLen && cur[start] === next[start]) start++;
	let endCur = cur.length;
	let endNext = next.length;
	while (endCur > start && endNext > start && cur[endCur - 1] === next[endNext - 1]) {
		endCur--;
		endNext--;
	}
	ytext.doc?.transact(() => {
		if (endCur > start) ytext.delete(start, endCur - start);
		if (endNext > start) ytext.insert(start, next.slice(start, endNext));
	});
}

// configText returns (creating if needed) the shared Y.Text stored under `key` in
// an agent's config map. Only merge:'crdt' free-form fields (additional_prompts,
// material, verification) use this; atomic keys stay plain scalars via Y.Map.set.
// A brand-new Y.Text is seeded with `seed` (the authored default) so every member
// shares the same base text before the first edit — then applyTextDiff merges
// concurrent typing per character instead of clobbering. Note: like noteText, a
// simultaneous first edit by two members can race at the Y.Map key (one Y.Text
// wins); the window is a few ms and matches the notes behaviour.
export function configText(cfg: Y.Map<unknown>, key: string, seed = ''): Y.Text {
	const existing = cfg.get(key);
	if (existing instanceof Y.Text) return existing;
	const t = new Y.Text();
	const base = typeof existing === 'string' ? existing : seed;
	if (base) t.insert(0, base);
	cfg.set(key, t);
	return t;
}

// agentConfig returns (creating if needed) the group's override map for an agent.
export function agentConfig(g: GroupDoc, agentId: string): Y.Map<unknown> {
	let m = g.agentConfigs.get(agentId);
	if (!m) {
		m = new Y.Map();
		g.agentConfigs.set(agentId, m);
	}
	return m;
}

// createGroupDoc wires a Y.Doc to the caller's group room. `sessionId` is the
// only identity the client supplies; the server maps it to (group, lesson) so a
// student can only ever touch their own group's document.
export function createGroupDoc(sessionId: string): GroupDoc {
	const doc = new Y.Doc();
	const notes = doc.getMap<Y.Text>('notes');
	const choices = doc.getMap<ResolvedValue>('choices');
	const agentConfigs = doc.getMap<Y.Map<unknown>>('agentConfigs');

	// Push local edits to the server. Remote-origin updates are skipped so an
	// applied update never bounces back out.
	const onUpdate = (update: Uint8Array, origin: unknown) => {
		if (origin === REMOTE) return;
		void push(update);
	};
	doc.on('update', onUpdate);

	async function push(update: Uint8Array): Promise<void> {
		try {
			const res = await fetch('/api/ydoc/push', {
				method: 'POST',
				headers: { Authorization: pb.authStore.token, 'Content-Type': 'application/json' },
				body: JSON.stringify({ session_id: sessionId, update: toBase64(update) })
			});
			// A missing endpoint (old server binary) means the whole group never sees
			// this edit — surface it loudly instead of silently dropping sync.
			if (!res.ok) warnSyncBroken('push', res.status);
		} catch (err) {
			// network/transport failure: the local doc still reflects the edit and
			// will re-sync from the log on next load, but flag it so a dead backend
			// isn't mistaken for a logic bug.
			warnSyncBroken('push', err);
		}
	}

	let unsub: (() => void) | undefined;
	// the server-derived room, learned from the pull; used to filter the realtime
	// stream so we never apply another group's updates to our doc.
	let room = '';

	const ready = (async () => {
		// 1. seed from the stored log, and learn our room.
		try {
			const res = await fetch(`/api/ydoc/pull?session_id=${encodeURIComponent(sessionId)}`, {
				headers: { Authorization: pb.authStore.token }
			});
			if (res.ok) {
				const body = (await res.json()) as { room?: string; updates?: string[] };
				room = body.room ?? '';
				for (const u of body.updates ?? []) Y.applyUpdate(doc, fromBase64(u), REMOTE);
			} else {
				// 404 here almost always means the running server predates /api/ydoc
				// (rebuild + restart it); other codes point at auth/session issues.
				warnSyncBroken('pull', res.status);
			}
		} catch (err) {
			/* an empty doc is a valid starting point; realtime will fill in. */
			warnSyncBroken('pull', err);
		}
		// 2. follow live updates for this room only. The server tags each row with
		//    its room; we filter client-side (matching the decisions store pattern)
		//    so a different group's document never bleeds into ours.
		try {
			unsub = await pb.collection('ydoc_updates').subscribe('*', (e) => {
				const r = e.record as unknown as { room?: string; update?: string };
				if (r.update && (!room || r.room === room)) Y.applyUpdate(doc, fromBase64(r.update), REMOTE);
			});
		} catch {
			/* realtime is optional; the pull already seeded current state. */
		}
	})();

	return {
		doc,
		notes,
		choices,
		agentConfigs,
		ready,
		destroy() {
			doc.off('update', onUpdate);
			unsub?.();
			doc.destroy();
		}
	};
}
