import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

const TYPED = { origin: { kind: 'composer' }, presentation: { isFullscreen: true, columns: 200 } } as const

const PANE = {
  plugin: 'goal',
  component: 'Pane',
  requestId: 'goal',
  props: {
    title: 'Goal',
    isFocused: true,
    bodyColumns: 60,
    placement: 'dock',
    scroll: { offset: 0, bodyRows: 40 },
    view: {},
  },
} as const

const DRAFT = {
  objective: 'nix build of the goal package exits 0',
  acceptance: [{ check: 'the package builds', verify: 'nix build .#goal exits 0' }],
  constraints: ['no switch'],
  notes: 'assumes the flake input is current',
}

const COMPOSED = { wait: false, origin: { kind: 'composer' } } as const
const TRANSCRIPT = '/tmp/session.jsonl'
const SESSION = 'session-1'
const STARTED = Date.parse('2026-11-12T10:00:00Z')
const FORCED = 'Goal cleared after an unrecoverable error (authentication failed): "tests pass". Run /goal again to continue.'

type Row = Record<string, unknown> & { uuid: string }

function sentinel(uuid: string, met: boolean, at = STARTED + 60_000): Row {
  return {
    type: 'attachment',
    uuid,
    timestamp: new Date(at).toISOString(),
    attachment: { type: 'goal_status', met, sentinel: true, condition: 'tests pass' },
  }
}

function notice(parentUuid: string, content = FORCED): Row {
  return { type: 'system', subtype: 'informational', level: 'warning', uuid: `after-${parentUuid}`, parentUuid, content }
}

type Seen = {
  prompts: string[]
  goals: string[]
  toasts: string[]
  spawned: string[]
  tasks: { id: string; subject: string; status: string }[]
  rows: Row[]
  store: Map<string, unknown>
  clock: ReturnType<typeof mock.clock>
  clearEntered: Promise<void>
  releaseClear: () => void
  releaseGoal: () => void
  signal: (fragment: string) => Promise<void>
}

