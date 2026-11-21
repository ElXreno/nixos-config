export const meta = {
  name: "arch-review",
  description:
    "Architecture review — mine git history for erosion evidence, diagnose design flaws, generate competing redesign proposals, red-team and judge them, deliver an ADR + Mikado-style migration plan.",
  whenToUse:
    "When the user is stuck fixing recurring micro-bugs in the same area (whack-a-mole), suspects the architecture itself is broken, or asks for a design-level review / simplification plan. Plan-only by default: args = problem description string, or {problem, paths}. After the user approves the returned plan, re-invoke with {execute: true, plan: <returned plan>} to apply it in an isolated worktree.",
  phases: [
    {
      title: "Evidence",
      detail:
        "Parallel git miners: hotspots+coupling, fix-commit classes, architecture map",
      model: "sonnet",
    },
    {
      title: "Diagnose",
      detail: "Map symptoms to design flaws, each tied to evidence",
      model: "opus",
    },
    {
      title: "Propose",
      detail:
        "3 independent proposals: tidy-first, incremental redesign, target architecture",
      model: "opus",
    },
    {
      title: "Red-team",
      detail: "Falsification attack on every proposal before judging",
      model: "opus",
    },
    {
      title: "Judge",
      detail:
        "Anonymized pairwise comparisons, both orders, red-team reports as grounding",
      model: "opus",
    },
    {
      title: "Synthesize",
      detail:
        "ADR + Mikado-style step plan from winner, grafting best ideas of losers",
      model: "opus",
    },
    {
      title: "Execute",
      detail:
        "Optional second run: sequential builder in isolated worktree, revert-on-red",
      model: "sonnet",
    },
  ],
};

// Evidence base (researched 2026-07-19): hotspots = churn x complexity (Tornhill);
// change coupling >=30% shared commits = hidden dependency; pairwise judging
// beats absolute scores (Open Proof Corpus) with both presentation orders to
// cancel position bias (no RNG in this realm); red-team precedes judging so
// verdicts are grounded in evidence ("No Free Labels"); deliverable is a plan,
// not a patch — human gate sits between plan and execute.

const A = typeof args === "string" ? { problem: args.trim() } : args || {};
const PROBLEM = (A.problem || "").trim();
const PATHS = Array.isArray(A.paths) ? A.paths : [];

