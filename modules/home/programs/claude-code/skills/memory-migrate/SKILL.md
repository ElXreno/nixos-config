---
name: memory-migrate
description: "Migrate a project's Claude Code memory index (MEMORY.md) from markdown-link entries to the compact bare-slug format, reconcile orphaned memory files, and split closed research clusters into hub files. Use when MEMORY.md is approaching the read cap, when a memory-index hook warns about size, when the user asks to compact or reorganize memory, or when bringing another project onto the current memory layout."
---

# Memory index migration

Rewrite `MEMORY.md` from the old markdown-link layout to the bare-slug layout, without losing a single entry.

`MEMORY.md` is loaded whole every session and is byte-capped. On overflow the harness drops the **tail**, so the newest memories — active work — vanish first. Everything here buys headroom for that.

## Target format

```text
Files: `memory/<slug>.md`. The prefix in parentheses after a line's header completes the bare slugs on that line; a slug carrying a different prefix is written in full. Rules go first, because the index is truncated from the tail.

- Rules·git (feedback_*): amend_followups · commit_title_only · no_push_default
- Infra·ci (reference_*): runner_cache_layout · project_pipeline_split · flaky_integration_suite - retries hide a real race
- Research·storage-tuning → reference_storage_tuning_hub - 26 files: patches, measurements, bugs
```

Rules:

- **No markdown links.** A bare slug resolves as `memory/<line prefix><slug>.md`.
- **`(prefix_*)` header** carries the dominant prefix of that line. A slug with a different prefix is written in full and overrides the header — both forms appear on the `Infra·ci` line above.
- Entries separated by ` · `; tightly-coupled companions by ` + `.
- **Inline ` - verdict` only when it changes behavior**: a danger, a measured number, an `open`/`SUPERSEDED`/`UNCOMMITTED` status. Never restate the slug in words, that is the redundancy the migration removes. Use a plain hyphen: the index is text written for the user, so the no-em-dash rule covers it.
- **Order is priority order**, because truncation eats the tail: rules first, then infra and active projects, reference clusters last. Reference files also surface on their own `description` at recall time, so they lose the least by sinking.
- Keep the first line — it is the resolution rule for a reader who has never seen this layout.

The example above is English, but write the index — including that first resolution line — in whatever language the existing one uses. Never translate existing entries.

## Procedure

`memory-reconcile` is on `PATH`; no interpreter prefix is needed. Let `$MEM` stand for the target memory directory, `~/.claude/projects/<slug>/memory/` — substitute it by hand, since shell state does not persist between tool calls.

### 1. Audit

```bash
memory-reconcile audit "$MEM"
```

Reports the current format, byte breakdown, detected hubs, and — critically — **orphans** (files reachable from nowhere) and **body-only** files (reachable only via `[[wikilink]]`). Both are unrecallable in practice and must be folded into the new index.

If the format already reads `NEW (bare slugs)` with no orphans, stop and say so.

### 2. Snapshot

```bash
memory-reconcile snapshot "$MEM" /tmp/mem-baseline.json
```

This records every file that must stay reachable. Do this **before** touching `MEMORY.md` — it is the only proof that the rewrite lost nothing.

### 3. Read the whole index

Read `MEMORY.md` in full. Do not rewrite it line by line from a script: deciding which verdicts survive requires judgement about what changes behavior. Scripted regex edits on this file silently mangle mixed-script labels.

### 4. Decide the hubs

A **closed** research cluster of more than ~15 entries gets its own hub file and one line in the index. Judge by hit rate, not size:

- Rules must fire unprompted → always inline, never hub.
- Repo and infra facts you need while working → inline.
- A concluded investigation you only open when the topic resurfaces → hub.
- **Active** work stays inline even inside a hub-worthy cluster. Split the cluster if needed: operational recipes inline, the research record in the hub.

A hub is an ordinary memory file with frontmatter, a short preamble stating that entries moved out of the always-loaded index, and backticked slugs grouped under headings. `memory-reconcile` auto-detects any indexed file that itself references ≥5 memory files as a hub and follows it.

### 5. Rewrite

Write the new `MEMORY.md` and any hub files. Fold every orphan and body-only file onto the right line as you go.

### 6. Verify

```bash
memory-reconcile verify "$MEM" /tmp/mem-baseline.json
```

Must print `PASS` with `LOST none`, `dangling refs none`, `unresolved slugs none`, and empty `orphans left` / `body-only left`. `unresolved slugs` means a bare slug did not resolve under the prefix rule — a typo or a wrong `(prefix_*)` header, and the entry is now unreachable. Fix and re-run until clean.

Report the before/after byte count.

### 7. Pin the format

If the project has a memory-index rule of its own (commonly `feedback_memory_index_concise`), update it to describe this format, or the next session reverts to markdown links. This step is not optional — it is what makes the migration stick.

## Expected result

A migrated index typically loses 40-50% of its bytes with zero entries dropped, plus however many orphans the audit turns up. Before the rewrite the breakdown is usually around 55% filenames plus `[]()` syntax and 35% labels — those labels being mostly the slug spelled out with spaces, which is the redundancy being removed.

A smaller saving is not automatically a failure. An index dense with genuine verdicts — ports, thresholds, expiry dates, open/closed statuses — legitimately compresses less than one whose labels merely spelled out the slug. Work out which case you are in before pruning anything: a verdict that answers a question without opening the file is worth more than the bytes it costs, and stripping those is a loss `verify` cannot catch, because every file is still reachable.

The audit's `bare-slug est.` line is not a lower bound — hoisting prefixes into the line headers routinely lands below it.
