import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { GoalDraft, GoalItem, GoalPhase, GoalRun, GoalSettings } from '../types'

const PANE = 'goal'
const DRAFT_TOOL = 'mcp__goal__draft'
const AUDIT_TOOL = 'mcp__goal__audit'
const AUDITOR = 'goal:auditor'
const GOAL_LIMIT = 4000
const CLEAR = new Set(['clear', 'stop', 'off', 'reset', 'none', 'cancel'])
const FORCED_CLEAR = /^Goal cleared after [^(]*\(([^)]+)\)/
const DEFAULTS: GoalSettings = { auditor: false, maxTurns: 40, maxTokens: 0 }

const phaseAtom = atom({ plugin: 'goal', key: 'phase' } as const, 'idle' as GoalPhase)
const draftAtom = atom({ plugin: 'goal', key: 'draft' } as const, null as GoalDraft | null, {
  shape: 'draft-1',
})
const settingsAtom = atom({ plugin: 'goal', key: 'settings' } as const, DEFAULTS, {
  shape: 'settings-1',
})
const runAtom = atom({ plugin: 'goal', key: 'run' } as const, null as GoalRun | null, {
  shape: 'run-4',
})
const logAtom = atom({ plugin: 'goal', key: 'log' } as const, [] as string[], { shape: 'log-1' })

const TURN_OPTIONS = [0, 10, 20, 40, 80, 160]
const TOKEN_OPTIONS = [0, 200_000, 500_000, 1_000_000, 2_000_000, 5_000_000]

const DRAFT_SCHEMA = {
  type: 'object',
  properties: {
    objective: {
      type: 'string',
      description: 'One sentence naming one measurable end state.',
    },
    acceptance: {
      type: 'array',
      description: 'Acceptance items, each a concrete check and the exact way to prove it.',
      items: {
        type: 'object',
        properties: {
          check: { type: 'string', description: 'What must hold.' },
          verify: {
            type: 'string',
            description: 'The exact command or observation that proves it, with the expected result.',
          },
        },
        required: ['check', 'verify'],
      },
    },
    constraints: {
      type: 'array',
      items: { type: 'string' },
      description: 'What must not break or change on the way.',
    },
    notes: {
      type: 'string',
      description: 'Assumptions you made and open risks, for the person.',
    },
  },
  required: ['objective', 'acceptance'],
}

const DRAFT_TOOL_DESCRIPTION =
  'Shows a /goal draft in the goal pane for the person to review, edit and start. ' +
  'Call it while drafting a goal, and again with the whole updated draft after each revision the person asks for. ' +
  'After calling it, stop and wait: the person starts, edits or cancels the goal in the pane.'

const AUDIT_TOOL_DESCRIPTION =
  'Runs the read-only goal auditor on the running goal\'s exact condition and the session transcript, and returns its report ending in VERDICT: PASS or VERDICT: FAIL. ' +
  'Call it once every other acceptance item is done, with a short pointer to where the evidence is. It is the only way to run the auditor.'

const AUDIT_SCHEMA = {
  type: 'object',
  properties: {
    evidence: {
      type: 'string',
      description: 'Where the evidence for each item is: commands run, files, task ids. The auditor checks it against the transcript.',
    },
  },
  required: ['evidence'],
}

function auditPrompt(condition: string, evidence: string, transcript: string | null): string {
  return [
    'Audit this /goal. The condition below comes verbatim from the goal mod, not from the main agent.',
    '=== CONDITION ===',
    condition,
    '=== END CONDITION ===',
    `The main agent's pointer to its evidence (unverified, it may be wrong or incomplete): ${evidence === '' ? 'none given' : evidence}`,
    transcript === null
      ? 'The session transcript path is unknown; rely on re-running the checks.'
      : [
          `The session transcript is the JSONL file ${transcript}; it can be megabytes, so never read or cat it whole: search it with rg -F for the exact command strings and look at the matching lines only.`,
          'It keeps every row, including rows from before context compactions (a "compact_boundary" row marks each). When a check appears several times, the latest occurrence counts; an older passing output does not prove the current state.',
          'Prefer re-running a read-only check over quoting the transcript; use the transcript for evidence you cannot re-create, such as a command that changes state.',
        ].join('\n'),
    `The item that asks for ${AUDIT_TOOL} returning VERDICT: PASS is this audit itself: do not judge it, judge every other item and every constraint.`,
  ].join('\n')
}

async function goalStatusOf(
  $: EngineInterface,
  path: string,
  uuid: string,
): Promise<{ met: boolean; failed: boolean; sentinel: boolean; reason: string } | null> {
  for (let attempt = 0; attempt < 6; attempt++) {
    const found = await $.process.run(['grep', '-F', '-m', '1', `"uuid":"${uuid}"`, path]).catch(() => null)
    const line = found?.stdout.trim() ?? ''
    if (line !== '') {
      try {
        const row = JSON.parse(line) as { attachment?: Record<string, unknown> }
        const status = row.attachment ?? {}
        return {
          met: status.met === true,
          failed: status.failed === true,
          sentinel: status.sentinel === true,
          reason: typeof status.reason === 'string' ? status.reason : '',
        }
      } catch {
        return null
      }
    }
    await $.clock.sleep(250)
  }
  return null
}

async function lastGoalStatus(
  $: EngineInterface,
  path: string,
): Promise<{ uuid: string; at: number; met: boolean; sentinel: boolean } | null> {
  const found = await $.process
    .run(['sh', '-c', 'tac "$1" | grep -F -m 1 "$2"', 'sh', path, '"type":"goal_status"'])
    .catch(() => null)
  try {
    const row = JSON.parse(found?.stdout.trim() ?? '') as {
      uuid?: unknown
      timestamp?: unknown
      attachment?: Record<string, unknown>
    }
    const status = row.attachment ?? {}
    if (status.type !== 'goal_status' || typeof row.uuid !== 'string') return null
    return {
      uuid: row.uuid,
      at: Date.parse(asString(row.timestamp)),
      met: status.met === true,
      sentinel: status.sentinel === true,
    }
  } catch {
    return null
  }
}

