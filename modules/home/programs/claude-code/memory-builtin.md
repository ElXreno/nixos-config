## Memory
- **`MEMORY.md` is byte-capped and truncates from the tail** — order is priority order: rules first, then infra and active projects, reference clusters last. Topic files also surface via their own `description` at recall time, so they lose the least by sinking
- **Index entries are bare slugs, not markdown links.** A slug resolves as `memory/<line prefix><slug>.md`, where the prefix comes from the `(prefix_*)` header of that line; a slug with a different prefix is written in full and overrides the header
- Entries separated by ` · `, tightly-coupled companions by ` + `
- **Inline ` - verdict` only when it changes behavior** (a danger, a measured number, an `open`/`SUPERSEDED` status). Never restate the slug in words, that redundancy is what blows the cap. Plain hyphen, never an em-dash: the index is text I write, so `no_em_dashes` applies to it too
- A closed research cluster of >15 entries gets a hub file plus one index line; active work stays inline even inside such a cluster
- Reconcile both directions periodically: every slug must resolve to a real file, every file must be reachable from `MEMORY.md`, a hub, or the archive
- To convert a project still on the old markdown-link layout, use the `memory-migrate` skill — it snapshots reachability first and proves nothing was lost
