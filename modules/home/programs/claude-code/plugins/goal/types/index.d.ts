export type GoalPhase = 'idle' | 'drafting' | 'ready' | 'running' | 'interrupted' | 'done'

export type GoalItem = { check: string; verify: string }

export type GoalDraft = {
  objective: string
  acceptance: GoalItem[]
  constraints: string[]
  notes: string
}

export type GoalSettings = {
  auditor: boolean
  maxTurns: number
  maxTokens: number
}

export type GoalRun = {
  condition: string
  startedAt: number
  turns: number
  tokens: number
  auditor: 'pass' | 'fail' | null
  outcome: string | null
  result: 'achieved' | 'failed' | 'cleared' | null
  verdict: string | null
  tasks: string[]
  done: string[]
  auditTask: string | null
}

declare module 'claude-code' {
  interface PluginState {
    goal: {
      phase: GoalPhase
      draft: Shaped<GoalDraft | null>
      settings: Shaped<GoalSettings>
      run: Shaped<GoalRun | null>
      log: Shaped<string[]>
    }
  }
}