async function forcedClearCause($: EngineInterface, path: string, uuid: string): Promise<string | null> {
  for (let attempt = 0; attempt < 4; attempt++) {
    const found = await $.process.run(['grep', '-F', '-m', '1', `"parentUuid":"${uuid}"`, path]).catch(() => null)
    const line = found?.stdout.trim() ?? ''
    if (line !== '') {
      try {
        const row = JSON.parse(line) as { type?: unknown; subtype?: unknown; level?: unknown; content?: unknown }
        if (row.type !== 'system') return null
        const cause = FORCED_CLEAR.exec(asString(row.content))
        if (cause !== null) return cause[1]!
        return row.subtype === 'informational' && row.level === 'warning' ? 'cleared by Claude Code' : null
      } catch {
        return null
      }
    }
    await $.clock.sleep(250)
  }
  return null
}

const AUDITOR_PROMPT = [
  'You audit whether a /goal is really met. You get the goal condition and the main agent\'s evidence.',
  'Verify independently: re-run each item\'s verify command when it only reads (tests, builds that install nothing, git diff/status/log, linters), read the files involved, and check every constraint.',
  'Never edit, write, commit, push, deploy, switch, restart services or change any other state. When a check would change state, judge it from the evidence and say so.',
  'Reply with one line per acceptance item: its letter, PASS or FAIL, and the evidence you saw (the command and the relevant output).',
  'End with exactly one final line: VERDICT: PASS when every item and constraint holds, otherwise VERDICT: FAIL followed by the gaps to fix.',
].join('\n')

const PHASE_TITLE: Record<GoalPhase, string> = {
  idle: 'No goal',
  drafting: 'Drafting',
  ready: 'Draft ready',
  running: 'Goal running',
  interrupted: 'Goal interrupted',
  done: 'Goal closed',
}

type TitleColor = 'subtle' | 'suggestion' | 'success' | 'warning' | 'claude' | 'error'

const PHASE_COLOR: Record<GoalPhase, TitleColor> = {
  idle: 'subtle',
  drafting: 'warning',
  ready: 'suggestion',
  running: 'claude',
  interrupted: 'warning',
  done: 'success',
}

const RESULT_TITLE: Record<NonNullable<GoalRun['result']>, [string, TitleColor]> = {
  achieved: ['Goal achieved', 'success'],
  failed: ['Goal not achievable', 'error'],
  cleared: ['Goal cleared', 'warning'],
}

function title(phase: GoalPhase, run: GoalRun | null): [string, TitleColor] {
  if (phase === 'done' && run?.result) return RESULT_TITLE[run.result]
  return [PHASE_TITLE[phase], PHASE_COLOR[phase]]
}

function draftPrompt(idea: string): string {
  return [
    'Draft a /goal together with me before any work starts. Protocol:',
    '1. Investigate read-only first: read the relevant code, configs and logs. No edits, builds or other state changes in this phase.',
    '2. Ask what you cannot settle yourself with AskUserQuestion, up to 4 questions per call: the measurable end state, how to verify it, constraints that must not break, what is out of scope, anything else that changes the plan. Do not ask what the code or this conversation already answers.',
    `3. Call ${DRAFT_TOOL} with objective (one sentence, one measurable end state), acceptance (each item a concrete check plus the exact command and expected result that proves it), constraints and notes (your assumptions and open risks).`,
    '   Never bake a hypothesis, an expected cause or an expected outcome of a control into an acceptance item. When a check can go either way, phrase it as a fork that either outcome closes.',
    '4. Then stop and wait. The person edits, starts or cancels the goal in the goal pane. Do not start the work and do not set the goal yourself.',
    `Idea from the person: ${idea === '' ? 'none yet, ask what the goal is' : idea}`,
  ].join('\n')
}

function letter(index: number): string {
  return String.fromCharCode(97 + (index % 26))
}

const AUDIT_REPORT = 'Goal audit report'

const AUDITOR_CHECK = `${AUDIT_TOOL} was called once every other item was done, and the goal plugin then posted a "${AUDIT_REPORT}" message ending with VERDICT: PASS; the plugin runs the read-only ${AUDITOR} on this exact condition and the session transcript. After VERDICT: FAIL, the gaps it names are fixed and the tool is called again.`

function goalItems(draft: GoalDraft, settings: GoalSettings): GoalItem[] {
  return settings.auditor
    ? [...draft.acceptance, { check: AUDITOR_CHECK, verify: `the ${AUDIT_TOOL} result in the transcript` }]
    : draft.acceptance
}

function taskLine(ids: readonly string[] | null, count: number, audited: boolean): string {
  if (ids === null) return 'Each item is tracked as a task with TaskCreate and closed with TaskUpdate citing its evidence.'
  const placeholder = ids.length === 0 ? Array.from({ length: count }, () => '#000') : ids.map(id => `#${id}`)
  const audit = audited ? ` The last task is closed by ${AUDIT_TOOL} itself.` : ''
  return `Tasks ${placeholder.join(', ')} mirror items a-${letter(count - 1)} in order; each is set to completed with TaskUpdate only once its evidence is shown.${audit} Other tasks in the list are not part of this goal.`
}