function engine(
  on: On,
  options: {
    taskCreateFails?: boolean
    clearUnconfirmed?: boolean
    holdClear?: boolean
    holdGoalAt?: number
    store?: Record<string, unknown>
  } = {},
): Seen {
  let enterClear = (): void => undefined
  let releaseClear = (): void => undefined
  const clearEntered = new Promise<void>(resolve => {
    enterClear = resolve
  })
  const clearHeld = new Promise<void>(resolve => {
    releaseClear = resolve
  })
  let releaseGoal = (): void => undefined
  const goalHeld = new Promise<void>(resolve => {
    releaseGoal = resolve
  })
  const signals: string[] = []
  const awaited: { fragment: string; resolve: () => void }[] = []
  const emit = (signal: string): void => {
    signals.push(signal)
    for (const entry of awaited) if (signal.includes(entry.fragment)) entry.resolve()
  }
  const seen: Seen = {
    prompts: [],
    goals: [],
    toasts: [],
    spawned: [],
    tasks: [],
    rows: [],
    store: new Map(),
    clock: mock.clock(on, { now: STARTED }),
    clearEntered,
    releaseClear: () => releaseClear(),
    releaseGoal: () => releaseGoal(),
    signal: fragment =>
      new Promise<void>(resolve => {
        if (signals.some(signal => signal.includes(fragment))) resolve()
        else awaited.push({ fragment, resolve })
      }),
  }
  const store = seen.store
  for (const [key, value] of Object.entries(options.store ?? {})) store.set(key, value)
  on('store.get', (_, e) => ({ value: store.get(e.key) }))
  on('store.set', (_, e) => {
    store.set(e.key, e.value)
    return { value: undefined }
  })
  on('store.delete', (_, e) => {
    store.delete(e.key)
    return { value: undefined }
  })
  on('store.keys', () => ({ value: [...store.keys()] }))
  on('session.id', () => ({ value: SESSION }))
  on('process.run', (_, e) => {
    const pattern = e.argv[0] === 'sh' ? 'last goal_status' : (e.argv[4] ?? '')
    const row =
      e.argv[0] === 'sh'
        ? seen.rows.findLast(entry => (entry.attachment as { type?: string } | undefined)?.type === 'goal_status')
        : seen.rows.find(entry => pattern === `"uuid":"${entry.uuid}"` || pattern === `"parentUuid":"${String(entry.parentUuid)}"`)
    emit(`run ${pattern}`)
    return {
      value: {
        exitCode: row === undefined ? 1 : 0,
        stdout: row === undefined ? '' : `${JSON.stringify(row)}\n`,
        stderr: '',
        isStdoutTruncated: false,
        isStderrTruncated: false,
      },
    }
  })
  on('agent.spawn', (_, e) => {
    seen.spawned.push(e.prompt)
    return { model: 'test', agentId: 'auditor-1' }
  })
  on('tool.call', { tool: 'Agent' }, () => ({ result: { agentId: 'auditor-1' }, text: 'agent ran' }))
  on('tool.call', { tool: 'TaskCreate' }, (_, e) => {
    if (options.taskCreateFails) return { isError: true, result: 'no task list', text: 'no task list' }
    const id = String(seen.tasks.length + 7)
    seen.tasks.push({ id, subject: e.subject, status: 'pending' })
    return { result: { task: { id, subject: e.subject } }, text: `Task #${id} created` }
  })
  on('tool.call', { tool: 'TaskUpdate' }, (_, e) => {
    const task = seen.tasks.find(entry => entry.id === e.taskId)
    if (task !== undefined && e.status !== undefined) task.status = e.status
    return { result: { success: true, taskId: e.taskId, updatedFields: ['status'] }, text: 'updated' }
  })
  on('command.register', (_, e) => ({ value: { command: e.name } }))
  on('tool.register', (_, e) => ({ value: { tool: `mcp__goal__${e.name}` } }))
  on('agent.register', (_, e) => ({ value: { agent: `goal:${e.name}` } }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.close', () => ({ value: undefined }))
  on('ui.status', () => ({ value: undefined }))
  on('ui.toast', (_, e) => {
    seen.toasts.push(e.text)
    emit(`toast ${e.text}`)
    return { value: undefined }
  })
  on('session.start', (_, e) => ({ cwd: e.cwd }))
  on('session.end', (_, e) => ({ sessionId: e.sessionId }))
  on('classic.SessionStart', () => ({}))
  on('classic.UserPromptSubmit', () => ({}))
  on('turn.start', (_, e) => ({ turnId: e.turnId }))
  on('turn.complete', () => ({ text: '' }))
  on('prompt.submit', (_, e) => {
    seen.prompts.push(e.text)
    return { text: e.text }
  })
  on('command.run', { command: 'goal' }, async (_, e) => {
    seen.goals.push(e.args)
    if (seen.goals.length === options.holdGoalAt) {
      emit('goal held')
      await goalHeld
    }
    if (e.args !== 'clear') return { text: 'goal set' }
    enterClear()
    if (options.holdClear) await clearHeld
    return { text: options.clearUnconfirmed ? 'something else' : 'Goal cleared: tests pass' }
  })
  return seen
}

test('bare /goal drafts, the pane shows the draft and Start runs /goal', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })

  const drafting = await $.command.run({ command: 'goal', args: '', ...TYPED })
  expect(drafting.text).toContain('Drafting a goal')
  await seen.clock.advance(100)
  expect(seen.prompts.length).toBe(1)
  expect(seen.prompts[0]).toContain('mcp__goal__draft')
  expect(seen.goals.length).toBe(0)

  const shown = await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })
  expect(shown.text).toContain('goal pane')

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ ...PANE, surface })
    expect(await ui.find({ type: 'Text', text: /Draft ready/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /nix build .#goal exits 0/ })).toBeDefined()
    await ui.unmount()
  }

  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'auditor' })
  await ui.press({ key: 'start' })
  await ui.unmount()

  expect(seen.goals.length).toBe(1)
  expect(seen.goals[0]).toContain('a. the package builds Verify: nix build .#goal exits 0')
  expect(seen.goals[0]).toContain('mcp__goal__audit')
  expect(seen.goals[0]).toContain('Constraints: no switch')
  expect(seen.tasks.map(task => task.subject)).toEqual([
    'goal a: the package builds',
    'goal b: mcp__goal__audit returns VERDICT: PASS',
  ])
  expect(seen.goals[0]).toContain('Tasks #7, #8 mirror items a-b in order')

  await $.tool.call({ tool: 'TaskUpdate', taskId: '7', status: 'completed' })
  const progress = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await progress.find({ type: 'Text', text: /tasks: 1\/2 done/ })).toBeDefined()
  await progress.unmount()
})

