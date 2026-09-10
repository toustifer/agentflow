import type { Context } from '@deepseek-ai/cordis';
import { type AgentflowConfig } from './config.js';
export type { AgentflowConfig } from './config.js';
export { Config } from './config.js';
/** Cordis plugin name used by loader diagnostics. */
export declare const name = "agentflow";
/** This plugin provides no services itself; mcp-client injects `tools`. */
export declare const inject: never[];
/**
 * Mount one agentflow MCP server and best-effort sync the bundled skill.
 * Skill sync failures are warnings, never fatal: the MCP bridge is the
 * critical capability.
 */
export declare function apply(ctx: Context, config: AgentflowConfig): void;