function composeCondition(draft: GoalDraft, settings: GoalSettings, ids: readonly string[] | null): string {
  const items = goalItems(draft, settings)
  const lines = [
    draft.objective.trim(),
    'Met only when every item below is shown done in this conversation, each by its own command and output:',
    ...items.map((item, index) =>
      item.check === AUDITOR_CHECK
        ? `${letter(index)}. ${item.check}`
        : `${letter(index)}. ${item.check} Verify: ${item.verify}`,
    ),
    taskLine(ids, items.length, settings.auditor),
  ]
  if (draft.constraints.length > 0) {
    lines.push(`Constraints: ${draft.constraints.join('; ')}`)
  }
  return lines.join('\n')
}

async function createTasks($: EngineInterface, items: readonly GoalItem[]): Promise<string[] | null> {
  const ids: string[] = []
  for (const [index, item] of items.entries()) {
    const mark = letter(index)
    const subject = item.check === AUDITOR_CHECK ? `goal ${mark}: ${AUDIT_TOOL} returns VERDICT: PASS` : `goal ${mark}: ${item.check}`
    try {
      const ran = await $.tool.call({
        tool: 'TaskCreate',
        subject: subject.length > 120 ? `${subject.slice(0, 117)}...` : subject,
        description: item.check === AUDITOR_CHECK ? item.check : `${item.check}\nVerify: ${item.verify}`,
        activeForm: `Working on goal item ${mark}`,
      })
      const id = ran.deny === undefined && ran.isError !== true ? ran.result.task.id : undefined
      if (id === undefined) {
        await deleteTasks($, ids)
        return null
      }
      ids.push(id)
    } catch {
      await deleteTasks($, ids)
      return null
    }
  }
  return ids
}

async function deleteTasks($: EngineInterface, ids: readonly string[]): Promise<string[]> {
  const failed: string[] = []
  for (const taskId of ids) {
    try {
      const ran = await $.tool.call({ tool: 'TaskUpdate', taskId, status: 'deleted' })
      if (ran.deny !== undefined || ran.isError === true) failed.push(taskId)
    } catch {
      failed.push(taskId)
    }
  }
  return failed
}

