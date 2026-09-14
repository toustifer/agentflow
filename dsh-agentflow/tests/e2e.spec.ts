import { spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

function findBinary(): string | null {
  const fromEnv = process.env.AGENTFLOW_BIN
  if (fromEnv !== undefined && fromEnv !== '') return fromEnv
  const repoBin = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'bin', 'agentflow.exe')
  if (existsSync(repoBin)) return repoBin
  return null
}

const binary = findBinary()

function frame(body: string): Buffer {
  return Buffer.from(`Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`)
}

function mcpCall(binaryPath: string, requests: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(binaryPath, ['stdio'], {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: { ...process.env, AGENTFLOW_DB_PATH: join(tmpdir(), 'dsh-af-e2e.db') },
    })
    let out = ''
    let err = ''
    child.stdout.on('data', (d: Buffer) => { out += d.toString() })
    child.stderr.on('data', (d: Buffer) => { err += d.toString() })
    child.on('error', reject)
    child.on('close', (code) => {
      if (code !== 0) reject(new Error(`agentflow exited ${code}: ${err}`))
      else resolve(out)
    })
    child.stdin.write(Buffer.concat(requests.map(frame)))
    child.stdin.end()
  })
}

/** Parse the JSON body out of a Content-Length framed output buffer. */
function parseFrameBody(out: string): any {
  const headerEnd = out.indexOf('\r\n\r\n')
  if (headerEnd < 0) throw new Error(`no frame terminator in output: ${out}`)
  return JSON.parse(out.slice(headerEnd + 4))
}

describe.skipIf(binary === null)('agentflow binary e2e', () => {
  it('answers initialize over standard MCP framing', async () => {
    const out = await mcpCall(binary!, [JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: {} })])
    expect(out).toContain('"serverInfo"')
    expect(out).toContain('"agentflow"')
  })

  it('flow_ping returns ok:true', async () => {
    const out = await mcpCall(binary!, [
      JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: 'flow_ping', arguments: {} } }),
    ])
    // flow_ping's payload arrives as the MCP tools/call result, its JSON body
    // re-escaped inside content[0].text — parse both layers before asserting.
    const response = parseFrameBody(out)
    const text = (response.result.content as { text: string }[])[0].text
    const payload = JSON.parse(text)
    expect(payload.ok).toBe(true)
  })
})
