import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

import { digest, emptyLedger, hits, merge, pair, reminder } from '../hooks/core'

const CWD = '/work/orch'
const at = (command: string): string => `cd ${CWD} && ${command}; echo "EXIT=$?"`

const OUTPUTS: Record<string, string> = {
  [at('./bin/start')]: 'orchestrator failed at stage 1 (bootstrap)\nsuspects: bogugos, sivos',
  [at('./bin/diag bogugos net')]: 'diag report: subsystem=bogugos angle=net\nangle net: no anomaly',
  [at('./bin/scan')]: 'scan: the scan service is disabled on this node',
  'file /work/orch/bin/*': '/work/orch/bin/diag: ELF 64-bit LSB executable, stripped',
}

const ATTEMPTS = [
  {
    subject: 'start failure and suspects',
    object: '/work/orch/bin/start',
    verdict: 'confirmed',
    reason: 'fails at stage 1',
    evidence: 'orchestrator failed at stage 1 (bootstrap)',
  },
  {
    subject: 'bogugos net angle',
    object: 'bin/diag bogugos {net,disk,conf}',
    verdict: 'ruled_out',
    reason: 'no anomaly',
    evidence: 'angle net: no anomaly',
  },
  {
    subject: 'scan approach',
    object: 'bin/scan',
    verdict: 'dead_end',
    reason: 'scan service disabled',
    evidence: 'scan: the scan service is disabled on this node',
  },
  {
    subject: 'tool binaries',
    object: '/work/orch/bin/{start,diag,scan}',
    verdict: 'confirmed',
    reason: 'stripped executables',
    evidence: 'ELF 64-bit LSB executable, stripped',
  },
  { subject: 'phantom', verdict: 'ruled_out', reason: 'never checked', evidence: 'angle net: no anomaly' },
  { subject: 'invented', verdict: 'dead_end', reason: 'never run', evidence: 'objdump -d bin/none' },
]

function seeded() {
  const ledger = emptyLedger()
  const pairs = Object.entries(OUTPUTS).map(([call, text]) => pair(call, text.replace(/\s+/g, ' '), CWD))
  const changed = merge(ledger, pairs, { attempts: ATTEMPTS, open: ['vetus'], next: 'diag vetus net' }, 4)
  return { ledger, pairs, changed }
}

test('a pass keeps only records that rest on a stored command or output', () => {
  const { ledger, changed } = seeded()
  expect(changed.filter(rec => rec.grounded).map(rec => rec.subject)).toEqual([
    'start failure and suspects',
    'bogugos net angle',
    'scan approach',
    'tool binaries',
  ])
  expect(ledger.records.filter(rec => !rec.grounded).map(rec => rec.subject)).toEqual(['phantom', 'invented'])
  expect(ledger.open).toEqual(['vetus'])
  expect(
    merge(
      ledger,
      [],
      { attempts: [{ id: 'a3', subject: 'scan approach', verdict: 'confirmed', reason: 'made up', evidence: 'nothing stored' }] },
      5,
    ),
  ).toEqual([])
})

test('a tool that was run is not an examined object, and cd is not part of a command', () => {
  const { ledger } = seeded()
  const none = new Set<string>()
  expect(hits(ledger, at('./bin/diag sivos disk'), CWD, none)).toEqual([])
  expect(hits(ledger, at('./bin/diag bogugos disk'), CWD, none)).toEqual([])
  expect(hits(ledger, at('./bin/start'), CWD, none)).toEqual([])
  expect(hits(ledger, at('./bin/diag bogugos net'), CWD, none).map(rec => rec.subject)).toEqual(['bogugos net angle'])
  expect(hits(ledger, '/work/orch/bin/scan --all', '/tmp', none).map(rec => rec.subject)).toEqual(['scan approach'])
  const strings = hits(ledger, at('strings -n 4 bin/diag | head -200'), CWD, none)
  expect(strings.map(rec => rec.subject)).toEqual(['tool binaries'])
  expect(reminder(strings)).toContain('/work/orch/bin/{start,diag,scan} was already examined')
  expect(hits(ledger, at('strings -n 4 bin/diag'), CWD, new Set(['a4']))).toEqual([])
})

test('the digest lists dead ends, exclusions, objects and open leads', () => {
  const text = digest(seeded().ledger)
  expect(text).toContain('Dead ends, do not try these again:\na3 scan approach')
  expect(text).toContain('Ruled out, do not re-examine without a new reason:\na2 bogugos net angle')
  expect(text).toContain('Objects already examined:\na1 /work/orch/bin/start: fails at stage 1')
  expect(text).toContain('Open leads:\nvetus')
  expect(text).not.toContain('phantom')
  expect(digest(emptyLedger())).toBe('')
})

type Seen = { prompts: string[]; mem: string[][]; appended: string[]; store: Map<string, unknown> }

