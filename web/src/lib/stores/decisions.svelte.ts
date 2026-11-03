import type { ResolvedValue, ChoiceGroup } from '$lib/types';
import type { Map as YMap, YMapEvent } from 'yjs';

// Reactive, merged choice values for the lesson the student is running, keyed by
// choice id. It fuses three sources so widgets don't care where a value came from:
//   • session-scoped picks — mirrored from the session's work_state
//   • group-scoped picks   — held in the group's shared Yjs document (the
//                            `choices` Y.Map), kept live by the Yjs provider
//   • built-ins            — username/email/group/session, re-derived each visit
// LmsPrint and LmsChoice read this reactively, so a group member's pick updates
// everyone's picker + printed value without a reload. See docs/lesson-choice-values.md.
export const choiceValues = $state<Record<string, ResolvedValue>>({});

// The group's shared choices map, bound while a lesson is open. Group-scoped
// picks are written here (and so replicate to the whole group); the observer
// mirrors remote changes back into `choiceValues`.
let boundChoices: YMap<ResolvedValue> | undefined;
let choicesObserver: ((e: YMapEvent<ResolvedValue>) => void) | undefined;

// setChoiceValue upserts one resolved value into the reactive mirror (optimistic
// on pick, or from a fetch / realtime / Yjs event). It does NOT write to the
// shared doc — use commitChoice for a user's pick.
export function setChoiceValue(id: string, v: ResolvedValue): void {
	choiceValues[id] = v;
}

// commitChoice records a user's pick: it updates the local mirror immediately and,
// for a group-scoped choice, writes it into the shared Yjs map so every group
// member sees it. Session/builtin scopes stay local (persisted to work_state by
// the caller).
export function commitChoice(id: string, v: ResolvedValue): void {
	choiceValues[id] = v;
	if (v.scope === 'group' && boundChoices) boundChoices.set(id, v);
}

// bindChoices attaches the store to a group's `choices` Y.Map: it seeds the mirror
// with whatever the group has already decided and observes future changes. Call
// once per lesson run (after the group doc is created); pair with unbindChoices.
export function bindChoices(map: YMap<ResolvedValue>): void {
	unbindChoices();
	boundChoices = map;
	for (const [id, v] of map.entries()) if (v) choiceValues[id] = v;
	choicesObserver = (e: YMapEvent<ResolvedValue>) => {
		for (const id of e.keysChanged) {
			const v = map.get(id);
			if (v) choiceValues[id] = v;
		}
	};
	map.observe(choicesObserver);
}

// unbindChoices detaches the observer; the Yjs doc is destroyed by its owner.
export function unbindChoices(): void {
	if (boundChoices && choicesObserver) boundChoices.unobserve(choicesObserver);
	boundChoices = undefined;
	choicesObserver = undefined;
}

// resetChoiceValues clears the map — call when entering/leaving a lesson so one
// session's values never bleed into another.
export function resetChoiceValues(): void {
	unbindChoices();
	for (const k of Object.keys(choiceValues)) delete choiceValues[k];
}

// setBuiltins seeds reserved, non-choice keys a {{< LMS t="print" from="…" >}} can
// print — username, email, group, session. Stored under scope 'builtin' so they
// are never persisted to work_state (they are re-derived each visit). Empty
// values are skipped so a print falls through to nothing.
export function setBuiltins(vals: Record<string, string>): void {
	for (const [k, v] of Object.entries(vals)) {
		if (v) choiceValues[k] = { key: v, label: v, slug: v, scope: 'builtin' };
	}
}

// resolvePick turns a picked option key into the value to store, reading the
// option's label/slug and the group's scope from the (lesson-wide) group def.
export function resolvePick(group: ChoiceGroup, key: string): ResolvedValue {
	const opt = group.options.find((o) => o.key === key);
	return {
		key,
		label: opt?.label ?? key,
		slug: opt?.slug ?? key,
		scope: group.scope ?? 'session'
	};
}
