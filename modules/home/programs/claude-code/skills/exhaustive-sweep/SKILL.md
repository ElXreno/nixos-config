---
name: exhaustive-sweep
description: "Exhaustive branch-by-branch coverage of a large unknown candidate space, in hardware or software. Use for open-ended 'why does X happen' questions — battery drain, latency spikes, memory growth, a perf regression, a flaky test, mystery wakeups — and for 'what would this affect' sweeps such as migration blast radius, call-site audits, or exposure reviews. Enumerates every candidate before measuring anything, keeps a machine-checked ledger, and closes a branch only against evidence. Do NOT use when the site of the problem is already known; that is ordinary debugging."
---

# Exhaustive sweep

For questions where the answer could be almost anywhere and the honest starting position is "I don't know yet which part". The user has explicitly traded time for coverage here: a run may last hours or days. Depth is not the constraint. Stopping early is the failure.

The mechanic is domain-neutral. A branch is a candidate: a subsystem that could be draining power, an allocation site that could be leaking, a service that could be adding latency, a call site a migration could break, a surface an audit must cover. What differs between domains is how a branch is enumerated and what counts as a measurement — not the protocol.

## The failure this exists to prevent

You find a plausible contributor, it explains part of the symptom, and you stop. The branches you never opened are nowhere on record, so their absence does not read as incompleteness — not to you, and not to the user reading your report. A ranked `powertop` list looks like an answer, which is exactly why it ends investigations that should have kept going.

So: **enumerate before you measure, and close nothing by reasoning.**

## Protocol

### 1. Build the tree first

Before running a single diagnostic command, write out every subsystem that could plausibly contribute. Cast wide — a branch costs one line now and one measurement later; a missing branch costs the whole investigation.

Enumerate by walking the stack layer by layer, not by recalling what usually goes wrong. Two worked examples of what "wide enough" means:

*Power drain on a laptop*: package and core C-state residency, cpuidle governor behaviour, EPP and frequency floor, wakeups per second and their sources, dGPU runtime PM and D3cold entry, GPU power profile and display state (PSR, panel refresh, vblank), NVMe APST, PCIe ASPM per device, s2idle/S0ix residency, WiFi power save and driver-level doze, wired NIC runtime PM, USB and Thunderbolt autosuspend, systemd timers and polling daemons, thermal and fan policy, platform profile and firmware settings, battery wear and charge thresholds.

*A service that got slower after a deploy*: the diff itself, dependency version drift, query plans and index usage, connection pool saturation, cache hit ratio and eviction, N+1 call patterns, serialization and payload size, GC pause distribution and heap growth, thread pool and lock contention, retry and timeout policy, sidecar and proxy overhead, DNS and TLS handshake cost, node-level CPU throttling and cgroup limits, noisy neighbours, clock skew in the measurement itself.

Then add the branches nobody lists: what changed recently — kernel, firmware, dependency, config generation, infrastructure — what is unique to this instance rather than general to the system, and what the symptom would look like if the measurement tool itself were lying.

### 2. Check what is already settled

Search memory before measuring. Some branches have been investigated before and carry a verdict with a date. Record them in the ledger with that verdict and treat them as provisionally closed — then re-verify the ones whose verdict predates the change you are investigating. A settled branch reopened blindly wastes a day; a settled branch trusted blindly hides a regression.

### 3. Create the ledger and hand the user their `/goal`

`sweep-ledger` is on `PATH`; no interpreter prefix is needed.

```bash
sweep-ledger init ~/.claude/investigations/<slug>.json --topic "<the question>"
```

`init` prints a ready-made `/goal` line between markers. **Show it to the user verbatim and tell them to paste it.** You cannot set a goal yourself; without it, nothing stops you from quitting early, which is the entire problem. Re-print it any time with `sweep-ledger goal <file>`.

