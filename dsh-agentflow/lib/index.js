import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { apply as mcpClientApply, Config as McpClientConfig, inject as mcpClientInject, name as mcpClientName, } from '@deepseek-ai/dsh-mcp-client';
import { agentsHome, buildMcpConfig, resolveCommand } from './config.js';
import { syncSkill } from './skill.js';
export { Config } from './config.js';
/** Cordis plugin name used by loader diagnostics. */
export const name = 'agentflow';
/** This plugin provides no services itself; mcp-client injects `tools`. */
export const inject = [];
/** Bundled skill bundle, copied by scripts/copy-skill.mjs into lib/skills/agentflow. */
const BUNDLED_SKILL_DIR = join(dirname(fileURLToPath(import.meta.url)), 'skills', 'agentflow');
const mcpClientPlugin = {
    name: mcpClientName,
    inject: mcpClientInject,
    Config: McpClientConfig,
    apply: mcpClientApply,
};
/**
 * Mount one agentflow MCP server and best-effort sync the bundled skill.
 * Skill sync failures are warnings, never fatal: the MCP bridge is the
 * critical capability.
 */
export function apply(ctx, config) {
    if (config.syncSkill) {
        const target = join(agentsHome(), 'agentflow');
        syncSkill(BUNDLED_SKILL_DIR, target).catch((error) => {
            console.warn(`dsh-agentflow: skill sync to ${target} failed (continuing): ${error instanceof Error ? error.message : String(error)}`);
        });
    }
    const command = resolveCommand(config);
    ctx.plugin(mcpClientPlugin, buildMcpConfig(config, command));
}