function engine(on: On, session: string, reply: () => string): Seen {
  const seen: Seen = { prompts: [], mem: [], appended: [], store: new Map() }
  const env: Record<string, string> = { HOME: '/home/t', OBSERVER_EVERY_STEPS: '4' }
  mock.clock(on, { now: Date.parse('2026-11-28T10:00:00Z') })
  on('store.get', (_, e) => ({ value: seen.store.get(e.key) }))
  on('store.set', (_, e) => {
    seen.store.set(e.key, e.value)
    return { value: undefined }
  })
  on('store.delete', (_, e) => {
    seen.store.delete(e.key)
    return { value: undefined }
  })
  on('store.keys', () => ({ value: [...seen.store.keys()] }))
  on('session.id', () => ({ value: session }))
  on('session.cwd', () => ({ value: CWD }))
  on('env.get', (_, e) => ({ value: env[e.name] }))
  on('fs.exists', () => ({ value: false }))
  on('process.run', (_, e) => {
    let stdout = ''
    let exitCode = 0
    if (e.argv[0] === 'mem') {
      seen.mem.push(e.argv.slice(1))
      if (e.argv[1] === 'here') stdout = '["project:orch","host:smok"]'
      if (e.argv[1] === 'search')
        stdout = JSON.stringify(
          e.argv.at(-1)?.includes('replay')
            ? [
                {
                  record: {
                    key: 'ledger_orch_replay',
                    title: 'replay stored reports',
                    body: 'Report retention is off.',
                    scopes: ['project:orch', 'topic:ledger'],
                  },
                  rerank: 0.9,
                },
              ]
            : [],
        )
    } else seen.appended.push(e.stdin ?? '')
    return { value: { exitCode, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('model.fork', (_, e) => {
    seen.prompts.push(e.prompt)
    return { value: { isAnswered: true, text: reply() } as never }
  })
  on('tool.call', { tool: 'Bash' }, (_, e) => ({ result: {} as never, text: OUTPUTS[e.command] ?? '' }))
  on('session.compact', () => ({ messages: [{ role: 'user', text: 'summary of the work', toolUses: [] }] }))
  return seen
}

test('tool calls feed the pass, and its records come back with later results and after a compaction', async ($, on) => {
  const seen = engine(on, 'session-a', () => `ok ${JSON.stringify({ attempts: ATTEMPTS, open: ['vetus'], next: 'diag vetus net' })}`)
  const run = (command: string) => $.tool.call({ tool: 'Bash', command })
  await run(at('./bin/start'))
  await run(at('./bin/diag bogugos net'))
  await run(at('./bin/scan'))
  expect(seen.prompts).toEqual([])
  const fourth = await run('file /work/orch/bin/*')
  expect(seen.prompts.length).toBe(1)
  expect(seen.prompts[0]).toContain('Registry so far:\n(empty)')
  expect((fourth.context ?? []).join('\n')).toContain('Registry update:\na1 start failure and suspects: confirmed')
  expect(seen.appended.length).toBe(4)
  const puts = seen.mem.filter(args => args[0] === 'put').map(args => args[1])
  expect(puts).toEqual([
    'ledger_orch_start_failure_and_suspects',
    'ledger_orch_bogugos_net_angle',
    'ledger_orch_scan_approach',
    'ledger_orch_tool_binaries',
  ])
  expect(seen.mem.find(args => args[0] === 'put')).toContain('topic:ledger')

  const again = await run(at('./bin/scan'))
  expect((again.context ?? []).join('\n')).toContain('scan approach is closed as dead_end')
  expect((await run(at('./bin/scan'))).context ?? []).toEqual([])
  const replay = await run(at('./bin/replay kedevos'))
  expect((replay.context ?? []).join('\n')).toContain('Memory of this project has related records:\nreplay stored reports')
  expect((await run(at('./bin/replay kedevos'))).context ?? []).toEqual([])

  const compacted = await $.session.compact({ trigger: 'manual', messages: [{ role: 'user', text: 'the task', toolUses: [] }] } as never)
  expect(seen.prompts.length).toBe(2)
  const last = compacted.messages?.at(-1)
  expect(last?.text).toContain('summary of the work\n\nRegistry kept outside the conversation:')
  expect(last?.text).toContain('Dead ends, do not try these again:\na3 scan approach')
  const after = await run(at('./bin/scan'))
  expect((after.context ?? []).join('\n')).toContain('scan approach is closed as dead_end')
  expect([...seen.store.keys()]).toEqual(['session:session-a'])
})

test('a subagent call and a denied call leave the registry alone', async ($, on) => {
  const seen = engine(on, 'session-b', () => '{}')
  await $.tool.call({ tool: 'Bash', command: 'ls', agentId: 'a1' } as never)
  expect(seen.appended).toEqual([])
})
