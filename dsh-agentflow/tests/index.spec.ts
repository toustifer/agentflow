import { describe, expect, it, vi } from 'vitest'
import { apply, Config, inject, name } from '../src/index.js'
import type { AgentflowConfig } from '../src/config.js'

describe('plugin exports', () => {
  it('exposes the cordis plugin contract', () => {
    expect(name).toBe('agentflow')
    expect(inject).toEqual([])
    expect(typeof Config).toBe('function')
    expect(typeof apply).toBe('function')
  })
})

describe('apply', () => {
  it('mounts mcp-client once with the resolved stdio config', () => {
    const ctx = { plugin: vi.fn() } as unknown as Parameters<typeof apply>[0]
    const config: AgentflowConfig = {
      serverName: 'agentflow',
      command: 'D:/bin/agentflow.exe',
      args: ['stdio'],
      dbPath: '',
      syncSkill: false,
      toolCallTimeoutMs: 60_000,
      failOnStartupError: false,
    }
    apply(ctx, config)
    expect(ctx.plugin).toHaveBeenCalledTimes(1)
    const [plugin, mcpConfig] = vi.mocked(ctx.plugin).mock.calls[0] as [
      { name: string },
      { transport: string; serverName: string; command: string },
    ]
    expect(plugin.name).toBe('mcp-client')
    expect(mcpConfig.transport).toBe('stdio')
    expect(mcpConfig.serverName).toBe('agentflow')
    expect(mcpConfig.command).toBe('D:/bin/agentflow.exe')
  })

  it('never touches the real agents home when syncSkill is false', async () => {
    const ctx = { plugin: vi.fn() } as unknown as Parameters<typeof apply>[0]
    apply(ctx, {
      serverName: 'agentflow',
      command: 'agentflow',
      args: ['stdio'],
      dbPath: '',
      syncSkill: false,
      toolCallTimeoutMs: 60_000,
      failOnStartupError: false,
    })
    expect(ctx.plugin).toHaveBeenCalledTimes(1)
  })
})
