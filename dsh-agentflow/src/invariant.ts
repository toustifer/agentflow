/**
 * Package-owned invariant companion for `@stifer/dsh-agentflow`.
 * @module @stifer/dsh-agentflow/invariant
 */

import type { Context } from '@deepseek-ai/cordis'
import type { InvariantInstaller } from '@deepseek-ai/dsh-invariants'

const PACKAGE_NAME = '@stifer/dsh-agentflow'

/** Cordis companion plugin name. */
export const name = 'agentflow-invariant'
/** Service required before the companion can reserve package ownership. */
export const inject = ['invariants']

/**
 * No runtime invariant: the plugin contributes through the tool registry
 * and the skill filesystem, with no independent snapshot to assert.
 */
const install: InvariantInstaller = () => {}

/**
 * Register this package's invariant companion.
 * @param ctx - Cordis context carrying the invariant service.
 * @returns the installed registration's disposer after setup succeeds.
 */
export const apply = (ctx: Context): Promise<() => void> =>
  Promise.resolve(ctx.invariants.register(PACKAGE_NAME, install))
