# Lesson choice values, scope & the `print` widget

Status: **MVP implemented** (2026-07-14) — session + group scope, resolved-value
storage, group sharing + realtime, and the `print` widget all built and verified
(group share / isolation / last-write-wins). **Group-scoped values moved from a
`decisions` collection to the group's Yjs document (2026-07-22); the collection is
dropped in migration `1751000021_drop_decisions.go`.** The **Unfinished /
deferred** list below is what remains.

Lets a lesson author reference a student/group's earlier choice inline, e.g.

```md
## Exploring {{< LMS t="print" from="human-topic" >}}
```

where `human-topic` is a `t_choice` group defined (possibly on another slide).
The referenced value is resolved to text at render time, scoped so one workshop
session never bleeds into another.

## Decisions taken

- Attribute name is **`from`** (not `formid`/`fromid`).
- **Store the resolved value** `{key,label,slug,scope}`, not just the key — because
  choice *definitions* are handed to the client per-slide
  (`Slide.svelte`: `groups: slide.t_choice`), so a `print` on a later slide has no
  access to the options to turn a key into a label. Storing the resolved value
  decouples `print` from where the choice was defined.
- **Two scopes now**: `session` (personal → `session.work_state.choices`) and
  `group` (shared → the group's Yjs `choices` map, persisted via `ydoc_updates`),
  with live update via PB realtime. Cross-cohort isolation rides on the
  `(lesson, session, tag)` group key (see item 5 below).
- **`scope` is required** in the `t_choice` schema; the catalogue declares a
  per-type `default_value_key` (choice → `slug`), checked for existence at seed.
- Metadata `locked|editable` / `autolock-timeout` is **deferred** (see below).

## Scope of the MVP (what "done" means for draft1)

- Group values persist in the group's **Yjs document** — the `choices` Y.Map,
  serialized as base64 updates into the `ydoc_updates` collection (append-only,
  keyed by a server-derived room `${lesson}:${session}:${group}`). Yjs is a CRDT so
  concurrent edits merge; PB realtime fans updates to every group member. *(Originally
  a `decisions` collection with `/api/decisions/set`, since replaced.)*
- `POST /api/ydoc/push`, `GET /api/ydoc/pull?session_id=…` (see internal/httpapi/ydoc.go).
- `Choice.Slug`, `ChoiceGroup.Scope`, `ChoiceGroup.DefaultValueKey` parsed + seeded.
- Catalogue `print` widget (`from` required, `show` optional enum `label|slug|key`)
  + per-type `default_value_key` with a seed-time existence check.
- `LmsPrint.svelte` renders `values[from][show ?? default_value_key ?? "label"]`.
- Client merges session `work_state.choices` + the group's Yjs `choices` map into
  one `values` map (`stores/decisions.svelte.ts` → `choiceValues`); a pick routes to
  `work_state` (session) or the Yjs `choices` map (group) by the group's scope.
- Verify with the `human-topic` example across two sessions + two group members.

## `t_mapper` + `tabulate` + `captures` (implemented 2026-07-15)

A choice can drive a **table of bulk data**: `t_mapper` frontmatter indexes a data
file by the chosen value (`data[root][chosen-slug]`) and exposes it under an id;
`{{< LMS t="tabulate" from="…" >}}` renders it, live with the group's pick. Data is
served by a Hugo-style **assets pipeline** (`assets/**` → `pb_public/**`, `*.yaml`
→ pretty `/data/*.json`, in `internal/assetpipe`, run at seed). The derived output
is **captured** on navigation/exit into the `captures` collection
(`{session, group, lesson, key_id, scope, value}`, unique `(session,key_id)`, via
`POST /api/captures/set`) so a teacher can inspect what each group worked on and
agents can reuse it. Columns are inferred per-topic (fields differ) with an optional
`columns:` override. Mapper deferred bits are folded into the list below.

## ⚠️ Unfinished / deferred (the "what's left" list)

These are intentionally **not** in the MVP. The vocabulary is reserved in the
schema, but using them should fail loudly, not silently:

1. **Scopes `lesson`, `course`, `nabu`, `user`** — schema accepts the words but seed
   must **error** "scope not yet implemented" for them. No storage home exists yet
   (would need per-lesson/per-course/global shared records + read rules).
2. **`locked | editable`** — freeze a decision so it can't be changed after commit.
3. **`autolock-timeout`** — auto-freeze a decision N seconds after it's made
   (the "last click wins, then lock" behaviour).
4. **Generic token `type`** — `print` resolves only `choice` values for now. Other
   referenceable widgets (text field, timer result, agent output, …) are future;
   the catalogue's per-type `default_value_key` is the extension point.
5. **Cross-cohort isolation** — ✅ **RESOLVED** (2026-07-22). Groups are now keyed
   by `(lesson, session, tag)` and the Yjs room is `${lesson}:${session}:${group}`
   (see docs/glossary.md), so the same tag `g1` in two different Sessions of one
   lesson is two independent groups with independent shared state. Isolation no
   longer relies on group names being cohort-unique.
6. **Mapper `select`/`where` DSL** — `t_mapper` is index-only (`data[root][slug]`);
   row-level filtering (e.g. `where start > 1900`) is future.
7. **Agents reading the `captures` cache** server-side, and perf/caching of captures
   (currently written on navigation/exit, not read back for display); deeper maps;
   printing a derived scalar from a mapper.

## Key files (when implementing)

- `nabu/internal/content/{model.go,parse.go,shortcodes.go,nabu-widget-catalogue.yaml}`
- `nabu/internal/httpapi/ydoc.go` (Yjs push/pull) + `migrations/1751000019_ydoc_updates.go`
  (group values were originally a `decisions` collection, replaced by the Yjs doc;
  the collection is dropped in `1751000021_drop_decisions.go`)
- `nabu/internal/seed/seed.go` (persist choice slug/scope/default_value_key; seed
  the scope/default_value_key validations)
- `nabu/web/src/lib/lms/{registry.ts,LmsPrint.svelte}`,
  `nabu/web/src/routes/session/[id]/run/+page.svelte`, `Slide.svelte`, `types.ts`