test('a task list that cannot be filled falls back to the agent tracking items', async ($, on) => {
  const seen = engine(on, { taskCreateFails: true })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.command.run({ command: 'goal-draft', args: '', ...TYPED })
  await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })

  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'start' })
  await ui.unmount()

  expect(seen.tasks.length).toBe(0)
  expect(seen.goals[0]).toContain('Each item is tracked as a task with TaskCreate')
})

test('the auditor is off by default', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.command.run({ command: 'goal-draft', args: 'build it', ...TYPED })
  await seen.clock.advance(100)
  expect(seen.prompts[0]).toContain('Idea from the person: build it')
  await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })

  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect((await ui.find({ key: 'auditor' }))?.text).toContain('auditor: off')
  await ui.press({ key: 'start' })
  await ui.unmount()

  expect(seen.goals.length).toBe(1)
  expect(seen.goals[0]).not.toContain('mcp__goal__audit')
  expect(seen.tasks.map(task => task.subject)).toEqual(['goal a: the package builds'])
})

test('a goal set by hand is tracked and the turn limit clears it', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })

  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  for (let press = 0; press < 4; press++) await ui.press({ key: 'turns' })
  expect((await ui.find({ key: 'turns' }))?.text).toContain('turns: 10')
  await ui.unmount()

  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })
  expect(seen.goals).toEqual(['tests pass'])

  for (let turn = 0; turn < 10; turn++) {
    await $.turn.complete({ answer: 'x', durationMs: 1, isAborted: false, turnId: `t${turn}`, reason: 'answer' })
  }
  await seen.clock.advance(100)

  expect(seen.goals).toEqual(['tests pass', 'clear'])
  expect(seen.toasts.some(text => text.includes('turn limit 10'))).toBe(true)
  const pane = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await pane.find({ type: 'Text', text: /Goal cleared/ })).toBeDefined()
  await pane.unmount()
})

test('an unconfirmed clear keeps the goal running', async ($, on) => {
  const seen = engine(on, { clearUnconfirmed: true })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'stop' })
  await seen.clock.advance(100)
  expect(seen.goals).toEqual(['tests pass', 'clear'])
  expect(await ui.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  expect(seen.toasts.some(text => text.includes('did not confirm'))).toBe(true)
  await ui.unmount()
})

test('the audit tool hands the auditor the exact condition and guards its task', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: '/tmp/session.jsonl' })
  await $.command.run({ command: 'goal-draft', args: '', ...TYPED })
  await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'auditor' })
  await ui.press({ key: 'start' })
  await ui.unmount()
  expect(seen.tasks.map(task => task.id)).toEqual(['7', '8'])

  const early = await $.tool.call({ tool: 'TaskUpdate', taskId: '8', status: 'completed' })
  expect(JSON.stringify(early)).toContain('closed by mcp__goal__audit')
  expect(seen.tasks[1]?.status).toBe('pending')

  const audit = await $.tool.call({ tool: 'mcp__goal__audit' as 'mcp__goal__draft', evidence: 'cat -A output above' } as never)
  expect(seen.spawned.length).toBe(1)
  expect(seen.spawned[0]).toContain(seen.goals[0]!)
  expect(seen.spawned[0]).toContain('/tmp/session.jsonl')
  expect(seen.spawned[0]).toContain('cat -A output above')
  expect(JSON.stringify(audit)).toContain('did not start')
  expect(seen.tasks[1]?.status).toBe('pending')
})

test('the model cannot call goal:auditor directly', async ($, on) => {
  engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  const direct = await $.tool.call({ tool: 'Agent', subagent_type: 'goal:auditor', description: 'x', prompt: 'audit' })
  expect(JSON.stringify(direct)).toContain('runs only through mcp__goal__audit')
  const other = await $.tool.call({ tool: 'Agent', subagent_type: 'researcher', description: 'x', prompt: 'look' })
  expect(other.text).toBe('agent ran')
})

function turnEnd(turnId: string, reason: 'answer' | 'error' = 'answer') {
  return { answer: reason === 'answer' ? 'x' : '', durationMs: 1, isAborted: false, turnId, reason } as const
}