// ─── Schemas ───
const METRICS_SCHEMA = {
  type: "object",
  required: ["hotspots", "coupling", "summary"],
  properties: {
    summary: { type: "string" },
    hotspots: {
      type: "array",
      maxItems: 10,
      items: {
        type: "object",
        required: ["file", "commits", "fixCommits", "note"],
        properties: {
          file: { type: "string" },
          commits: { type: "number" },
          fixCommits: { type: "number" },
          note: { type: "string" },
        },
      },
    },
    coupling: {
      type: "array",
      maxItems: 8,
      items: {
        type: "object",
        required: ["a", "b", "sharedPct", "revisions"],
        properties: {
          a: { type: "string" },
          b: { type: "string" },
          sharedPct: { type: "number" },
          revisions: { type: "number" },
          physicalDependency: { type: "boolean" },
        },
      },
    },
  },
};
const CLASSES_SCHEMA = {
  type: "object",
  required: ["classes", "summary"],
  properties: {
    summary: { type: "string" },
    classes: {
      type: "array",
      maxItems: 6,
      items: {
        type: "object",
        required: ["label", "count", "examples", "hypothesis"],
        properties: {
          label: { type: "string" },
          count: { type: "number" },
          examples: { type: "array", maxItems: 4, items: { type: "string" } },
          hypothesis: { type: "string" },
        },
      },
    },
  },
};
const ARCHMAP_SCHEMA = {
  type: "object",
  required: ["components", "dependencies", "summary"],
  properties: {
    summary: { type: "string" },
    boundaries: { type: "string" },
    components: {
      type: "array",
      maxItems: 12,
      items: {
        type: "object",
        required: ["name", "paths", "role"],
        properties: {
          name: { type: "string" },
          paths: { type: "array", items: { type: "string" } },
          role: { type: "string" },
        },
      },
    },
    dependencies: {
      type: "array",
      maxItems: 20,
      items: {
        type: "object",
        required: ["from", "to", "kind"],
        properties: {
          from: { type: "string" },
          to: { type: "string" },
          kind: { type: "string" },
        },
      },
    },
  },
};
const DIAGNOSIS_SCHEMA = {
  type: "object",
  required: ["flaws", "scopeStatement", "summary"],
  properties: {
    summary: { type: "string" },
    scopeStatement: { type: "string" },
    flaws: {
      type: "array",
      maxItems: 6,
      items: {
        type: "object",
        required: ["flaw", "smell", "evidence", "files", "severity"],
        properties: {
          flaw: { type: "string" },
          smell: { type: "string" },
          evidence: { type: "string" },
          files: { type: "array", items: { type: "string" } },
          severity: { enum: ["critical", "major", "minor"] },
        },
      },
    },
  },
};
const PROPOSAL_SCHEMA = {
  type: "object",
  required: [
    "title",
    "design",
    "migrationSteps",
    "noWorseMechanisms",
    "cost",
    "risks",
  ],
  properties: {
    title: { type: "string" },
    design: { type: "string" },
    migrationSteps: { type: "array", maxItems: 12, items: { type: "string" } },
    noWorseMechanisms: {
      type: "array",
      maxItems: 6,
      items: { type: "string" },
    },
    cost: { type: "string" },
    risks: { type: "array", maxItems: 6, items: { type: "string" } },
  },
};
const ATTACK_SCHEMA = {
  type: "object",
  required: ["findings", "fatal", "summary"],
  properties: {
    summary: { type: "string" },
    fatal: { type: "boolean" },
    findings: {
      type: "array",
      maxItems: 8,
      items: {
        type: "object",
        required: ["attack", "severity", "detail"],
        properties: {
          attack: { type: "string" },
          severity: { enum: ["fatal", "major", "minor"] },
          detail: { type: "string" },
        },
      },
    },
  },
};
const CRITERIA = {
  behaviorPreservation: { type: "number", minimum: 1, maximum: 5 },
  migrationRisk: { type: "number", minimum: 1, maximum: 5 },
  simplicityDelta: { type: "number", minimum: 1, maximum: 5 },
  reversibility: { type: "number", minimum: 1, maximum: 5 },
  flawCoverage: { type: "number", minimum: 1, maximum: 5 },
};
const VERDICT_SCHEMA = {
  type: "object",
  required: ["winner", "first", "second", "reasoning"],
  properties: {
    winner: { enum: ["first", "second"] },
    first: {
      type: "object",
      required: Object.keys(CRITERIA),
      properties: CRITERIA,
    },
    second: {
      type: "object",
      required: Object.keys(CRITERIA),
      properties: CRITERIA,
    },
    reasoning: { type: "string" },
  },
};
const PLAN_SCHEMA = {
  type: "object",
  required: ["adr", "steps", "openRisks", "summary"],
  properties: {
    summary: { type: "string" },
    adr: {
      type: "object",
      required: ["title", "context", "decision", "consequences"],
      properties: {
        title: { type: "string" },
        context: { type: "string" },
        decision: { type: "string" },
        consequences: { type: "string" },
        alternativesRejected: { type: "string" },
      },
    },
    steps: {
      type: "array",
      maxItems: 15,
      items: {
        type: "object",
        required: ["id", "title", "detail", "prereqs", "noWorse"],
        properties: {
          id: { type: "string" },
          title: { type: "string" },
          detail: { type: "string" },
          prereqs: { type: "array", items: { type: "string" } },
          noWorse: { type: "string" },
        },
      },
    },
    openRisks: { type: "array", maxItems: 6, items: { type: "string" } },
  },
};
const EXEC_SCHEMA = {
  type: "object",
  required: ["worktree", "steps", "testsGreen", "summary"],
  properties: {
    worktree: { type: "string" },
    testsGreen: { type: "boolean" },
    summary: { type: "string" },
    steps: {
      type: "array",
      items: {
        type: "object",
        required: ["id", "status", "note"],
        properties: {
          id: { type: "string" },
          status: { enum: ["done", "blocked", "skipped"] },
          note: { type: "string" },
        },
      },
    },
  },
};
const EXEC_REVIEW_SCHEMA = {
  type: "object",
  required: ["approved", "findings", "summary"],
  properties: {
    approved: { type: "boolean" },
    summary: { type: "string" },
    findings: { type: "array", maxItems: 8, items: { type: "string" } },
  },
};

