import { cp, mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

// dsh-agentflow/ 目录（本脚本位于 dsh-agentflow/scripts/ 下）
const root = dirname(dirname(fileURLToPath(import.meta.url)))
const source = join(root, '..', 'skills', 'agentflow')

if (!source) throw new Error('missing source skill bundle')

for (const dest of [join(root, 'src', 'skills', 'agentflow'), join(root, 'lib', 'skills', 'agentflow')]) {
  await mkdir(dest, { recursive: true })
  await cp(source, dest, { recursive: true, force: true })
}

console.log('dsh-agentflow: copied skills/agentflow -> src/skills/agentflow and lib/skills/agentflow')