test('a goal Claude Code drops after an error is interrupted and set again after the next message', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal-draft', args: '', ...TYPED })
  await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await ui.press({ key: 'auditor' })
  await ui.press({ key: 'start' })
  await ui.unmount()
  await $.tool.call({ tool: 'TaskUpdate', taskId: '7', status: 'completed' })

  seen.rows.push(sentinel('set-1', false))
  await $.turn.complete(turnEnd('t0'))
  await seen.clock.advance(400)
  expect(seen.toasts.length).toBe(0)

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  expect(seen.toasts.some(text => text.includes('authentication failed'))).toBe(true)
  expect(seen.tasks.map(task => task.status)).toEqual(['completed', 'pending'])
  const interrupted = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await interrupted.find({ type: 'Text', text: /Goal interrupted/ })).toBeDefined()
  expect(await interrupted.find({ type: 'Text', text: /outcome: interrupted: authentication failed/ })).toBeDefined()
  await interrupted.unmount()

  const early = await $.tool.call({ tool: 'TaskUpdate', taskId: '8', status: 'completed' })
  expect(JSON.stringify(early)).toContain('closed by mcp__goal__audit')

  await $.prompt.submit({ text: 'a note from another plugin', wait: false, origin: { kind: 'plugin', name: 'other' } })
  await $.turn.complete(turnEnd('t2'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(1)

  await $.prompt.submit({ text: 'still logged out', ...COMPOSED })
  await $.turn.complete(turnEnd('t3', 'error'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(1)

  await $.prompt.submit({ text: 'logged in again, go on', ...COMPOSED })
  expect(seen.goals.length).toBe(1)
  await $.turn.complete(turnEnd('t4'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(2)
  expect(seen.goals[1]).toBe(seen.goals[0])

  await $.prompt.submit({ text: 'one more message', ...COMPOSED })
  await $.turn.complete(turnEnd('t5'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(2)
  const resumed = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await resumed.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  expect(await resumed.find({ type: 'Text', text: /tasks: 1\/2 done/ })).toBeDefined()
  await resumed.unmount()
})

test('the pane resumes or drops an interrupted goal', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal-draft', args: '', ...TYPED })
  await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })
  const draft = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await draft.press({ key: 'start' })
  await draft.unmount()

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  const first = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await first.press({ key: 'resume' })
  expect(seen.goals.length).toBe(2)
  expect(seen.goals[1]).toBe(seen.goals[0])
  expect(await first.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  await first.unmount()

  seen.rows.push(sentinel('forced-2', true), notice('forced-2'))
  await $.turn.complete(turnEnd('t2', 'error'))
  await seen.clock.advance(400)
  await $.turn.start({ text: 'busy', turnId: 't3' })
  const busy = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await busy.press({ key: 'resume' })
  expect(seen.goals.length).toBe(2)
  expect(seen.toasts.some(text => text.includes('when this turn ends'))).toBe(true)
  expect(await busy.find({ type: 'Text', text: /Goal interrupted/ })).toBeDefined()
  await $.turn.complete(turnEnd('t3'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(3)
  expect(await busy.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  await busy.unmount()

  seen.rows.push(sentinel('forced-3', true), notice('forced-3'))
  await $.turn.complete(turnEnd('t4', 'error'))
  await seen.clock.advance(400)
  const last = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await last.press({ key: 'drop' })
  expect(await last.find({ type: 'Text', text: /Goal cleared/ })).toBeDefined()
  await last.unmount()
  expect(seen.tasks.map(task => task.status)).toEqual(['deleted'])

  await $.prompt.submit({ text: 'anything', ...COMPOSED })
  await $.turn.complete(turnEnd('t5'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(3)
})

test('/goal typed by hand resumes or clears an interrupted goal', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  const bare = await $.command.run({ command: 'goal', args: '', ...TYPED })
  expect(bare.text).toContain('dropped the goal')
  expect(seen.goals).toEqual(['tests pass'])

  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })
  expect(seen.goals).toEqual(['tests pass', 'tests pass'])
  const resumed = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await resumed.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  expect(await resumed.find({ type: 'Text', text: /turn 1\// })).toBeDefined()
  await resumed.unmount()

  seen.rows.push(sentinel('forced-2', true), notice('forced-2'))
  await $.turn.complete(turnEnd('t2', 'error'))
  await seen.clock.advance(400)
  await $.prompt.submit({ text: 'go on', ...COMPOSED })
  await $.command.run({ command: 'goal', args: 'clear', ...TYPED })
  expect(seen.goals).toEqual(['tests pass', 'tests pass', 'clear'])
  const cleared = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await cleared.find({ type: 'Text', text: /Goal cleared/ })).toBeDefined()
  await cleared.unmount()

  await $.turn.complete(turnEnd('t3'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(3)
})

test('only a sentinel of this run that Claude Code explains with a notice interrupts the goal', async ($, on) => {
  const seen = engine(on, { clearUnconfirmed: true, holdClear: true })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })

  seen.rows.push(sentinel('earlier-goal', true, STARTED - 60_000), notice('earlier-goal'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(2000)
  expect(await ui.find({ type: 'Text', text: /Goal running/ })).toBeDefined()

  seen.rows.push(sentinel('by-hand', true), { type: 'user', uuid: 'command-row', parentUuid: 'by-hand', content: '/goal clear' })
  await $.turn.complete(turnEnd('t2', 'error'))
  await seen.clock.advance(2000)
  expect(await ui.find({ type: 'Text', text: /Goal running/ })).toBeDefined()

  seen.rows.push(sentinel('set-again', false), notice('set-again'))
  await $.turn.complete(turnEnd('t2b', 'error'))
  await seen.clock.advance(2000)
  expect(await ui.find({ type: 'Text', text: /Goal running/ })).toBeDefined()

  await ui.press({ key: 'stop' })
  void seen.clock.advance(60)
  await seen.clearEntered
  seen.rows.push(sentinel('forced-1', true), notice('forced-1', 'Goal dropped: the API key was revoked.'))
  await $.turn.complete(turnEnd('t3', 'error'))
  void seen.clock.advance(400)
  await seen.signal('run "parentUuid":"forced-1"')
  seen.releaseClear()
  await seen.signal('did not confirm')
  expect(await ui.find({ type: 'Text', text: /Goal running/ })).toBeDefined()

  await $.turn.complete(turnEnd('t4', 'error'))
  void seen.clock.advance(400)
  await seen.signal('Claude Code dropped the goal')
  expect(await ui.find({ type: 'Text', text: /Goal interrupted/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /cleared by Claude Code/ })).toBeDefined()
  await ui.unmount()
})

test('replacing an interrupted goal deletes the tasks it kept', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal-draft', args: '', ...TYPED })
  await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })
  const draft = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await draft.press({ key: 'start' })
  await draft.unmount()

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  await $.prompt.submit({ text: 'go on', ...COMPOSED })
  await $.command.run({ command: 'goal', args: 'lint passes', ...TYPED })
  expect(seen.tasks.map(task => task.status)).toEqual(['deleted'])
  expect(seen.goals.at(-1)).toBe('lint passes')
  await $.turn.complete(turnEnd('t2'))
  await seen.clock.advance(400)
  expect(seen.goals.length).toBe(2)
  const replaced = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await replaced.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  expect(await replaced.find({ type: 'Text', text: /^lint passes$/ })).toBeDefined()
  await replaced.unmount()
})

test('a /goal typed while the resume is on its way wins in the engine too', async ($, on) => {
  const seen = engine(on, { holdGoalAt: 2 })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  const pressed = ui.press({ key: 'resume' })
  await seen.signal('goal held')
  await $.command.run({ command: 'goal', args: 'lint passes', ...TYPED })
  seen.releaseGoal()
  await pressed
  expect(seen.goals).toEqual(['tests pass', 'tests pass', 'lint passes', 'lint passes'])
  expect(await ui.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /^lint passes$/ })).toBeDefined()
  await ui.unmount()
})

test('a /goal clear typed while the resume is on its way leaves no goal behind', async ($, on) => {
  const seen = engine(on, { holdGoalAt: 2 })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  const pressed = ui.press({ key: 'resume' })
  await seen.signal('goal held')
  await $.command.run({ command: 'goal', args: 'clear', ...TYPED })
  seen.releaseGoal()
  await pressed
  expect(seen.goals).toEqual(['tests pass', 'tests pass', 'clear', 'clear'])
  expect(await ui.find({ type: 'Text', text: /Goal cleared/ })).toBeDefined()
  await ui.unmount()
})

test('a resume asked while a typed /goal is running gives way to it', async ($, on) => {
  const seen = engine(on, { holdGoalAt: 2 })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  const typed = $.command.run({ command: 'goal', args: 'lint passes', ...TYPED })
  await seen.signal('goal held')
  await ui.press({ key: 'resume' })
  seen.releaseGoal()
  await typed
  expect(seen.goals).toEqual(['tests pass', 'lint passes'])
  expect(await ui.find({ type: 'Text', text: /^lint passes$/ })).toBeDefined()
  await ui.unmount()
})

test('/goal-draft over an interrupted goal deletes the tasks it kept', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.UserPromptSubmit({ prompt: 'go', transcript_path: TRANSCRIPT })
  await $.command.run({ command: 'goal-draft', args: '', ...TYPED })
  await $.tool.call({ tool: 'mcp__goal__draft', ...DRAFT })
  const draft = await $.ui.mount({ ...PANE, surface: 'terminal' })
  await draft.press({ key: 'start' })
  await draft.unmount()

  seen.rows.push(sentinel('forced-1', true), notice('forced-1'))
  await $.turn.complete(turnEnd('t1', 'error'))
  await seen.clock.advance(400)
  await $.command.run({ command: 'goal-draft', args: 'another idea', ...TYPED })
  expect(seen.tasks.map(task => task.status)).toEqual(['deleted'])
  const drafting = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await drafting.find({ type: 'Text', text: /Drafting/ })).toBeDefined()
  await drafting.unmount()
})

const SESSION_SHAPE = 'session-1/draft-1/run-4/log-1'

type Saved = { phase?: string; draft?: { objective?: string } | null; run?: { condition?: string; turns?: number } | null }

function savedSession(phase: string, run: Record<string, unknown> | null, draft: typeof DRAFT | null = null) {
  return {
    shape: SESSION_SHAPE,
    savedAt: STARTED - 60_000,
    phase,
    draft,
    run:
      run === null
        ? null
        : {
            condition: 'tests pass',
            startedAt: STARTED - 120_000,
            turns: 3,
            tokens: 1500,
            auditor: null,
            outcome: null,
            result: null,
            verdict: null,
            tasks: [],
            done: [],
            auditTask: null,
            ...run,
          },
    log: ['09:58 goal started, 10 chars'],
  }
}

async function storedWhen(
  read: () => Promise<unknown>,
  check: (saved: Saved | undefined) => boolean,
): Promise<Saved | undefined> {
  let saved: Saved | undefined
  for (let attempt = 0; attempt < 5000; attempt++) {
    saved = (await read()) as Saved | undefined
    if (check(saved)) return saved
  }
  return saved
}

test('a running goal is kept in the store under the session id, turn by turn', async ($, on) => {
  const seen = engine(on)
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  const key = `session:${SESSION}`
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })
  await $.turn.complete(turnEnd('t0'))
  await seen.clock.advance(1000)
  const saved = await storedWhen(
    async () => seen.store.get(key),
    value => value?.run?.turns === 1,
  )
  expect(saved?.phase).toBe('running')
  expect(saved?.run?.condition).toBe('tests pass')
  expect(saved?.run?.turns).toBe(1)
})

test('a restart brings a running goal back with its progress', async ($, on) => {
  const id = SESSION
  const seen = engine(on, { store: { [`session:${id}`]: savedSession('running', {}) } })
  seen.rows.push(sentinel('set-1', false))
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.SessionStart({ source: 'resume', session_id: SESSION, transcript_path: TRANSCRIPT })
  await seen.clock.advance(100)

  const pane = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await pane.find({ type: 'Text', text: /Goal running/ })).toBeDefined()
  expect(await pane.find({ type: 'Text', text: /turn 3/ })).toBeDefined()
  expect(await pane.find({ type: 'Text', text: /restored after a restart/ })).toBeDefined()
  await pane.unmount()
  expect(seen.goals.length).toBe(0)

  await $.turn.complete(turnEnd('t0'))
  const after = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await after.find({ type: 'Text', text: /turn 4/ })).toBeDefined()
  await after.unmount()
})

