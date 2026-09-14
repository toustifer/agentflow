import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { compareVersions, readVersion, syncSkill } from '../src/skill.js'

let tmp: string
beforeEach(async () => { tmp = await mkdtemp(join(tmpdir(), 'dsh-af-skill-')) })
afterEach(async () => { await rm(tmp, { recursive: true, force: true }) })

describe('compareVersions', () => {
  it('compares numeric segments', () => {
    expect(compareVersions('v0.2.6', 'v0.2.5')).toBe(1)
    expect(compareVersions('0.2.5', '0.2.6')).toBe(-1)
    expect(compareVersions('v0.2.6', '0.2.6')).toBe(0)
  })

  it('treats unparseable versions as older', () => {
    expect(compareVersions('garbage', 'v0.2.6')).toBe(-1)
    expect(compareVersions('v0.2.6', 'garbage')).toBe(1)
  })
})

describe('syncSkill', () => {
  async function makeSkill(dir: string, version: string): Promise<void> {
    await mkdir(dir, { recursive: true })
    await writeFile(join(dir, 'VERSION'), version)
    await writeFile(join(dir, 'SKILL.md'), '# test skill\n')
  }

  it('copies when the target is absent', async () => {
    const source = join(tmp, 'source')
    await makeSkill(source, 'v0.2.6')
    const target = join(tmp, 'target')
    expect(await syncSkill(source, target)).toBe('copied')
    expect(await readFile(join(target, 'VERSION'), 'utf8')).toBe('v0.2.6')
  })

  it('copies when the target is older', async () => {
    const source = join(tmp, 'source')
    await makeSkill(source, 'v0.2.7')
    const target = join(tmp, 'target')
    await makeSkill(target, 'v0.2.6')
    expect(await syncSkill(source, target)).toBe('copied')
    expect(await readVersion(target)).toBe('v0.2.7')
  })

  it('skips when the target is equal or newer', async () => {
    const source = join(tmp, 'source')
    await makeSkill(source, 'v0.2.6')
    const equal = join(tmp, 'equal')
    await makeSkill(equal, 'v0.2.6')
    expect(await syncSkill(source, equal)).toBe('skipped')

    const newer = join(tmp, 'newer')
    await makeSkill(newer, 'v0.2.8')
    expect(await syncSkill(source, newer)).toBe('skipped')
  })

  it('copies when versions cannot be compared (missing VERSION)', async () => {
    const source = join(tmp, 'source')
    await makeSkill(source, 'v0.2.6')
    const target = join(tmp, 'target')
    await mkdir(target, { recursive: true })
    await writeFile(join(target, 'SKILL.md'), '# mine\n')
    expect(await syncSkill(source, target)).toBe('copied')
  })
})
