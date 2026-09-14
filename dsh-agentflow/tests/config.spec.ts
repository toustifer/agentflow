import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildMcpConfig, Config, resolveCommand, type AgentflowConfig } from '../src/config.js'

describe('Config schema defaults', () => {
  it('applies defaults for omitted fields', () => {
    const resolved = Config({}) as AgentflowConfig
    expect(resolved.serverName).toBe('agentflow')
    expect(resolved.args).toEqual(['stdio'])
    expect(resolved.syncSkill).toBe(true)
    expect(resolved.toolCallTimeoutMs).toBe(60_000)
    expect(resolved.failOnStartupError).toBe(false)
    expect(resolved.dbPath).toBe('')
  })

  it('keeps provided values', () => {
    const resolved = Config({ serverName: 'af2', command: 'C:/x/agentflow.exe', syncSkill: false }) as AgentflowConfig
    expect(resolved.serverName).toBe('af2')
    expect(resolved.command).toBe('C:/x/agentflow.exe')
    expect(resolved.syncSkill).toBe(false)
  })
})

describe('resolveCommand', () => {
  afterEach(() => vi.unstubAllEnvs())

  it('prefers an explicit command', () => {
    expect(resolveCommand({ command: 'D:/bin/agentflow.exe' })).toBe('D:/bin/agentflow.exe')
  })

  it('falls back to AGENTFLOW_BIN when command is empty', () => {
    vi.stubEnv('AGENTFLOW_BIN', 'C:/env/agentflow.exe')
    expect(resolveCommand({ command: '' })).toBe('C:/env/agentflow.exe')
  })

  it('falls back to the PATH name when both are empty', () => {
    vi.stubEnv('AGENTFLOW_BIN', '')
    expect(resolveCommand({ command: '' })).toBe('agentflow')
  })
})

describe('buildMcpConfig', () => {
  it('omits AGENTFLOW_DB_PATH when dbPath is empty', () => {
    const mcp = buildMcpConfig(Config({}) as AgentflowConfig, 'agentflow')
    expect(mcp.transport).toBe('stdio')
    expect(mcp.serverName).toBe('agentflow')
    expect(mcp.args).toEqual(['stdio'])
    expect(mcp.env).toEqual({})
  })

  it('injects AGENTFLOW_DB_PATH when dbPath is set', () => {
    const mcp = buildMcpConfig(Config({ dbPath: 'C:/dsh/agentflow/af.db' }) as AgentflowConfig, 'agentflow')
    expect(mcp.env).toEqual({ AGENTFLOW_DB_PATH: 'C:/dsh/agentflow/af.db' })
  })
})
