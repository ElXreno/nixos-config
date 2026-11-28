export type Verdict = 'dead_end' | 'ruled_out' | 'confirmed' | 'inconclusive' | 'in_progress'

export type Rec = {
  id: string
  key: string
  subject: string
  object: string
  verdict: Verdict
  reason: string
  evidence: string
  grounded: boolean
  signature: string[]
  objects: string[]
  step: number
}

export type Pair = { call: string; text: string; cwd: string; lower: string }

export type Ledger = { records: Rec[]; open: string[]; next: string; seq: number; version: number }

const VERDICTS: readonly string[] = ['dead_end', 'ruled_out', 'confirmed', 'inconclusive', 'in_progress']
const CLOSED: readonly string[] = ['dead_end', 'ruled_out']
const MOVES: readonly string[] = ['cd', 'pushd']

export const INSTRUCTION = `Observer pass. This message is not part of the task. Do not call any tool and do not continue the task in this reply. Reply with one JSON object and nothing else.

Record what the conversation above has established. Use only what a tool call or tool result above supports; do not record guesses. Prefer one line per component, object or approach over one line per command. For every object you examined (a file, a binary, a process, a socket, an endpoint, a function) keep one line that says what it is and what examining it showed, and put its name or path in "object". Reuse the id of a registry line when you update it.

Verdicts: dead_end is an approach or tool that was tried and gave nothing useful, so it should not be tried again; ruled_out is a candidate cause that was checked and excluded, a suspect that turned out healthy included; confirmed is an established fact; in_progress is something partly examined; inconclusive is something examined without a clear result.

{
  "attempts": [{"id": "id of a registry line to update, otherwise omit", "subject": "short name of what was examined: a component, an object, a hypothesis or an approach", "object": "name or path of the examined object, when the line is about one", "verdict": "dead_end | ruled_out | confirmed | inconclusive | in_progress", "reason": "one sentence", "evidence": "one line copied exactly from a tool result above, or the exact command that was run when its result was empty or useless"}],
  "open": ["leads that were noticed and not examined yet"],
  "next": "the step you were about to take"
}

Registry so far:
`

export const COMPACT_NOTE =
  'Summarize only what the user asked for and what the work established. Leave out everything the harness supplies: the system prompt, the environment block, tool and skill listings, memory and hook reminders, attribution and safety rules. They are sent again after the summary, so repeating them only spends its room. Do not copy lines of the registry kept outside the conversation either: it is appended after your summary.'

export function emptyLedger(): Ledger {
  return { records: [], open: [], next: '', seq: 0, version: 0 }
}

export function squash(text: unknown): string {
  return String(text).replace(/\s+/g, ' ').trim()
}

export function pair(call: string, text: string, cwd: string): Pair {
  return { call, text, cwd, lower: `${call} ${text}`.toLowerCase() }
}

export function isClosed(rec: Rec): boolean {
  return CLOSED.includes(rec.verdict)
}

export function extractJson(text: string): Record<string, unknown> | undefined {
  const start = text.indexOf('{')
  const end = text.lastIndexOf('}')
  if (start < 0 || end <= start) return undefined
  try {
    const parsed: unknown = JSON.parse(text.slice(start, end + 1))
    return typeof parsed === 'object' && parsed !== null ? (parsed as Record<string, unknown>) : undefined
  } catch {
    return undefined
  }
}

export function renderRecord(rec: Rec): string {
  return `${rec.id} ${rec.subject}: ${rec.verdict}${rec.reason ? ` (${rec.reason})` : ''}`
}

export function registryText(ledger: Ledger, lines: number): string {
  const shown = ledger.records.slice(-lines).map(renderRecord)
  return shown.length > 0 ? shown.join('\n') : '(empty)'
}