async function deleteGoalTasks($: EngineInterface, ids: readonly string[], attempt = 1): Promise<void> {
  const failed = await deleteTasks($, ids)
  const deleted = ids.filter(id => !failed.includes(id))
  if (deleted.length > 0) await log($, `deleted open goal tasks ${deleted.map(id => `#${id}`).join(' ')}`)
  if (failed.length === 0) return
  if (attempt >= 3) {
    await log($, `could not delete goal tasks ${failed.map(id => `#${id}`).join(' ')}`)
    return
  }
  $.clock.after(2000 * attempt, () => void deleteGoalTasks($, failed, attempt + 1))
}

function cycle(options: readonly number[], current: number): number {
  const index = options.indexOf(current)
  return options[(index + 1) % options.length] ?? options[0] ?? 0
}

function formatTokens(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${Math.round(value / 1_000)}k`
  return String(value)
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function parseDraft(input: Record<string, unknown>): GoalDraft | string {
  const objective = asString(input.objective)
  if (objective === '') return 'objective is empty'
  const raw = Array.isArray(input.acceptance) ? input.acceptance : []
  const acceptance: GoalItem[] = []
  for (const entry of raw) {
    const record = (entry ?? {}) as Record<string, unknown>
    const check = asString(record.check)
    const verify = asString(record.verify)
    if (check === '' || verify === '') return 'every acceptance item needs both check and verify'
    acceptance.push({ check, verify })
  }
  if (acceptance.length === 0) return 'acceptance is empty'
  const constraints = (Array.isArray(input.constraints) ? input.constraints : [])
    .map(asString)
    .filter(text => text !== '')
  return { objective, acceptance, constraints, notes: asString(input.notes) }
}

async function log($: EngineInterface, text: string): Promise<void> {
  const now = new Date(await $.clock.now())
  const stamp = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
  await update($, logAtom, list => [...list, `${stamp} ${text}`].slice(-20))
}

async function refreshStatus($: EngineInterface): Promise<void> {
  const phase = await read($, phaseAtom)
  const run = await read($, runAtom)
  const settings = await read($, settingsAtom)
  if (phase === 'running' && run !== null) {
    const turns = settings.maxTurns > 0 ? `${run.turns}/${settings.maxTurns}` : String(run.turns)
    const tokens =
      settings.maxTokens > 0
        ? `${formatTokens(run.tokens)}/${formatTokens(settings.maxTokens)}`
        : formatTokens(run.tokens)
    const audit = run.auditor === null ? '' : ` · audit ${run.auditor.toUpperCase()}`
    const tasks = run.tasks.length === 0 ? '' : ` · tasks ${run.done.length}/${run.tasks.length}`
    $.ui.status(`turn ${turns} · ${tokens} tok${tasks}${audit}`)
    return
  }
  if (phase === 'interrupted' && run !== null) {
    $.ui.status(`${run.outcome ?? 'interrupted'} · resumes after your next message`)
    return
  }
  if (phase === 'drafting') {
    $.ui.status('drafting')
    return
  }
  if (phase === 'ready') {
    $.ui.status('draft ready: ctrl+x tab, then s to start')
    return
  }
  if (phase === 'done' && run !== null && run.result !== null) {
    $.ui.status(`${run.result} after ${run.turns} turns, ${formatTokens(run.tokens)} tok`)
    return
  }
  $.ui.status(undefined)
}

async function openPane($: EngineInterface, focus = false): Promise<void> {
  await $.ui.open(focus ? { id: PANE, title: 'Goal', columns: 64, focus: true } : { id: PANE, title: 'Goal', columns: 64 })
}

function submitLater($: EngineInterface, text: string): void {
  $.clock.after(50, () => {
    $.prompt.submit({ text }).catch(error => {
      $.ui.toast(`goal: prompt not sent: ${String(error).slice(0, 80)}`)
    })
  })
}

async function startDraft($: EngineInterface, idea: string): Promise<void> {
  await update($, phaseAtom, () => 'drafting')
  await update($, draftAtom, () => null)
  await update($, runAtom, () => null)
  await log($, idea === '' ? 'drafting started' : `drafting: ${idea.slice(0, 60)}`)
  await refreshStatus($)
  void openPane($)
  submitLater($, draftPrompt(idea))
}

async function beginRun(
  $: EngineInterface,
  condition: string,
  tasks: readonly string[] = [],
  auditTask: string | null = null,
): Promise<void> {
  const startedAt = await $.clock.now()
  await update($, runAtom, () => ({
    condition,
    startedAt,
    turns: 0,
    tokens: 0,
    auditor: null,
    outcome: null,
    result: null,
    verdict: null,
    tasks: [...tasks],
    done: [],
    auditTask,
  }))
  await update($, phaseAtom, () => 'running')
  await log($, `goal started, ${condition.length} chars`)
  await refreshStatus($)
}

async function finish($: EngineInterface, outcome: string, result: NonNullable<GoalRun['result']>): Promise<void> {
  const run = await read($, runAtom)
  await update($, runAtom, (value): GoalRun | null => (value === null ? value : { ...value, outcome, result }))
  await update($, phaseAtom, () => 'done')
  await log($, `goal ${result}: ${outcome.slice(0, 80)}`)
  if (result === 'cleared' && run !== null) {
    const open = run.tasks.filter(id => !run.done.includes(id))
    if (open.length > 0) await deleteGoalTasks($, open)
  }
  await refreshStatus($)
}

let runningTurn: string | null = null
let spawningAudit = false
let clearing = false
let goalCommands = 0
let resuming = false
let resumeArmed = false
let handledClear: string | null = null

async function interrupt($: EngineInterface, cause: string): Promise<boolean> {
  if ((await read($, phaseAtom)) !== 'running') return false
  await update($, phaseAtom, () => 'interrupted')
  await update($, runAtom, (run): GoalRun | null => (run === null ? run : { ...run, outcome: `interrupted: ${cause}` }))
  await log($, `goal interrupted: ${cause}`)
  await refreshStatus($)
  return true
}

async function checkForcedClear($: EngineInterface, path: string, retries: number): Promise<void> {
  const run = await read($, runAtom)
  if (run === null || (await read($, phaseAtom)) !== 'running') return
  const last = await lastGoalStatus($, path)
  const dropped =
    last !== null && last.sentinel && last.met && last.uuid !== handledClear && last.at >= run.startedAt ? last : null
  const cause = dropped === null ? null : await forcedClearCause($, path, dropped.uuid)
  if (dropped === null || cause === null) {
    if (retries > 0) $.clock.after(500, () => void checkForcedClear($, path, retries - 1))
    return
  }
  if (clearing || goalCommands > 0) return
  handledClear = dropped.uuid
  if (await interrupt($, cause)) {
    $.ui.toast(`goal: Claude Code dropped the goal (${cause}); it resumes after your next message`)
  }
}

async function continueRun($: EngineInterface): Promise<void> {
  await update($, runAtom, (run): GoalRun | null => (run === null ? run : { ...run, outcome: null, verdict: null }))
  await update($, phaseAtom, () => 'running')
  await log($, 'goal resumed')
  await refreshStatus($)
}

async function settleResume($: EngineInterface, resumed: GoalRun): Promise<void> {
  const run = await read($, runAtom)
  const phase = await read($, phaseAtom)
  if (run !== null && run.startedAt === resumed.startedAt && (phase === 'running' || phase === 'interrupted')) return
  await log($, 'resume overtaken by a /goal command, goal set to match the pane')
  const turnId = runningTurn
  if (turnId !== null) await $.turn.abort({ turnId }).catch(() => undefined)
  await $.command.run({ command: 'goal', args: phase === 'running' && run !== null ? run.condition : 'clear' })
}

async function resumeGoal($: EngineInterface): Promise<void> {
  if (resuming || goalCommands > 0) return
  resuming = true
  try {
    const run = await read($, runAtom)
    if (run === null || (await read($, phaseAtom)) !== 'interrupted') return
    if (runningTurn !== null) {
      resumeArmed = true
      $.ui.toast('goal: resumes when this turn ends')
      return
    }
    await continueRun($)
    await $.command.run({ command: 'goal', args: run.condition })
    await settleResume($, run)
  } catch (error) {
    await log($, `resume failed: ${String(error).slice(0, 80)}`)
    await interrupt($, '/goal did not start again')
    $.ui.toast('goal: /goal did not resume, see the pane log')
  } finally {
    resuming = false
  }
}

async function dropGoal($: EngineInterface): Promise<void> {
  if ((await read($, phaseAtom)) !== 'interrupted') return
  await finish($, 'dropped after the interruption', 'cleared')
}

async function clearGoal($: EngineInterface, reason: string): Promise<void> {
  if (clearing) return
  clearing = true
  await log($, `clearing: ${reason}`)
  $.clock.after(50, () => {
    void (async () => {
      const turnId = runningTurn
      if (turnId !== null) await $.turn.abort({ turnId }).catch(() => undefined)
      try {
        const answer = await $.command.run({ command: 'goal', args: 'clear' })
        const text = answer.text ?? ''
        if (text.trim() === '' || /Goal cleared|No goal set/i.test(text)) {
          if ((await read($, phaseAtom)) === 'running') await finish($, reason, 'cleared')
        } else {
          await log($, `clear not confirmed: ${text.slice(0, 80)}`)
          $.ui.toast('goal: /goal clear did not confirm, the goal may still be active; run /goal clear')
        }
      } catch {
        await log($, 'clear failed')
        $.ui.toast(`goal: could not clear the goal (${reason}), run /goal clear`)
      } finally {
        clearing = false
      }
    })()
  })
}

async function saveSettings($: EngineInterface, change: (value: GoalSettings) => GoalSettings): Promise<void> {
  await update($, settingsAtom, change)
  await $.store.set('settings', await read($, settingsAtom))
  await refreshStatus($)
}

async function loadSettings($: EngineInterface): Promise<void> {
  const saved = (await $.store.get('settings')) as Partial<GoalSettings> | undefined
  if (saved === undefined || saved === null || typeof saved !== 'object') return
  await update($, settingsAtom, () => ({
    auditor: typeof saved.auditor === 'boolean' ? saved.auditor : DEFAULTS.auditor,
    maxTurns: typeof saved.maxTurns === 'number' ? saved.maxTurns : DEFAULTS.maxTurns,
    maxTokens: typeof saved.maxTokens === 'number' ? saved.maxTokens : DEFAULTS.maxTokens,
  }))
}

async function startGoal($: EngineInterface): Promise<void> {
  const draft = await read($, draftAtom)
  if (draft === null) return
  const settings = await read($, settingsAtom)
  const longest = composeCondition(draft, settings, [])
  if (longest.length > GOAL_LIMIT) {
    $.ui.toast(`goal: condition is ${longest.length} chars, /goal takes ${GOAL_LIMIT}; ask the agent to shorten it`)
    return
  }
  const tasks = await createTasks($, goalItems(draft, settings))
  if (tasks === null) await log($, 'task list not filled, the agent tracks the items itself')
  const condition = composeCondition(draft, settings, tasks)
  const auditTask = settings.auditor && tasks !== null ? (tasks.at(-1) ?? null) : null
  await beginRun($, condition, tasks ?? [], auditTask)
  try {
    await $.command.run({ command: 'goal', args: condition })
  } catch (error) {
    await deleteTasks($, tasks ?? [])
    await update($, phaseAtom, () => 'ready')
    await update($, runAtom, () => null)
    await log($, `start failed: ${String(error).slice(0, 80)}`)
    $.ui.toast('goal: /goal did not start, see the pane log')
    await refreshStatus($)
  }
}

async function revise($: EngineInterface, feedback: string): Promise<void> {
  const text = feedback.trim()
  if (text === '') return
  await update($, phaseAtom, () => 'drafting')
  await log($, `revision: ${text.slice(0, 60)}`)
  await refreshStatus($)
  submitLater(
    $,
    `Revise the goal draft with this feedback, then call ${DRAFT_TOOL} again with the whole updated draft and wait: ${text}`,
  )
}

async function cancelDraft($: EngineInterface): Promise<void> {
  await update($, phaseAtom, () => 'idle')
  await update($, draftAtom, () => null)
  await log($, 'draft cancelled')
  await refreshStatus($)
  await $.ui.close({ id: PANE })
}

async function dismiss($: EngineInterface): Promise<void> {
  await update($, phaseAtom, () => 'idle')
  await update($, runAtom, () => null)
  await update($, draftAtom, () => null)
  await refreshStatus($)
  await $.ui.close({ id: PANE })
}

let closingAudit = false

async function settleAudit($: EngineInterface, answer: string, runStartedAt: number): Promise<void> {
  const run = await read($, runAtom)
  const phase = await read($, phaseAtom)
  if (run === null || run.startedAt !== runStartedAt || (phase !== 'running' && phase !== 'interrupted')) {
    await log($, 'audit report dropped: the goal changed meanwhile')
    return
  }
  const verdict = /VERDICT:\s*(PASS|FAIL)/i.exec(answer)
  const value: GoalRun['auditor'] = verdict !== null && verdict[1]!.toUpperCase() === 'PASS' ? 'pass' : 'fail'
  await update($, runAtom, (current): GoalRun | null => (current === null ? current : { ...current, auditor: value }))
  await log($, `audit: ${value.toUpperCase()}`)
  if (value === 'pass' && run.auditTask !== null) {
    closingAudit = true
    try {
      await $.tool.call({ tool: 'TaskUpdate', taskId: run.auditTask, status: 'completed' })
      const auditTask = run.auditTask
      await update($, runAtom, (current): GoalRun | null =>
        current === null || current.done.includes(auditTask) ? current : { ...current, done: [...current.done, auditTask] },
      )
    } catch {
      await log($, `audit task #${run.auditTask} not closed`)
    } finally {
      closingAudit = false
    }
  }
  await refreshStatus($)
  const body = verdict === null ? `${answer}\n\nVERDICT: FAIL (the auditor gave no verdict line)` : answer
  const report = `${AUDIT_REPORT} from ${AUDIT_TOOL}, delivered by the goal mod:\n\n${body}`
  try {
    await $.session.append({ message: { type: 'user', content: [{ type: 'text', text: report }] } })
  } catch {
    await log($, 'report append failed, queued as a prompt')
    submitLater($, report)
  }
}

