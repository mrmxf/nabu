#!/usr/bin/env bash
# check-widget-docs.sh — every LMS widget in the catalogue must be documented in
# the built-in help, and the help must not describe widgets that don't exist.
#
# The widget catalogue (internal/content/nabu-widget-catalogue.yaml) is the
# single source of truth for what exists; the help lesson is what a teacher
# actually reads. They drift silently — a widget added without docs is invisible,
# and a widget documented after being removed is worse than no docs. This makes
# the drift a build failure instead.
#
# Silent on success, like `nabu admin validate -q`.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
catalogue="$root/internal/content/nabu-widget-catalogue.yaml"
doc="$root/help/using-nabu/authoring-lessons/40-widgets.md"

python3 - "$catalogue" "$doc" <<'PY'
import re, sys

catalogue, doc = sys.argv[1], sys.argv[2]

# Widget names are the keys under `widgets:` — two-space-indented, at the top of
# the block. Parsed by hand so this script needs no yaml module.
text = open(catalogue, encoding="utf-8").read()
body = text.split("\nwidgets:\n", 1)[1]
have = set(re.findall(r"(?m)^  ([a-z][a-z0-9-]*):$", body))

# A widget counts as documented if the help names it anywhere as a backticked
# word or in a `t="x"` example. Deliberately generous: the aim is to catch a
# widget nobody mentioned at all, not to police how it is described.
d = open(doc, encoding="utf-8").read()
# The lookbehind matters: without it, `start="onshow"` matches as t="onshow".
used_as_t = set(re.findall(r'(?<![a-zA-Z0-9_])t\s*=\s*"([a-z][a-z0-9-]*)"', d))
documented = used_as_t | (set(re.findall(r"`([a-z][a-z0-9-]*)`", d)) & have)

missing = sorted(have - documented)
# The reverse check is deliberately narrow: only a `t="x"` example is an
# unambiguous claim that widget x exists. Backticked prose is full of param
# names (`id`, `tone`, `scope`) and would produce noise.
unknown = sorted(used_as_t - have)

fail = False
if missing:
    print("undocumented widgets (add them to 40-widgets.md):", ", ".join(missing))
    fail = True
if unknown:
    print("documented but not in the catalogue:", ", ".join(unknown))
    fail = True
sys.exit(1 if fail else 0)
PY
