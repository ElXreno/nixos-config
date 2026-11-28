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

test('a quote that occurs in several outputs rests on the latest of them', () => {
  const ledger = emptyLedger()
  const pairs = [
    pair('Read /work/orch/tool/report.sh', 'echo "report: storage backend is not reachable"', CWD),
    pair('./tool/report.sh; echo "EXIT:$?"', 'report: storage backend is not reachable EXIT:1', CWD),
  ]
  const attempt = {
    subject: 'running the report tool',
    object: './tool/report.sh',
    verdict: 'dead_end',
    reason: 'no backend',
    evidence: 'report: storage backend is not reachable',
  }
  expect(merge(ledger, pairs, { attempts: [attempt] }, 2).length).toBe(1)
  expect(hits(ledger, './tool/report.sh; wc -l SUMMARY.md', CWD, new Set()).map(rec => rec.id)).toEqual(['a1'])
  expect(hits(ledger, 'ls tool', CWD, new Set())).toEqual([])
})

test('a quote may be abbreviated or carry written-out newlines, and a relative command names its object', () => {
  const ledger = emptyLedger()
  const pairs = [
    pair(
      'ls -la /work/orch',
      'total 1061 drwxr-xr-x 4 user users 6 Nov 28 20:41 . -rw-r--r-- 1 user users 391 Nov 28 20:41 SUMMARY.md drwxr-xr-x 2 user users 4 Nov 28 20:38 notes',
      CWD,
    ),
    pair('rg -n "TODO" . | head', './notes/a.txt:2:TODO: rotate the staging keys ./notes/b.txt:1:TODO: write the migration plan', CWD),
    pair('cat notes/a.txt', 'meeting notes TODO: rotate the staging keys all good here', CWD),
  ]
  const attempts = [
    {
      subject: 'directory layout',
      object: '/work/orch',
      verdict: 'confirmed',
      reason: 'holds notes',
      evidence: 'total 1061 ... -rw-r--r-- 1 user users 391 Nov 28 20:41 SUMMARY.md ... drwxr-xr-x 2 user users 4 Nov 28 20:38 notes',
    },
    {
      subject: 'search with rg',
      object: '/work/orch (rg -n todo .)',
      verdict: 'confirmed',
      reason: 'two lines',
      evidence: './notes/a.txt:2:TODO: rotate the staging keys',
    },
    {
      subject: 'first note',
      object: '/work/orch/notes/a.txt',
      verdict: 'confirmed',
      reason: 'one todo',
      evidence: 'meeting notes\\nTODO: rotate the staging keys\\nall good here',
    },
    {
      subject: 'second note',
      object: '/work/orch/notes/b.txt',
      verdict: 'confirmed',
      reason: 'never read',
      evidence: 'meeting notes ... nothing of the kind was printed',
    },
    {
      subject: 'elsewhere',
      object: '/opt/other/file',
      verdict: 'confirmed',
      reason: 'never read',
      evidence: 'meeting notes TODO: rotate the staging keys',
    },
  ]
  merge(ledger, pairs, { attempts }, 3)
  expect(ledger.records.map(rec => rec.grounded)).toEqual([true, true, true, false, false])
  const file = pair('file data.bin; echo ---', 'data.bin: ELF 64-bit LSB pie executable, x86-64', CWD)
  merge(
    ledger,
    [file],
    {
      attempts: [
        {
          subject: 'data file type',
          object: '/work/orch/data.bin',
          verdict: 'confirmed',
          reason: 'an executable',
          evidence: 'data.bin: ELF 64-bit LSB pie executable',
        },
      ],
    },
    4,
  )
  expect(hits(ledger, 'xxd -l 64 data.bin', CWD, new Set()).map(rec => rec.subject)).toEqual(['data file type'])
  expect(hits(ledger, 'xxd -l 64 other.bin', CWD, new Set())).toEqual([])
})