test('a goal met just before the restart is closed as achieved', async ($, on) => {
  const id = SESSION
  const seen = engine(on, { store: { [`session:${id}`]: savedSession('running', {}) } })
  seen.rows.push(sentinel('set-1', false), {
    type: 'attachment',
    uuid: 'met-1',
    timestamp: new Date(STARTED + 120_000).toISOString(),
    attachment: { type: 'goal_status', met: true, condition: 'tests pass', reason: 'all 12 tests pass' },
  })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.SessionStart({ source: 'resume', session_id: SESSION, transcript_path: TRANSCRIPT })
  await seen.clock.advance(100)

  const pane = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await pane.find({ type: 'Text', text: /Goal achieved/ })).toBeDefined()
  expect(await pane.find({ type: 'Text', text: /outcome: all 12 tests pass/ })).toBeDefined()
  await pane.unmount()
})

test('a goal Claude Code no longer holds after the restart is interrupted', async ($, on) => {
  const id = SESSION
  const seen = engine(on, { store: { [`session:${id}`]: savedSession('running', {}) } })
  seen.rows.push(sentinel('cleared-1', true))
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.classic.SessionStart({ source: 'resume', session_id: SESSION, transcript_path: TRANSCRIPT })
  await seen.clock.advance(100)

  const pane = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await pane.find({ type: 'Text', text: /Goal interrupted/ })).toBeDefined()
  expect(await pane.find({ type: 'Text', text: /did not restore the goal/ })).toBeDefined()
  await pane.unmount()
})