const CONTEXT_BLOCK =
  (PROBLEM
    ? "## Reported problem\n" + PROBLEM + "\n\n"
    : "## No problem statement given\nSelf-discovery mode: survey the whole repository and let the git evidence point at the worst area.\n\n") +
  (PATHS.length ? "## Focus paths\n" + PATHS.join("\n") + "\n\n" : "");

// ─── Execute mode: second invocation after human approval of the plan ───
if (A.execute) {
  if (!A.plan || !Array.isArray(A.plan.steps) || A.plan.steps.length === 0) {
    return {
      error:
        "execute: true requires plan: <object returned by the plan run> with non-empty steps.",
    };
  }
  phase("Execute");
  const stepsBlock = A.plan.steps
    .map(
      (s) =>
        "### " +
        s.id +
        ": " +
        s.title +
        "\nPrereqs: " +
        (s.prereqs.length ? s.prereqs.join(", ") : "none") +
        "\n" +
        s.detail +
        "\nNo-worse mechanism: " +
        s.noWorse,
    )
    .join("\n\n");
  const build = await agent(
    "## Migration builder (Mikado discipline)\n\n" +
      "You are in a fresh git worktree. Execute this approved migration plan step by step.\n\n" +
      "## ADR\n" +
      A.plan.adr.title +
      "\n" +
      A.plan.adr.decision +
      "\n\n## Steps (dependency order)\n" +
      stepsBlock +
      "\n\n## Rules\n" +
      "1. One step at a time, prereqs first. Skip a step whose prereq is blocked.\n" +
      "2. After each step: run the project's build/tests (check flake.nix / justfile / Makefile / package.json for the command). Green -> `git commit` that step alone (conventional title, no body).\n" +
      "3. Red and not quickly fixable -> `git checkout .` to revert JUST this step, mark it blocked with the exact error, move on. Never leave the tree red between steps.\n" +
      "4. Never push. Never touch files outside the plan's scope.\n" +
      "5. Report the worktree path (`git rev-parse --show-toplevel`) and per-step status.\n\nStructured output only.",
    {
      label: "build",
      phase: "Execute",
      schema: EXEC_SCHEMA,
      model: "sonnet",
      effort: "medium",
      isolation: "worktree",
    },
  );
  if (!build) return { error: "Builder agent returned no result." };
  const review = await agent(
    "## Execution reviewer\n\n" +
      "A builder executed an approved migration plan in worktree: " +
      build.worktree +
      "\nBuilder report: " +
      JSON.stringify(build.steps) +
      "\n\n## Task\n" +
      "cd into the worktree. Inspect `git log` + `git diff main...HEAD`. Verify: every 'done' step matches its plan intent, no out-of-scope edits, no-worse mechanisms present (run tests yourself), no test-gaming (assertions rewritten to match broken behavior). Be adversarial: approved=false on any material mismatch.\n\nStructured output only.",
    {
      label: "review-exec",
      phase: "Execute",
      schema: EXEC_REVIEW_SCHEMA,
      model: "opus",
      effort: "high",
    },
  );
  return {
    mode: "execute",
    worktree: build.worktree,
    steps: build.steps,
    testsGreen: build.testsGreen,
    review: review || {
      approved: false,
      findings: ["review agent failed"],
      summary: "",
    },
    next: "Main agent: summarize the review verdict and per-step status to the user. On their approval merge the worktree branch yourself (git merge <branch> from the repo root, then git worktree remove <path>); if review.approved is false or steps are blocked, propose plan revision instead of merging.",
  };
}

