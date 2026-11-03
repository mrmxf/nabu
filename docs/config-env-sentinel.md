# Config `ENV` sentinel — "you decide what's secret"

Status: **planned — not yet implemented** (2026-07-14). A note for a future update
(see also docs/lesson-choice-values.md for the other deferred work).

## The idea

The number of secret-ish values in the config is growing, and *which* values are
secret is a per-deployment choice — some teams won't want email addresses,
usernames, or URLs checked into the repo, not just passwords. Rather than a
bespoke `*_env` field per secret, use one uniform formula.

Any leaf in `config/nabu-config.yaml` whose value is the literal string **`ENV`**
is fetched from the environment at parse time. The variable name is derived from
the leaf's fully-qualified dot-path:

```
env var = admin.env_prefix  +  UPPERCASE( dotpath with '.' → '_' )
```

Example — with `admin.env_prefix: NABU_`:

```yaml
pocketbase:
  smtp:
    username: ENV      # → read NABU_POCKETBASE_SMTP_USERNAME
    password: ENV      # → read NABU_POCKETBASE_SMTP_PASSWORD
```

The prefix carries its own separator (`NABU_`), so the join is a plain
concatenation. This lets a deployment move *any* field into the environment (via
a free Infisical account, say) without code changes.

## Implementation sketch

Substitution happens **only on the runtime path**, so secrets never enter the
display path (this is the masking decision — see below):

- **Parser — runtime path** (`nabu/embedfs.LoadConfig`): after kfg merges embedded
  + `--config` override (`loadRaw`), walk the merged map; for every string leaf
  equal to `"ENV"`, replace it with
  `os.Getenv(prefix + strings.ToUpper(strings.ReplaceAll(path, ".", "_")))`
  (`prefix` = `admin.env_prefix`) **before `kfg.Unmarshal`**. The typed `Config`
  gets real values.
- **Raw path** (`RawMerged`, used by `showconfig`): do **not** substitute — leave
  the `ENV` sentinels in place. See masking.
- **Missing var**: resolve to empty at parse time (don't hard-fail every command);
  let `validate` be the gate.
- **`nabu admin validate`**: collect every `ENV`-sentinel path, compute its var
  name, and **error if unset** — generalizes today's `password_env` checks
  (see `nabu/internal/admin/checks.go` `requireEnv`).

## Masking (decided: config-time separation, not a mask registry)

Anything sourced via the `env_prefix` formula must never print. Rather than track
a set of "secret paths" and redact them at log time, keep the two paths separate:

- The **runtime** config (typed `Config`) has resolved values — used to actually
  run; never serialized wholesale.
- The **raw** config that `showconfig` prints still holds the literal `ENV`, so
  secret values are masked **by construction** — there's no resolved value in that
  path to leak.
- At display, `showconfig` renders each `ENV` leaf as its derived var name, e.g.
  `«env: NABU_POCKETBASE_SMTP_USERNAME»` (computed from the path it's already
  walking) — so you see *where* the value comes from, never the value.

This also means no general log-redaction filter is needed for these fields.

## Decisions to make when building

1. **Relationship to `password_env`**: the sentinel subsumes it —
   `smtp.password: ENV` replaces `smtp.password_env: NAME`. Migrate the existing
   `password_env` fields (smtp, backups.s3, superuser) to the `ENV` sentinel, or
   support both for a transition?
2. **Only string leaves** equal to `"ENV"` are treated as sentinels; numbers/bools
   are untouched. Confirm no legitimate value is literally `"ENV"`.
3. **`env_prefix` normalization**: the example includes the trailing `_`. Either
   require the author to include a separator, or normalize (append `_` if absent).

(Masking is decided — see the Masking section above.)

## Files (when implementing)

- `nabu/embedfs/embedfs.go` (`loadRaw`/`RawMerged` — the walk + substitution)
- `nabu/internal/admin/checks.go` (validate: require the derived env vars)
- `nabu/internal/admin/showconfig.go` (masking)