test('a draft comes back ready to start after a restart', async ($, on) => {
  const id = SESSION
  const seen = engine(on, { store: { [`session:${id}`]: savedSession('ready', null, DRAFT) } })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })

  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ type: 'Text', text: /Draft ready/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /nix build of the goal package exits 0/ })).toBeDefined()
  await ui.press({ key: 'start' })
  await ui.unmount()
  expect(seen.goals.length).toBe(1)
  expect(seen.goals[0]).toContain('a. the package builds Verify: nix build .#goal exits 0')
})

test('/clear keeps the old session saved and a resume into another session restores that one', async ($, on) => {
  const id = SESSION
  const seen = engine(on, { store: { 'session:other': savedSession('ready', null, DRAFT) } })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await $.command.run({ command: 'goal', args: 'tests pass', ...TYPED })
  await $.turn.complete(turnEnd('t0'))
  await seen.clock.advance(1000)
  await storedWhen(
    async () => seen.store.get(`session:${id}`),
    value => value?.run?.turns === 1,
  )

  await $.session.end({ reason: 'clear', sessionId: id, resume: { id } })
  const cleared = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await cleared.find({ type: 'Text', text: /No goal/ })).toBeDefined()
  await cleared.unmount()
  await seen.clock.advance(1000)
  const kept = (seen.store.get(`session:${id}`)) as Saved | undefined
  expect(kept?.phase).toBe('running')
  expect(kept?.run?.turns).toBe(1)

  await $.classic.SessionStart({ source: 'resume', session_id: 'other', transcript_path: TRANSCRIPT })
  const other = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await other.find({ type: 'Text', text: /Draft ready/ })).toBeDefined()
  await other.unmount()
})