// ─── Phase 1: Evidence — parallel git miners ───
phase("Evidence");
const [metrics, classes, archmap] = await parallel([
  () =>
    agent(
      "## Metrics miner\n\n" +
        CONTEXT_BLOCK +
        "## Task\nMine this repository's git history (last ~18 months, `git log --format='%H %s' --name-only`, use `-n 100000` style limits, never bare `git log`).\n" +
        "1. Hotspots: files ranked by commit count, weighted toward commits whose message matches fix/bug/hotfix/regression (recent fixes weigh more than old ones).\n" +
        "2. Change coupling: file pairs changed in the same commit in >=30% of either file's commits, min 5 shared revisions. For each pair check whether a physical dependency exists (import/include/reference via rg) — coupling WITHOUT physical dependency is the smell.\n" +
        "Exclude lockfiles, vendored code, generated files.\n\nStructured output only.",
      {
        label: "metrics",
        phase: "Evidence",
        schema: METRICS_SCHEMA,
        model: "sonnet",
        effort: "medium",
      },
    ),
  () =>
    agent(
      "## Fix-commit classifier\n\n" +
        CONTEXT_BLOCK +
        "## Task\nCollect the last ~80 commits whose message matches fix/bug/hotfix/regression" +
        (PATHS.length ? " touching the focus paths" : "") +
        ". Read the diffs of a representative sample (`git show`). Group them into recurring bug CLASSES (same underlying mistake, not same file): e.g. 'stale state after partial update', 'divergent duplicated validation'. For each class: count, example SHAs, and a hypothesis for the DESIGN cause behind the recurrence.\n\nStructured output only.",
      {
        label: "fix-classes",
        phase: "Evidence",
        schema: CLASSES_SCHEMA,
        model: "sonnet",
        effort: "medium",
      },
    ),
  () =>
    agent(
      "## Architecture mapper\n\n" +
        CONTEXT_BLOCK +
        "## Task\nMap the " +
        (PATHS.length || PROBLEM ? "affected subsystem" : "repository") +
        ": components (name, paths, role), dependencies between them (imports, data flow, runtime calls), and where the intended boundaries are unclear or violated. Read the code, not just the tree. Note cycles and hub components explicitly.\n\nStructured output only.",
      {
        label: "arch-map",
        phase: "Evidence",
        schema: ARCHMAP_SCHEMA,
        model: PROBLEM || PATHS.length ? "sonnet" : "opus",
        effort: "medium",
      },
    ),
]);
if (!metrics && !classes && !archmap) {
  return { error: "All evidence miners failed — nothing to diagnose." };
}
const EVIDENCE =
  "## Evidence: metrics\n" +
  JSON.stringify(metrics) +
  "\n\n## Evidence: recurring fix classes\n" +
  JSON.stringify(classes) +
  "\n\n## Evidence: architecture map\n" +
  JSON.stringify(archmap);
log(
  "Evidence: " +
    (metrics
      ? metrics.hotspots.length +
        " hotspots, " +
        metrics.coupling.length +
        " coupled pairs"
      : "metrics failed") +
    "; " +
    (classes ? classes.classes.length + " bug classes" : "classifier failed"),
);

// ─── Phase 2: Diagnose ───
phase("Diagnose");
const diagnosis = await agent(
  "## Architecture diagnostician\n\n" +
    CONTEXT_BLOCK +
    EVIDENCE +
    "\n\n## Task\nMap the symptoms to named design flaws (divergent change, shotgun surgery, cyclic dependency, hub-like dependency, god component, leaky boundary, wrong data model...). Every flaw MUST cite concrete evidence above (files, coupling pairs, bug classes) — no flaw without evidence. Verify each candidate flaw against the actual code before asserting it. Then write a scopeStatement: one paragraph defining exactly what the redesign proposals must fix.\n\nStructured output only.",
  {
    label: "diagnose",
    phase: "Diagnose",
    schema: DIAGNOSIS_SCHEMA,
    model: "opus",
    effort: "high",
  },
);
if (!diagnosis) return { error: "Diagnosis agent failed." };
log(
  "Diagnosis: " +
    diagnosis.flaws.map((f) => f.smell + " (" + f.severity + ")").join(", "),
);

