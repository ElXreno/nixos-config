import type { EngineInterface, Register } from 'claude-code'

import {
  INSTRUCTION,
  delta,
  digest,
  emptyLedger,
  extractJson,
  hits,
  isClosed,
  merge,
  pair,
  registryText,
  reminder,
  slug,
  squash,
} from './core'
import type { Ledger, Pair, Rec } from './core'

type Config = {
  enabled: boolean
  everySteps: number
  beforeCompact: boolean
  feedback: boolean
  memory: boolean
  pairChars: number
  pairsKept: number
  registryLines: number
  memLimit: number
  minRerank: number
  minOverlap: number
  memTimeoutMs: number
}

type Session = {
  id: string
  ledger: Ledger
  pairs: Pair[]
  file: string
  dir: string
  steps: number
  lastPass: number
  savedVersion: number
  digestVersion: number
  afterCompact: boolean
  passing: boolean
  nudged: Set<string>
  recalled: Set<string>
  scope: string
  memFailures: number
}

type Saved = { ledger: Ledger; steps: number; lastPass: number; savedAt: number }

type Engine = EngineInterface

const SESSION_PREFIX = 'session:'
const SESSION_TTL = 30 * 24 * 60 * 60 * 1000
const LEDGER_TOPIC = 'topic:ledger'

function isSaved(value: unknown): value is Saved {
  const saved = value as Saved | undefined
  return typeof saved === 'object' && saved !== null && typeof saved.ledger === 'object' && Array.isArray(saved.ledger?.records)
}

let config: Config | undefined
let current: Session | undefined
let pruned = false

function number(raw: string | undefined, fallback: number): number {
  const value = raw === undefined || raw === '' ? Number.NaN : Number(raw)
  return Number.isFinite(value) && value >= 0 ? value : fallback
}

async function settings($: Engine): Promise<Config> {
  if (config !== undefined) return config
  config = {
    enabled: number(await $.env.get('OBSERVER').catch(() => undefined), 1) !== 0,
    everySteps: number(await $.env.get('OBSERVER_EVERY_STEPS').catch(() => undefined), 0),
    beforeCompact: number(await $.env.get('OBSERVER_BEFORE_COMPACT').catch(() => undefined), 1) !== 0,
    feedback: number(await $.env.get('OBSERVER_FEEDBACK').catch(() => undefined), 1) !== 0,
    memory: number(await $.env.get('OBSERVER_MEMORY').catch(() => undefined), 1) !== 0,
    pairChars: 20000,
    pairsKept: 400,
    registryLines: 200,
    memLimit: 3,
    minRerank: 0.6,
    minOverlap: 0.5,
    memTimeoutMs: 6000,
  }
  return config
}

async function mem($: Engine, s: Session, cfg: Config, args: string[]): Promise<{ exitCode: number; stdout: string }> {
  const answer = await $.process.run(['mem', ...args], { timeoutMs: cfg.memTimeoutMs }).catch(() => ({ exitCode: -1, stdout: '' }))
  s.memFailures = answer.exitCode === 0 || answer.exitCode === 3 ? 0 : s.memFailures + 1
  return answer
}

async function prune($: Engine, now: number): Promise<void> {
  if (pruned) return
  pruned = true
  for (const key of await $.store.keys().catch(() => [] as string[])) {
    if (!key.startsWith(SESSION_PREFIX)) continue
    const saved = await $.store.get(key).catch(() => undefined)
    if (!isSaved(saved) || now - saved.savedAt > SESSION_TTL) await $.store.delete(key).catch(() => undefined)
  }
}