test('closing a finished goal drops its saved session', async ($, on) => {
  const id = SESSION
  const seen = engine(on, {
    store: { [`session:${id}`]: savedSession('done', { result: 'achieved', outcome: 'condition met' }) },
  })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ type: 'Text', text: /Goal achieved/ })).toBeDefined()
  await ui.press({ key: 'close' })
  await ui.unmount()
  await seen.clock.advance(1000)
  const gone = await storedWhen(
    async () => seen.store.get(`session:${id}`),
    value => value === undefined,
  )
  expect(gone).toBeUndefined()
})

test('old and surplus saved sessions are pruned at start', async ($, on) => {
  const entries: Record<string, unknown> = {
    'session:stale': { ...savedSession('running', {}), savedAt: STARTED - 31 * 24 * 60 * 60 * 1000 },
    'session:old-shape': { ...savedSession('running', {}), shape: 'session-0' },
    settings: { auditor: false, maxTurns: 40, maxTokens: 0 },
  }
  for (let index = 0; index < 25; index++) {
    entries[`session:s${index}`] = { ...savedSession('running', {}), savedAt: STARTED - index * 1000 }
  }
  const seen = engine(on, { store: entries })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await seen.clock.advance(10_000)
  let keys: string[] = []
  for (let attempt = 0; attempt < 5000; attempt++) {
    keys = [...seen.store.keys()]
    if (keys.filter(key => key.startsWith('session:')).length <= 19) break
  }
  const sessions = keys.filter(key => key.startsWith('session:')).sort()
  expect(sessions.length).toBe(19)
  expect(sessions).not.toContain('session:stale')
  expect(sessions).not.toContain('session:old-shape')
  expect(sessions).toContain('session:s0')
  expect(sessions).not.toContain('session:s19')
  expect(keys).toContain('settings')
})