// ─── Phase 3: Propose — 3 independent lenses ───
phase("Propose");
const LENSES = [
  {
    key: "tidy-first",
    brief:
      "MINIMAL SIMPLIFICATION lens (Kent Beck tidy-first): smallest set of behavior-preserving tidyings that makes the recurring changes easy. No new abstractions unless they delete complexity. Bias: boring, incremental, this-week-sized.",
  },
  {
    key: "incremental-redesign",
    brief:
      "INCREMENTAL REDESIGN lens (strangler fig / branch-by-abstraction / expand-contract): introduce the right seam, migrate call sites gradually, contract the old path as an explicit tracked final step. Every step reversible, old path alive until cutover.",
  },
  {
    key: "target-architecture",
    brief:
      "TARGET ARCHITECTURE lens: design the structure this subsystem should have had, then derive a realistic migration path to it. Ambition allowed, hand-waving not: the migration path must be as concrete as the design.",
  },
];
const proposals = (
  await parallel(
    LENSES.map(
      (l) => () =>
        agent(
          "## Redesign proposer — " +
            l.brief +
            "\n\n" +
            CONTEXT_BLOCK +
            EVIDENCE +
            "\n\n## Diagnosis\n" +
            JSON.stringify(diagnosis) +
            "\n\n## Task\nProduce ONE proposal through your lens. Requirements:\n" +
            "1. design: concrete target structure (name real files/modules from the map).\n" +
            "2. migrationSteps: ordered, each independently shippable, tree green after each.\n" +
            "3. noWorseMechanisms: executable guarantees per risky step — characterization/golden-master tests pinning current behavior BEFORE touching it, ratchet checks, revert points. 'We will be careful' does not count.\n" +
            "4. cost: honest size estimate (files touched, sessions of work).\n" +
            "5. risks: what breaks if you are wrong, including hidden-coupling and stalled-migration risk.\n" +
            "Read the actual code before proposing. Address every critical/major flaw in the diagnosis or state explicitly why you leave it.\n\nStructured output only.",
          {
            label: "propose:" + l.key,
            phase: "Propose",
            schema: PROPOSAL_SCHEMA,
            model: "opus",
            effort: "high",
          },
        ).then((p) => (p ? { lens: l.key, ...p } : null)),
    ),
  )
).filter(Boolean);
if (proposals.length < 2) {
  return {
    error:
      "Fewer than 2 proposals survived generation — judging is meaningless. Got: " +
      proposals.length,
    diagnosis,
  };
}

// ─── Phase 4: Red-team every proposal before judging ───
phase("Red-team");
const attacked = await parallel(
  proposals.map(
    (p, i) => () =>
      agent(
        "## Red-team attacker\n\nTry to KILL this redesign proposal. Be maximally adversarial.\n\n" +
          "## Diagnosis it claims to fix\n" +
          JSON.stringify(diagnosis) +
          "\n\n## Proposal\n" +
          JSON.stringify({ ...p, lens: undefined }) +
          "\n\n## Attack checklist\n" +
          "1. Behavior loss: which current behaviors (incl. ugly load-bearing ones) does the migration silently drop? Check the real code.\n" +
          "2. Hidden coupling: which consumers/paths does it miss (rg for real call sites)?\n" +
          "3. Stalled migration: if this stops halfway, is the halfway state worse than today?\n" +
          "4. Overengineering: does any step add abstraction without deleting complexity?\n" +
          "5. No-worse mechanisms: are they actually executable and sufficient, or decorative?\n" +
          "6. Cost realism: is the estimate off by >2x?\n" +
          "fatal=true only for flaws that invalidate the proposal as designed.\n\nStructured output only.",
        {
          label: "redteam:" + i,
          phase: "Red-team",
          schema: ATTACK_SCHEMA,
          model: "opus",
          effort: "high",
        },
      ).then((a) => ({
        proposal: p,
        attack: a || { findings: [], fatal: false, summary: "attacker failed" },
      })),
  ),
);

// ─── Phase 5: Judge — anonymized pairwise, both orders ───
phase("Judge");
const pairs = [];
for (let i = 0; i < attacked.length; i++)
  for (let j = 0; j < attacked.length; j++) if (i !== j) pairs.push([i, j]);
const anon = (x) =>
  JSON.stringify({ ...x.proposal, lens: undefined, title: undefined });