async function open($: Engine): Promise<Session | undefined> {
  const cfg = await settings($)
  if (!cfg.enabled) return undefined
  const id = await $.session.id().catch(() => '')
  if (id === '') return undefined
  if (current?.id === id) return current
  const home = (await $.env.get('HOME').catch(() => undefined)) ?? ''
  const state = (await $.env.get('XDG_STATE_HOME').catch(() => undefined)) || `${home}/.local/state`
  const dir = `${state}/claude-observer`
  const saved = await $.store.get(`${SESSION_PREFIX}${id}`).catch(() => undefined)
  const s: Session = {
    id,
    ledger: isSaved(saved) ? saved.ledger : emptyLedger(),
    pairs: [],
    file: `${dir}/${id}.pairs.jsonl`,
    dir,
    steps: isSaved(saved) ? saved.steps : 0,
    lastPass: isSaved(saved) ? saved.lastPass : 0,
    savedVersion: -1,
    digestVersion: -1,
    afterCompact: isSaved(saved) && saved.ledger.records.length > 0,
    passing: false,
    nudged: new Set(),
    recalled: new Set(),
    scope: '',
    memFailures: 0,
  }
  s.savedVersion = s.ledger.version
  if (await $.fs.exists(s.file).catch(() => false)) {
    const text = await $.fs.read(s.file).catch(() => '')
    for (const line of typeof text === 'string' ? text.split('\n') : []) {
      try {
        const entry = JSON.parse(line) as Partial<Pair>
        if (typeof entry.call === 'string' && typeof entry.text === 'string') s.pairs.push(pair(entry.call, entry.text, entry.cwd ?? '/'))
      } catch {
        continue
      }
    }
    s.pairs = s.pairs.slice(-cfg.pairsKept)
  }
  if (cfg.memory) {
    const cwd = await $.session.cwd().catch(() => '')
    const here = await mem($, s, cfg, ['here', '--json', ...(cwd === '' ? [] : ['--cwd', cwd])])
    try {
      const scopes = JSON.parse(here.stdout || '[]') as string[]
      s.scope = scopes.find(scope => scope.startsWith('project:')) ?? scopes.find(scope => scope.startsWith('host:')) ?? ''
    } catch {
      s.scope = ''
    }
  }
  await prune($, await $.clock.now().catch(() => Date.now()))
  current = s
  return s
}

async function save($: Engine, s: Session): Promise<void> {
  if (s.savedVersion === s.ledger.version && s.steps % 10 !== 0) return
  s.savedVersion = s.ledger.version
  const saved: Saved = { ledger: s.ledger, steps: s.steps, lastPass: s.lastPass, savedAt: await $.clock.now().catch(() => Date.now()) }
  await $.store.set(`${SESSION_PREFIX}${s.id}`, saved).catch(() => undefined)
}

function memKey(s: Session, rec: Rec): string {
  return `ledger_${slug(s.scope.replace(/^[a-z]+:/, ''))}_${slug(rec.key)}`.slice(0, 96).replace(/_+$/, '')
}

async function remember($: Engine, s: Session, cfg: Config, rec: Rec): Promise<void> {
  if (!cfg.memory || s.scope === '' || s.memFailures >= 3 || !rec.grounded || !(rec.object !== '' || isClosed(rec))) return
  const key = memKey(s, rec)
  const body = [
    `${(rec.reason || rec.subject).replace(/\.+$/, '')}.`,
    `Verdict: ${rec.verdict}.`,
    rec.object === '' ? '' : `Object: ${rec.object}.`,
    `Evidence: ${rec.evidence}`,
  ]
    .filter(part => part !== '')
    .join(' ')
  const why = ['--why', 'observer registry']
  const created = await mem($, s, cfg, [
    'put',
    key,
    '--kind',
    'fact',
    '--scope',
    s.scope,
    '--scope',
    LEDGER_TOPIC,
    '--title',
    rec.subject.slice(0, 120),
    '--no-gate',
    ...why,
    '--body',
    body,
  ])
  if (created.exitCode === 3) await mem($, s, cfg, ['put', key, '--update', ...why, '--body', body])
}

async function recall($: Engine, s: Session, cfg: Config, query: string): Promise<string> {
  if (!cfg.memory || s.scope === '' || s.memFailures >= 3 || query === '') return ''
  const base = ['search', '--json', '--scope', LEDGER_TOPIC, '--kind', 'fact', '--limit', String(cfg.memLimit * 2)]
  let answer = await mem($, s, cfg, [...base, '--', query.slice(0, 600)])
  if (answer.exitCode !== 0) answer = await mem($, s, cfg, [...base, '--mode', 'fused', '--', query.slice(0, 600)])
  if (answer.exitCode !== 0) return ''
  type Hit = { record: { key: string; title: string; body: string; scopes: string[] }; rerank?: number | null; overlap?: number }
  let found: Hit[] = []
  try {
    found = JSON.parse(answer.stdout || '[]') as Hit[]
  } catch {
    return ''
  }
  const own = new Set(s.ledger.records.map(rec => memKey(s, rec)))
  const fresh = found
    .filter(hit => hit.record.scopes.includes(s.scope) && !own.has(hit.record.key) && !s.recalled.has(hit.record.key))
    .filter(hit => (typeof hit.rerank === 'number' ? hit.rerank >= cfg.minRerank : (hit.overlap ?? 0) >= cfg.minOverlap))
    .slice(0, cfg.memLimit)
  if (fresh.length === 0) return ''
  for (const hit of fresh) s.recalled.add(hit.record.key)
  return (
    'Memory of this project has related records:\n' +
    fresh.map(hit => `${hit.record.title}: ${squash(hit.record.body).slice(0, 400)}`).join('\n') +
    '\nIf this step repeats one of them, state what the earlier result leaves open; otherwise go on.'
  )
}