test('a line that many outputs share rests on the command that names the subject', () => {
  const ledger = emptyLedger()
  const pairs = [
    pair(at('./bin/diag rezafen net'), 'diag report: subsystem=rezafen angle=net result: no anomaly found related: levix', CWD),
    pair(at('./bin/diag rezafen disk'), 'diag report: subsystem=rezafen angle=disk result: no anomaly found related: remir', CWD),
    pair(at('./bin/diag levix disk'), 'diag report: subsystem=levix angle=disk result: no anomaly found related: rezafen', CWD),
  ]
  const attempt = { subject: 'rezafen net angle', verdict: 'ruled_out', reason: 'clean', evidence: 'result: no anomaly found' }
  merge(ledger, pairs, { attempts: [attempt] }, 3)
  const none = new Set<string>()
  expect(hits(ledger, at('./bin/diag rezafen net'), CWD, none).map(rec => rec.subject)).toEqual(['rezafen net angle'])
  expect(hits(ledger, at('./bin/diag levix disk'), CWD, none)).toEqual([])
  expect(hits(ledger, at('./bin/diag rezafen disk'), CWD, none)).toEqual([])
  const other = { id: 'a1', subject: 'rezafen disk angle', verdict: 'ruled_out', reason: 'clean', evidence: 'subsystem=rezafen angle=disk' }
  merge(ledger, pairs, { attempts: [other] }, 4)
  expect(ledger.records.map(rec => `${rec.id} ${rec.subject}`)).toEqual(['a1 rezafen net angle', 'a2 rezafen disk angle'])
  const reworded = {
    id: 'a1',
    subject: 'rezafen net angle check',
    verdict: 'ruled_out',
    reason: 'still clean',
    evidence: 'subsystem=rezafen angle=net',
  }
  merge(ledger, pairs, { attempts: [reworded] }, 5)
  expect(ledger.records.length).toBe(2)
  expect(ledger.records[0]?.reason).toBe('still clean')
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

type Seen = { prompts: string[]; mem: string[][]; appended: string[]; dirs: string[]; instructions: string[]; store: Map<string, unknown> }

function engine(on: On, session: string, reply: () => string, extra: Record<string, string> = {}): Seen {
  const seen: Seen = { prompts: [], mem: [], appended: [], dirs: [], instructions: [], store: new Map() }
  const env: Record<string, string> = { HOME: '/home/t', OBSERVER_EVERY_STEPS: '4', ...extra }
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
    let stderr = ''
    let exitCode = 0
    if (e.argv[0] === 'mem') {
      seen.mem.push(e.argv.slice(1))
      if (e.argv[1] === 'here') stdout = '["project:orch","host:smok"]'
      if (e.argv[1] === 'search' && extra.NO_DB !== undefined) {
        exitCode = 1
        stderr = 'mem: no memory database at /home/t/.local/share/agent-memory/mem.db; mem put creates it'
      } else if (e.argv[1] === 'search')
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
    } else {
      seen.appended.push(e.stdin ?? '')
      seen.dirs.push(e.argv[4] ?? '')
    }
    return { value: { exitCode, stdout, stderr, isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('model.fork', (_, e) => {
    seen.prompts.push(e.prompt)
    return { value: { isAnswered: true, text: reply() } as never }
  })
  on('tool.call', { tool: 'Bash' }, (_, e) => ({ result: {} as never, text: OUTPUTS[e.command] ?? '' }))
  on('session.compact', (_, e) => {
    seen.instructions.push(e.instructions ?? '')
    return {
      messages: [
        { role: 'user', text: 'summary of the work', toolUses: [] },
        { role: 'assistant', text: 'going on', toolUses: [] },
      ],
    }
  })
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
  expect(seen.instructions[0]).toContain('Leave out everything the harness supplies')
  expect(compacted.messages?.at(-1)?.text).toBe('going on')
  const last = compacted.messages?.at(0)
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

test('a session of a local agent keeps its pairs under a directory of its own', async ($, on) => {
  const seen = engine(on, 'session-c', () => '{}', { MEM_AGENT: 'local' })
  await $.tool.call({ tool: 'Bash', command: 'ls' })
  expect(seen.dirs).toEqual(['/home/t/.local/state/claude-observer/agents/local'])
})

test('searches before the memory database exists do not switch memory off', async ($, on) => {
  const seen = engine(on, 'session-d', () => JSON.stringify({ attempts: ATTEMPTS }), { NO_DB: '1' })
  for (const command of [at('./bin/start'), at('./bin/diag bogugos net'), at('./bin/scan'), 'file /work/orch/bin/*'])
    await $.tool.call({ tool: 'Bash', command })
  expect(seen.mem.filter(args => args[0] === 'search').length).toBeGreaterThan(3)
  expect(seen.mem.filter(args => args[0] === 'put').length).toBe(4)
})