const verdicts = (
  await parallel(
    pairs.map(
      ([i, j]) =>
        () =>
          agent(
            "## Architecture judge — pairwise\n\nTwo anonymous redesign proposals for the same diagnosed problem. Pick the one a senior engineer should bet on.\n\n" +
              "## Diagnosis\n" +
              JSON.stringify(diagnosis) +
              "\n\n## Proposal FIRST\n" +
              anon(attacked[i]) +
              "\nRed-team findings on FIRST: " +
              JSON.stringify(attacked[i].attack) +
              "\n\n## Proposal SECOND\n" +
              anon(attacked[j]) +
              "\nRed-team findings on SECOND: " +
              JSON.stringify(attacked[j].attack) +
              "\n\n## Rubric — score each proposal 1-5 per criterion (anchors: 1 = clearly fails it, 3 = acceptable with caveats, 5 = exemplary)\n" +
              "- behaviorPreservation: will current behavior survive the migration?\n" +
              "- migrationRisk: 5 = every step small and shippable, 1 = big-bang in disguise.\n" +
              "- simplicityDelta: net complexity removed from the system.\n" +
              "- reversibility: can any step be abandoned without being worse than today?\n" +
              "- flawCoverage: are the diagnosed critical/major flaws actually fixed?\n" +
              "Score each criterion independently; do not average by vibe. Red-team fatal findings must dominate. winner = overall bet, not score sum.\n\nStructured output only.",
            {
              label: "judge:" + i + "v" + j,
              phase: "Judge",
              schema: VERDICT_SCHEMA,
              model: "opus",
              effort: "high",
            },
          ).then((v) => (v ? { i, j, v } : null)),
    ),
  )
).filter(Boolean);
const wins = attacked.map(() => 0);
for (const { i, j, v } of verdicts) wins[v.winner === "first" ? i : j]++;
const ranking = attacked
  .map((a, idx) => ({
    idx,
    lens: a.proposal.lens,
    wins: wins[idx],
    fatal: a.attack.fatal,
  }))
  .sort((a, b) => b.wins - a.wins);
log(
  "Judging: " +
    ranking
      .map((r) => r.lens + "=" + r.wins + (r.fatal ? " (FATAL)" : ""))
      .join(", "),
);
const winner = attacked[ranking[0].idx];

// ─── Phase 6: Synthesize — ADR + Mikado plan ───
phase("Synthesize");
const plan = await agent(
  "## Plan synthesizer\n\n" +
    CONTEXT_BLOCK +
    "## Diagnosis\n" +
    JSON.stringify(diagnosis) +
    "\n\n## Winning proposal (" +
    ranking[0].wins +
    " pairwise wins)\n" +
    JSON.stringify(winner.proposal) +
    "\n\n## Red-team findings on winner (must be mitigated in the plan)\n" +
    JSON.stringify(winner.attack) +
    "\n\n## Losing proposals (graft their best ideas where they beat the winner)\n" +
    JSON.stringify(
      ranking.slice(1).map((r) => ({
        proposal: attacked[r.idx].proposal,
        attack: attacked[r.idx].attack,
        wins: r.wins,
      })),
    ) +
    "\n\n## Task\nProduce the final deliverable:\n" +
    "1. adr: Nygard-style — context (diagnosis essence), decision, consequences (negatives included), alternativesRejected (the losing lenses and why).\n" +
    "2. steps: Mikado-style dependency-ordered graph. Each step: independently shippable, prereqs listed, and a noWorse mechanism (which characterization test / ratchet / revert point guards it). First steps must pin current behavior with tests BEFORE any restructuring. The contract/cleanup step that deletes the old path must be an explicit tracked step, never implied.\n" +
    "3. openRisks: what the plan does NOT guarantee, including every unmitigated red-team finding.\n\nStructured output only.",
  {
    label: "synthesize",
    phase: "Synthesize",
    schema: PLAN_SCHEMA,
    model: "opus",
    effort: "high",
  },
);
if (!plan) {
  return {
    error: "Synthesis failed — returning raw winner instead of a plan.",
    diagnosis,
    winner: winner.proposal,
    redTeam: winner.attack,
    ranking,
  };
}

return {
  mode: "plan",
  diagnosis,
  ranking,
  adr: plan.adr,
  steps: plan.steps,
  openRisks: plan.openRisks,
  summary: plan.summary,
  redTeamWinner: winner.attack,
  next: "Review the plan. To apply it: Workflow({name: 'arch-review', args: {execute: true, plan: <this result>}}) — runs a builder in an isolated worktree, you merge manually.",
  stats: {
    proposals: proposals.length,
    pairwiseComparisons: verdicts.length,
    agents: 3 + 1 + proposals.length + attacked.length + verdicts.length + 1,
  },
};