function identifiers(text: string): string[] {
  return (squash(text).match(/[A-Za-z0-9_./-]{4,}/g) ?? []).map(token => token.toLowerCase().replace(/^\.\//, ''))
}

export function resolvePath(dir: string, path: string): string {
  const out: string[] = []
  for (const part of (path.startsWith('/') ? path : `${dir}/${path}`).split('/')) {
    if (part === '' || part === '.') continue
    if (part === '..') out.pop()
    else out.push(part)
  }
  return `/${out.join('/')}`
}

function locate(dir: string, token: string): string {
  const cut = token.replace(/[*?{[].*$/, '')
  if (cut === '') return ''
  const path = resolvePath(dir, cut).toLowerCase()
  return cut.endsWith('/') && path !== '/' ? `${path}/` : path
}

type Segment = { program: string; args: string[]; paths: string[] }

function segments(command: string, cwd: string): Segment[] {
  let dir = cwd
  const found: Segment[] = []
  for (const part of command.split(/[;|&\n]/)) {
    const words = part
      .trim()
      .split(/\s+/)
      .map(word => word.replace(/^\d*[<>]+&?/, '').replace(/^[("'`]+|[)"'`]+$/g, ''))
      .filter(word => word !== '' && !/^[A-Za-z_][A-Za-z0-9_]*=/.test(word))
    const head = words[0]
    if (head === undefined) continue
    if (MOVES.includes(head)) {
      if (words[1] !== undefined) dir = resolvePath(dir, words[1])
      continue
    }
    const place = (word: string): string => (word.includes('/') ? locate(dir, word) : word.toLowerCase())
    const rest = words.slice(1).filter(word => !word.startsWith('-') && !/^\d+$/.test(word))
    found.push({
      program: place(head),
      paths: rest.map(word => locate(dir, word)).filter(word => word !== ''),
      args: words
        .slice(1)
        .filter(word => !word.startsWith('-') && !/^\d+$/.test(word))
        .map(place)
        .filter(word => word !== ''),
    })
  }
  return found
}

function signature(call: string, cwd: string): string[] {
  const first = segments(call, cwd)[0]
  if (first === undefined) return []
  const tokens = [first.program, ...first.args]
  return tokens.length >= 2 || first.program.includes('/') ? tokens : []
}

function examined(command: string, cwd: string): string[] {
  return [...new Set(segments(command, cwd).flatMap(segment => segment.paths))]
}

function commandTokens(command: string, cwd: string): Set<string> {
  return new Set(segments(command, cwd).flatMap(segment => [segment.program, ...segment.args]))
}

function located(text: string, cwd: string): string[] {
  return identifiers(text)
    .map(token => (token.includes('/') ? locate(cwd, token) : token))
    .filter(token => token !== '')
}

function covers(seen: readonly string[], object: string): boolean {
  return seen.some(
    path => path === object || (object.endsWith('/') && path.startsWith(object)) || (path.endsWith('/') && object.startsWith(path)),
  )
}

function names(lower: string, path: string): boolean {
  if (lower.includes(path)) return true
  const cut = path.lastIndexOf('/')
  return cut > 0 && lower.includes(path.slice(0, cut)) && lower.includes(path.slice(cut + 1))
}

function named(entry: Pair, path: string): boolean {
  if (names(entry.lower, path)) return true
  const cwd = entry.cwd.toLowerCase()
  if (path === cwd || cwd.startsWith(`${path}/`)) return true
  if (!path.startsWith(`${cwd}/`)) return false
  const inside = path.slice(cwd.length + 1)
  return inside !== '' && names(entry.lower, inside)
}

function quoted(text: string, evidence: string): boolean {
  const parts = evidence.split(/\s*(?:\.\.\.|\u2026)\s*/).filter(part => part.length >= 8)
  return parts.length > 0 && parts.every(part => text.includes(part))
}

function objectsOf(object: string, source: Pair): string[] {
  const seen = examined(source.call, source.cwd).flatMap(path => (path.endsWith('/') ? [path] : [path, `${path}/`]))
  return [...new Set(located(object, source.cwd).filter(token => token.startsWith('/') && covers(seen, token)))]
}

function sameSubject(before: string, after: string): boolean {
  const words = (text: string): Set<string> => new Set(text.toLowerCase().match(/[a-z0-9_./-]{3,}/g) ?? [])
  const old = words(before)
  const next = words(after)
  const shared = [...old].filter(word => next.has(word)).length
  const all = new Set([...old, ...next]).size
  return all === 0 || shared / all >= 0.6
}

export function merge(ledger: Ledger, pairs: readonly Pair[], data: Record<string, unknown>, step: number): Rec[] {
  const changed: Rec[] = []
  const attempts: unknown[] = Array.isArray(data.attempts) ? data.attempts : []
  for (const raw of attempts) {
    if (typeof raw !== 'object' || raw === null) continue
    const attempt = raw as Record<string, unknown>
    if (typeof attempt.subject !== 'string' || typeof attempt.verdict !== 'string' || !VERDICTS.includes(attempt.verdict)) continue
    const subject = squash(attempt.subject).slice(0, 200)
    if (subject === '') continue
    const evidence = typeof attempt.evidence === 'string' ? squash(attempt.evidence.replace(/\\[nrt]/g, ' ')) : ''
    const object = typeof attempt.object === 'string' ? squash(attempt.object).toLowerCase().slice(0, 200) : ''
    const tokens = [...identifiers(subject), ...identifiers(object)]
    const need = Math.max(1, Math.ceil(tokens.length / 2))
    const paths = identifiers(object)
      .filter(token => token.includes('/'))
      .map(token => token.replace(/\/+$/, ''))
      .filter(token => token !== '')
    const about = (entry: Pair): boolean =>
      paths.some(path => named(entry, path)) || tokens.filter(token => entry.lower.includes(token)).length >= need
    const words = `${subject} ${object}`.toLowerCase().match(/[a-z0-9_./-]{3,}/g) ?? []
    let source: Pair | undefined
    let best = -1
    for (const entry of evidence.length >= 8 ? pairs : []) {
      const inCall = entry.call.includes(evidence)
      if (!inCall && !(quoted(entry.text, evidence) && about(entry))) continue
      const call = entry.call.toLowerCase()
      const score = inCall ? words.length + 1 : words.filter(word => call.includes(word)).length
      if (score >= best) {
        best = score
        source = entry
      }
    }
    const grounded = source !== undefined
    const byId = typeof attempt.id === 'string' ? ledger.records.find(rec => rec.id === attempt.id) : undefined
    const key = byId !== undefined && sameSubject(byId.subject, subject) ? byId.key : subject.toLowerCase()
    const at = ledger.records.findIndex(rec => rec.key === key)
    const previous = at >= 0 ? ledger.records[at] : undefined
    if (previous?.grounded === true && !grounded) continue
    const record: Rec = {
      id: previous?.id ?? `a${++ledger.seq}`,
      key,
      subject: previous?.subject ?? subject,
      object: object || previous?.object || '',
      verdict: attempt.verdict as Verdict,
      reason: typeof attempt.reason === 'string' ? squash(attempt.reason).slice(0, 300) : '',
      evidence: evidence.slice(0, 400),
      grounded,
      signature: source ? signature(source.call, source.cwd) : [],
      objects: source ? objectsOf(object, source) : [],
      step,
    }
    if (previous !== undefined && previous.verdict === record.verdict && previous.reason === record.reason) continue
    if (at >= 0) ledger.records[at] = record
    else ledger.records.push(record)
    changed.push(record)
  }
  if (Array.isArray(data.open))
    ledger.open = data.open
      .filter((item): item is string => typeof item === 'string')
      .map(squash)
      .slice(0, 12)
  if (typeof data.next === 'string') ledger.next = squash(data.next).slice(0, 300)
  if (changed.length > 0) ledger.version++
  return changed
}

export function digest(ledger: Ledger): string {
  const grounded = ledger.records.filter(rec => rec.grounded)
  const about = grounded.filter(rec => rec.object !== '' && !isClosed(rec))
  const rest = grounded.filter(rec => !about.includes(rec))
  const section = (title: string, verdict: Verdict): string => {
    const lines = rest.filter(rec => rec.verdict === verdict).map(renderRecord)
    return lines.length > 0 ? `${title}:\n${lines.join('\n')}\n` : ''
  }
  const body =
    section('Dead ends, do not try these again', 'dead_end') +
    section('Ruled out, do not re-examine without a new reason', 'ruled_out') +
    (about.length > 0
      ? `Objects already examined:\n${about.map(rec => `${rec.id} ${rec.object}: ${rec.reason || rec.subject}`).join('\n')}\n`
      : '') +
    section('Confirmed', 'confirmed') +
    section('In progress', 'in_progress') +
    section('Inconclusive', 'inconclusive') +
    (ledger.open.length > 0 ? `Open leads:\n${ledger.open.join('\n')}\n` : '') +
    (ledger.next !== '' ? `Planned next step before the checkpoint: ${ledger.next}\n` : '')
  return body === '' ? '' : `Registry kept outside the conversation:\n${body}`
}

export function delta(changed: readonly Rec[]): string {
  const lines = changed.filter(rec => rec.grounded).map(renderRecord)
  return lines.length > 0 ? `Registry update:\n${lines.join('\n')}` : ''
}

export function hits(ledger: Ledger, command: string, cwd: string, nudged: ReadonlySet<string>): Rec[] {
  const seen = commandTokens(command, cwd)
  const touched = examined(command, cwd)
  const all = (tokens: readonly string[]): boolean => tokens.length > 0 && tokens.every(token => seen.has(token))
  return ledger.records.filter(rec => {
    if (!rec.grounded || nudged.has(rec.id)) return false
    if (rec.objects.some(object => covers(touched, object))) return true
    return isClosed(rec) && (all(located(rec.subject, cwd)) || all(rec.signature))
  })
}

export function reminder(found: readonly Rec[]): string {
  return (
    found
      .map(rec =>
        isClosed(rec)
          ? `Registry: ${rec.subject} is closed as ${rec.verdict} (${rec.reason || 'no reason recorded'}); evidence: ${rec.evidence}`
          : `Registry: ${rec.object || rec.subject} was already examined (${rec.reason || rec.subject}); evidence: ${rec.evidence}`,
      )
      .join('\n') + '\nIf you examine it again, state what the earlier result leaves open; otherwise go on with another lead.'
  )
}

export function slug(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
}