The ledger must live on persistent storage. `init` refuses any path whose filesystem is `tmpfs`, `ramfs` or `devtmpfs` — checking the actual mount rather than the path, because on an impermanent root far more than `/tmp` is volatile. A sweep can outlive a reboot, and the reboot is sometimes the experiment.

`init` also sets a floor on how many branches must exist before anything may be measured — 12 by default, `--min N` to change it. Add every branch from step 1 before measuring:

```bash
sweep-ledger add <file> nvme-apst "NVMe APST deepest state never entered" --why "would show as steady controller draw at idle"
```

### 4. Breadth before depth

`probe` and `close` refuse while the branch count is under the floor. That refusal is the point: the first branch you measure is the one you will anchor on, so the tree has to be finished before any of it looks interesting. A whole-machine question usually lands at 15-25 branches; three means you listed the usual suspects and called it a space. If the question really is narrow enough to justify fewer, lower the floor explicitly with `sweep-ledger floor <file> --set N` and say in the report why the space is that small.

Then one cheap measurement per branch, all branches, before going deep on any of them. Rank by observed signal, not by how interesting the branch feels. The first branch that shows something is not the answer; it is the first branch that shows something.

Record each probe:

```bash
sweep-ledger probe <file> nvme-apst --measurement "nvme get-feature /dev/nvme0 -f 0x0c -H" --result "APST enabled, max latency 25ms"
```

### 5. Validate the instrument before you trust the number

Confirm the binary exists. Run the measurement against a case whose answer you already know, and print both. An anomalous number means a broken tool until proven otherwise. A measurement you cannot sanity-check does not close a branch.

### 6. Close only against evidence

```bash
sweep-ledger close <file> nvme-apst --verdict ruled-out \
  --measurement "powertop --time=60 idle sample, controller in PS4" \
  --evidence "0.02W of a 9.1W total — below noise"
```

`close` refuses without both a measurement and a result. That refusal is the feature. "Unlikely", "probably fine", "standard configuration" are not closures. Use `inconclusive` when the branch resisted measurement, and say what instrument would settle it — an inconclusive branch is an honest open question, a falsely ruled-out one is a lie in the report.

New information reopens a branch: `sweep-ledger reopen <file> <id> --why "..."`.

### 7. Print the status line every turn

```bash
sweep-ledger status <file>
```

The goal evaluator only sees what lands in the conversation. The `LEDGER ... EXHAUSTED=no` line is what keeps it running; omit it and the loop ends on a technicality.

### 8. Keep going until the tree stops growing

Measuring one branch teaches you about others — a wakeup trace names a driver you never listed, a flame graph names a library you did not know was in the path. Add it. Any `add` resets the breadth counter, so a growing tree cannot be exhausted by definition.

### 9. Breadth re-check, then report

`EXHAUSTED=yes` needs one more thing after every branch is closed: a re-check that found nothing new. Reread the whole tree and ask, in writing — which branch closed without a number? which measurement modality was never run at all: tracing, sampling, counters, A/B by disabling, bisection across generations or versions? what would the answer look like if the leading contributor were wrong? which part of the stack has no branch pointing at it?

Record the outcome honestly:

```bash
sweep-ledger breadth <file> --added 3   # found more — add them and re-check
sweep-ledger breadth <file> --added 0   # genuinely nothing new; this is what flips EXHAUSTED
```

A re-check that finds something is a good re-check, not a setback. Reaching `--added 0` on the first try usually means the pass was performed as a formality.

## Reporting

`sweep-ledger table <file>` renders the full branch table. Lead with the contributing branches and their measured magnitudes, then the ruled-out ones in one line each — the ruled-out list is a large part of the value, because it is what stops the next investigation from re-treading them. List inconclusive branches separately with the instrument that would settle each.

Never present a contributor without a number attached. "Wi-Fi power save was off" is a finding only with the watts it cost.

When the run ends, offer to write the durable verdicts to memory so the next investigation starts from step 2 rather than step 1.