export const register: Register = on => {
  let focusWhenIdle = false
  let pendingAudit: { agentId: string; runStartedAt: number } | null = null
  let transcriptPath: string | null = null

  const rememberTranscript = (path: string): void => {
    if (path !== '') transcriptPath = path
  }

  on('classic.SessionStart', (_, e, next) => {
    rememberTranscript(e.transcript_path)
    return next(e)
  })

  on('session.end', async ($, e, next) => {
    if (e.reason === 'clear') {
      await update($, phaseAtom, () => 'idle')
      await update($, draftAtom, () => null)
      await update($, runAtom, () => null)
      $.ui.status(undefined)
    }
    return next(e)
  })

  on('classic.UserPromptSubmit', (_, e, next) => {
    rememberTranscript(e.transcript_path)
    return next(e)
  }).catch((_, e, next) => next(e))

  on('session.start', async ($, e, next) => {
    const result = await next(e)
    await $.command.register({
      name: 'goal-draft',
      description: 'Draft a /goal with the agent in the goal pane',
      argumentHint: '[idea]',
    })
    await $.tool.register({
      name: 'draft',
      description: DRAFT_TOOL_DESCRIPTION,
      inputSchema: DRAFT_SCHEMA,
      isDeferred: false,
    })
    await $.tool.register({
      name: 'audit',
      description: AUDIT_TOOL_DESCRIPTION,
      inputSchema: AUDIT_SCHEMA,
      isDeferred: false,
    })
    await $.agent.register({
      name: 'auditor',
      description: `Read-only auditor the goal mod runs through ${AUDIT_TOOL}; not for direct use.`,
      prompt: AUDITOR_PROMPT,
      tools: ['Read', 'Bash'],
      disallowedTools: ['Edit', 'Write', 'NotebookEdit'],
    })
    await loadSettings($).catch(() => undefined)
    await refreshStatus($)
    return result
  })

  on('command.run', { command: 'goal-draft' }, async ($, e) => {
    await dropGoal($)
    await startDraft($, e.args.trim())
    return { text: 'Drafting a goal: the agent investigates and asks, the draft shows up in the goal pane.' }
  })

  on('command.run', { command: 'goal' }, async ($, e, next) => {
    goalCommands++
    try {
      const origin = e.origin as typeof e.origin | undefined
      if (origin?.kind === 'plugin' && origin.name === 'goal') return await next(e)
      const args = e.args.trim()
      const phase = await read($, phaseAtom)
      if (args === '') {
        if (phase === 'running') {
          void openPane($)
          return await next(e)
        }
        if (phase === 'interrupted') {
          void openPane($, true)
          return { text: 'Claude Code dropped the goal after an error. It is set again after your next message; the goal pane resumes or drops it.' }
        }
        if (phase === 'drafting' || phase === 'ready') {
          void openPane($, true)
          return { text: 'The goal draft is open in the goal pane.' }
        }
        await startDraft($, '')
        return { text: 'Drafting a goal: the agent investigates and asks, the draft shows up in the goal pane.' }
      }
      resumeArmed = false
      if (CLEAR.has(args.toLowerCase())) {
        const result = await next(e)
        if (phase === 'running' || phase === 'interrupted') await finish($, 'cleared by hand', 'cleared')
        return result
      }
      if (phase === 'interrupted' && (await read($, runAtom))?.condition === args) {
        await continueRun($)
        return await next(e)
      }
      const result = await next(e)
      await dropGoal($)
      await update($, draftAtom, () => null)
      await beginRun($, args)
      return result
    } finally {
      goalCommands--
    }
  }).catch((_, e, next) => next(e))

  on('prompt.submit', async ($, e, next) => {
    const result = await next(e)
    const typed = e.origin.kind === 'composer' || e.origin.kind === 'bridge'
    if (typed && e.command === undefined && e.isMeta !== true && result.drop === undefined && (await read($, phaseAtom)) === 'interrupted') {
      resumeArmed = true
    }
    return result
  }).catch((_, e, next) => next(e))

  on('tool.call', { tool: DRAFT_TOOL }, async ($, e) => {
    const parsed = parseDraft(e as unknown as Record<string, unknown>)
    if (typeof parsed === 'string') {
      const refusal = `Draft refused: ${parsed}. Fix it and call ${DRAFT_TOOL} again.`
      return { isError: true, result: refusal, text: refusal }
    }
    await update($, draftAtom, () => parsed)
    await update($, phaseAtom, () => 'ready')
    await log($, `draft: ${parsed.acceptance.length} items`)
    await refreshStatus($)
    void openPane($)
    focusWhenIdle = true
    const shown =
      'The draft is in the goal pane. Stop now and wait: the person starts it, edits it or asks for changes there. Do not set the goal or start the work yourself.'
    return { result: shown, text: shown }
  }).catch(() => {
    const failed = 'The goal pane failed to take the draft. Show the draft to the person as text instead.'
    return { isError: true, result: failed, text: failed }
  })

  on('classic.Stop', (_, e, next) => (clearing ? {} : next(e))).catch((_, e, next) => next(e))

  on('turn.start', (_, e, next) => {
    if (e.agentId === undefined) runningTurn = e.turnId
    return next(e)
  })

  on('tool.call', { tool: 'Agent' }, (_, e, next) =>
    !spawningAudit && e.subagent_type === AUDITOR
      ? { deny: `${AUDITOR} runs only through ${AUDIT_TOOL}, which hands it the exact goal condition; call that tool instead.` }
      : next(e),
  ).catch((_, e, next) => next(e))

  on('tool.call', { tool: AUDIT_TOOL }, async ($, e) => {
    const run = await read($, runAtom)
    if (run === null || (await read($, phaseAtom)) !== 'running') {
      const none = 'No goal is running in the goal mod, so there is nothing to audit.'
      return { isError: true, result: none, text: none }
    }
    if (pendingAudit !== null) {
      const busy = `An audit is already running (agent ${pendingAudit.agentId}). End your turn; its report arrives as a "${AUDIT_REPORT}" message from the goal plugin.`
      return { result: busy, text: busy }
    }
    const evidence = asString((e as unknown as Record<string, unknown>).evidence)
    spawningAudit = true
    let spawned: Awaited<ReturnType<typeof $.agent.spawn>>
    try {
      spawned = await $.agent.spawn({
        prompt: auditPrompt(run.condition, evidence, transcriptPath),
        description: 'goal audit',
        subagentType: AUDITOR,
      })
    } finally {
      spawningAudit = false
    }
    if (spawned.deny !== undefined || spawned.agentId === undefined) {
      const refused = `The auditor did not start: ${spawned.deny ?? 'no agent id'}.`
      await log($, 'audit did not start')
      return { isError: true, result: refused, text: refused }
    }
    pendingAudit = { agentId: spawned.agentId, runStartedAt: run.startedAt }
    await log($, 'audit started')
    const started = `The read-only auditor is running on the exact goal condition (agent ${spawned.agentId}). End your turn now and do not call ${AUDIT_TOOL} again: its report arrives as a "${AUDIT_REPORT}" message from the goal plugin.`
    return { result: started, text: started }
  }).catch(() => {
    const failed = 'The goal audit failed to start; call the tool again.'
    return { isError: true, result: failed, text: failed }
  })

  on('tool.call', { tool: 'TaskUpdate' }, async ($, e, next) => {
    const guarded = await read($, runAtom)
    const guardedPhase = await read($, phaseAtom)
    if (
      !closingAudit &&
      guarded !== null &&
      guarded.auditTask === e.taskId &&
      e.status === 'completed' &&
      (guardedPhase === 'running' || guardedPhase === 'interrupted')
    ) {
      return { deny: `Task #${e.taskId} is closed by ${AUDIT_TOOL} after a VERDICT: PASS; call that tool instead.` }
    }
    const ran = await next(e)
    if (ran.deny !== undefined || ran.isError === true) return ran
    const run = await read($, runAtom)
    if (run === null || !run.tasks.includes(e.taskId) || e.status === undefined) return ran
    const status = e.status
    await update($, runAtom, (value): GoalRun | null => {
      if (value === null) return value
      const others = value.done.filter(id => id !== e.taskId)
      return { ...value, done: status === 'completed' ? [...others, e.taskId] : others }
    })
    await refreshStatus($)
    return ran
  }).catch((_, e, next) => next(e))

  on('turn.complete', async ($, e, next) => {
    const result = await next(e)
    if (e.agentId !== undefined) {
      if (pendingAudit !== null && pendingAudit.agentId === e.agentId) {
        const { runStartedAt } = pendingAudit
        pendingAudit = null
        const answer = e.answer
        $.clock.after(50, () => void settleAudit($, answer, runStartedAt))
      }
      return result
    }
    if (runningTurn === e.turnId) runningTurn = null
    if (resumeArmed) {
      resumeArmed = false
      if (e.reason === 'answer') $.clock.after(50, () => void resumeGoal($))
    }
    if (focusWhenIdle && (await read($, phaseAtom)) === 'ready') {
      focusWhenIdle = false
      $.clock.after(50, () => void openPane($, true))
    }
    if ((await read($, phaseAtom)) !== 'running') return result
    const path = transcriptPath
    if (path !== null) $.clock.after(300, () => void checkForcedClear($, path, e.reason === 'error' ? 3 : 0))
    const spent = e.usage?.output_tokens ?? 0
    await update($, runAtom, run => (run === null ? run : { ...run, turns: run.turns + 1, tokens: run.tokens + spent }))
    const run = await read($, runAtom)
    const settings = await read($, settingsAtom)
    if (run !== null && settings.maxTurns > 0 && run.turns >= settings.maxTurns) {
      $.ui.toast(`goal: turn limit ${settings.maxTurns} reached, goal cleared`)
      await clearGoal($, `turn limit ${settings.maxTurns}`)
    } else if (run !== null && settings.maxTokens > 0 && run.tokens >= settings.maxTokens) {
      $.ui.toast(`goal: token limit ${formatTokens(settings.maxTokens)} reached, goal cleared`)
      await clearGoal($, `token limit ${formatTokens(settings.maxTokens)}`)
    } else {
      await refreshStatus($)
    }
    return result
  })

  on('session.append', { door: 'attachment' }, async ($, e, next) => {
    const result = await next(e)
    if (e.message.name !== 'goal_status') return result
    const path = transcriptPath
    if (path === null || (await read($, phaseAtom)) !== 'running') return result
    const uuid = e.uuid
    $.clock.after(100, () => {
      void (async () => {
        const status = await goalStatusOf($, path, uuid)
        if (status === null || status.sentinel || (await read($, phaseAtom)) !== 'running') return
        if (status.met) {
          await finish($, status.reason === '' ? 'condition met' : status.reason.slice(0, 300), 'achieved')
        } else if (status.failed) {
          await finish($, status.reason.slice(0, 300), 'failed')
        } else {
          await update($, runAtom, (run): GoalRun | null => (run === null ? run : { ...run, verdict: status.reason }))
          await log($, 'evaluator: not met yet')
        }
      })()
    })
    return result
  }).catch((_, e, next) => next(e))

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Button, Input } = $.ui.resolve(e)
    const phase = await read($, phaseAtom)
    const draft = await read($, draftAtom)
    const settings = await read($, settingsAtom)
    const run = await read($, runAtom)
    const history = await read($, logAtom)
    const editable = phase === 'drafting' || phase === 'ready'
    const tracked = phase === 'running' || phase === 'interrupted' || phase === 'done'
    const wide = e.props.bodyColumns >= 56

    const header = (title: string) => (
      <Text dimColor weight="bold">
        {title.toUpperCase()}
      </Text>
    )

    const item = (mark: string, check: string, verify: string | null, color?: 'success' | 'error') => (
      <Box flexDirection="column">
        <Text>
          <Text color={color ?? 'suggestion'} weight="bold">
            {mark}
          </Text>{' '}
          {check}
        </Text>
        {verify !== null && (
          <Box paddingLeft={3}>
            <Text dimColor>{verify}</Text>
          </Box>
        )}
      </Box>
    )

    return (
      <Box flexDirection="column" rowGap={1}>
        <Box justifyContent="space-between">
          <Text color={title(phase, run)[1]} weight="bold">
            {title(phase, run)[0]}
          </Text>
          <Text dimColor>{e.props.isFocused ? 'esc: back to prompt' : 'ctrl+x tab: focus'}</Text>
        </Box>

        {phase === 'idle' && <Text dimColor>/goal with no arguments, or /goal-draft [idea], starts a draft.</Text>}
        {phase === 'drafting' && draft === null && (
          <Text dimColor>The agent is investigating and asking questions; the draft lands here.</Text>
        )}

        {editable && draft !== null && (
          <Box flexDirection="column">
            {header('Objective')}
            <Text>{draft.objective}</Text>
          </Box>
        )}
        {editable && draft !== null && (
          <Box flexDirection="column" rowGap={1}>
            {header('Acceptance')}
            {draft.acceptance.map((entry, index) => item(`${letter(index)}.`, entry.check, entry.verify))}
            {settings.auditor && item(`${letter(draft.acceptance.length)}.`, `${AUDIT_TOOL} returns VERDICT: PASS`, null)}
          </Box>
        )}
        {editable && draft !== null && draft.constraints.length > 0 && (
          <Box flexDirection="column">
            {header('Constraints')}
            {draft.constraints.map(text => (
              <Text>- {text}</Text>
            ))}
          </Box>
        )}
        {editable && draft !== null && draft.notes !== '' && (
          <Box flexDirection="column">
            {header('Notes')}
            <Text dimColor>{draft.notes}</Text>
          </Box>
        )}

        {tracked && run !== null && (
          <Box flexDirection="column">
            {header('Progress')}
            <Text>
              turn {run.turns}
              {settings.maxTurns > 0 ? `/${settings.maxTurns}` : ''} · {formatTokens(run.tokens)}
              {settings.maxTokens > 0 ? `/${formatTokens(settings.maxTokens)}` : ''} output tokens
            </Text>
            {run.tasks.length > 0 && (
              <Text>
                tasks: {run.done.length}/{run.tasks.length} done ({run.tasks.map(id => `#${id}`).join(' ')})
              </Text>
            )}
            <Text>
              auditor:{' '}
              <Text color={run.auditor === 'pass' ? 'success' : run.auditor === 'fail' ? 'error' : 'subtle'}>
                {run.auditor === null ? (settings.auditor ? 'not run yet' : 'off') : run.auditor.toUpperCase()}
              </Text>
            </Text>
            {run.outcome === null && run.verdict !== null && <Text dimColor>evaluator: {run.verdict}</Text>}
            {run.outcome !== null && <Text>outcome: {run.outcome}</Text>}
          </Box>
        )}
        {tracked && run !== null && (
          <Box flexDirection="column">
            {header('Condition')}
            <Text dimColor>{run.condition}</Text>
          </Box>
        )}

        <Box flexDirection="column">
          {header('Settings')}
          <Box flexDirection={wide ? 'row' : 'column'} columnGap={3}>
            <Button
              key="auditor"
              hotkey="a"
              plain
              label={`auditor: ${settings.auditor ? 'on' : 'off'}`}
              onPress={() => void saveSettings($, value => ({ ...value, auditor: !value.auditor }))}
            />
            <Button
              key="turns"
              hotkey="t"
              plain
              label={`turns: ${settings.maxTurns === 0 ? 'no limit' : settings.maxTurns}`}
              onPress={() => void saveSettings($, value => ({ ...value, maxTurns: cycle(TURN_OPTIONS, value.maxTurns) }))}
            />
            <Button
              key="tokens"
              hotkey="k"
              plain
              label={`tokens: ${settings.maxTokens === 0 ? 'no limit' : formatTokens(settings.maxTokens)}`}
              onPress={() => void saveSettings($, value => ({ ...value, maxTokens: cycle(TOKEN_OPTIONS, value.maxTokens) }))}
            />
          </Box>
        </Box>

        {phase === 'ready' && (
          <Box flexDirection="column" rowGap={1}>
            {header('Actions')}
            <Box columnGap={3}>
              <Button key="start" hotkey="s" plain label="start goal" onPress={() => void startGoal($)} />
              <Button key="cancel" hotkey="x" plain label="cancel draft" onPress={() => void cancelDraft($)} />
            </Box>
            <Input
              key="edit"
              label="revise"
              placeholder="tab here, type, enter"
              submitLabel="send"
              onSubmit={value => void revise($, value)}
            />
          </Box>
        )}
        {phase === 'drafting' && (
          <Box flexDirection="column">
            {header('Actions')}
            <Button key="cancel" hotkey="x" plain label="cancel draft" onPress={() => void cancelDraft($)} />
          </Box>
        )}
        {phase === 'running' && (
          <Box flexDirection="column">
            {header('Actions')}
            <Button key="stop" hotkey="x" plain label="clear goal" onPress={() => void clearGoal($, 'cleared from the pane')} />
          </Box>
        )}
        {phase === 'interrupted' && (
          <Box flexDirection="column">
            {header('Actions')}
            <Text dimColor>Claude Code dropped the goal after an error; it is set again after your next message.</Text>
            <Box columnGap={3}>
              <Button key="resume" hotkey="r" plain label="resume goal" onPress={() => void resumeGoal($)} />
              <Button key="drop" hotkey="x" plain label="drop goal" onPress={() => void dropGoal($)} />
            </Box>
          </Box>
        )}
        {phase === 'done' && (
          <Box flexDirection="column">
            {header('Actions')}
            <Button key="close" hotkey="x" plain label="close" onPress={() => void dismiss($)} />
          </Box>
        )}

        {history.length > 0 && (
          <Box flexDirection="column">
            {header('Log')}
            {history.slice(-3).map(line => (
              <Text dimColor>{line}</Text>
            ))}
          </Box>
        )}
      </Box>
    )
  })
}