async function pass($: Engine, s: Session, cfg: Config): Promise<Rec[]> {
  if (s.passing) return []
  s.passing = true
  try {
    s.lastPass = s.steps
    const reply = await $.model.fork({ prompt: INSTRUCTION + registryText(s.ledger, cfg.registryLines) })
    if (!reply.isAnswered) return []
    const data = extractJson(reply.text)
    if (data === undefined) return []
    const changed = merge(s.ledger, s.pairs, data, s.steps)
    await save($, s)
    for (const rec of changed) await remember($, s, cfg, rec)
    return changed
  } finally {
    s.passing = false
  }
}

function callText(e: Record<string, unknown>): string {
  const { tool, tool_use_id: _id, agentId: _agent, requestMeta: _meta, ...args } = e
  if (tool === 'Bash' && typeof args.command === 'string') return squash(args.command)
  const path = typeof args.file_path === 'string' ? args.file_path : typeof args.path === 'string' ? args.path : undefined
  if (path !== undefined) return squash(`${String(tool)} ${typeof args.pattern === 'string' ? `${args.pattern} ` : ''}${path}`)
  return squash(`${String(tool)} ${JSON.stringify(args)}`).slice(0, 2000)
}

export const register: Register = on => {
  on('tool.call', async ($, e, next) => {
    const res = await next(e)
    if (e.agentId !== undefined) return res
    const s = await open($).catch(() => undefined)
    if (s === undefined) return res
    const cfg = await settings($)
    const input = e as unknown as Record<string, unknown>
    const outcome = res as { text?: string; result?: unknown; deny?: string; isError?: boolean; context?: readonly string[] }
    if (outcome.deny !== undefined) return res
    const cwd = await $.session.cwd().catch(() => '/')
    const call = callText(input)
    const text = squash(typeof outcome.text === 'string' ? outcome.text : JSON.stringify(outcome.result ?? '')).slice(0, cfg.pairChars)
    const entry = pair(call, text, cwd)
    s.pairs.push(entry)
    if (s.pairs.length > cfg.pairsKept) s.pairs.splice(0, s.pairs.length - cfg.pairsKept)
    s.steps++
    const line = `${JSON.stringify({ call: entry.call, text: entry.text, cwd })}\n`
    await $.process
      .run(['sh', '-c', 'mkdir -p "$1" && cat >> "$2"', 'sh', s.dir, s.file], { stdin: line, timeoutMs: 5000 })
      .catch(() => undefined)
    const notes: string[] = []
    if (cfg.feedback) {
      if (s.afterCompact) {
        s.afterCompact = false
        const whole = s.ledger.version === s.digestVersion ? '' : digest(s.ledger)
        if (whole !== '') notes.push(whole)
      }
      const found = hits(s.ledger, call, cwd, s.nudged)
      if (found.length > 0) {
        for (const rec of found) s.nudged.add(rec.id)
        notes.push(reminder(found))
      } else if (input.tool === 'Bash') {
        const memory = await recall($, s, cfg, call)
        if (memory !== '') notes.push(memory)
      }
    }
    if (cfg.everySteps > 0 && s.steps - s.lastPass >= cfg.everySteps) {
      const update = delta(await pass($, s, cfg))
      if (cfg.feedback && update !== '') notes.push(update)
    }
    await save($, s)
    if (notes.length === 0 || outcome.isError === true || outcome.result === undefined) return res
    return { ...res, context: [...(outcome.context ?? []), ...notes] } as typeof res
  })

  on('session.compact', async ($, e, next) => {
    if (e.agentId !== undefined) return next(e)
    const s = await open($).catch(() => undefined)
    if (s === undefined) return next(e)
    const cfg = await settings($)
    if (cfg.beforeCompact && e.trigger !== 'precompute' && s.steps > s.lastPass) await pass($, s, cfg).catch(() => [])
    const res = await next(e)
    if (res.skip !== undefined) return res
    s.nudged.clear()
    s.recalled.clear()
    s.afterCompact = true
    const whole = cfg.feedback ? digest(s.ledger) : ''
    const last = res.messages[res.messages.length - 1]
    if (whole === '' || last === undefined) return res
    s.digestVersion = s.ledger.version
    return { ...res, messages: [...res.messages.slice(0, -1), { ...last, text: `${last.text}\n\n${whole}` }] }
  })
}
