import { homedir } from 'node:os';
import { join } from 'node:path';
import z from '@deepseek-ai/schemastery';
/** Default namespace for agentflow's model-facing tool names. */
export const DEFAULT_SERVER_NAME = 'agentflow';
/** Default per-tool-call timeout in milliseconds. */
export const DEFAULT_TOOL_CALL_TIMEOUT_MS = 60_000;
/** Valid `serverName`, matching dsh-mcp-client's public tool-name budget. */
export const SERVER_NAME_PATTERN = /^[A-Za-z0-9_-]{1,32}$/;
const Reconnect = z.object({
    enabled: z.boolean(),
    initialDelayMs: z.number(),
    maxDelayMs: z.number(),
    maxAttempts: z.number(),
});
/** Schemastery schema; defaults are applied by the cordis loader. */
export const Config = z.object({
    serverName: z.string().pattern(SERVER_NAME_PATTERN).default(DEFAULT_SERVER_NAME),
    command: z.string().default(''),
    args: z.array(String).default(['stdio']),
    dbPath: z.string().default(''),
    syncSkill: z.boolean().default(true),
    toolCallTimeoutMs: z.number().default(DEFAULT_TOOL_CALL_TIMEOUT_MS),
    failOnStartupError: z.boolean().default(false),
    reconnect: Reconnect,
});
/**
 * Resolve the agentflow binary path: an explicit `command` wins, then
 * `$AGENTFLOW_BIN`, then the bare `agentflow` name resolved from PATH.
 */
export function resolveCommand(config) {
    if (config.command.trim() !== '')
        return config.command;
    const fromEnv = process.env.AGENTFLOW_BIN;
    if (fromEnv !== undefined && fromEnv.trim() !== '')
        return fromEnv;
    return 'agentflow';
}
/** Resolve the agents home: `$DSH_AGENTS_HOME`, falling back to `~/.agents` (same default as skill-filesystem). */
export function agentsHome() {
    return process.env.DSH_AGENTS_HOME ?? join(homedir(), '.agents');
}
/** Build the config handed to dsh-mcp-client for one agentflow stdio server. */
export function buildMcpConfig(config, command) {
    const env = config.dbPath === '' ? {} : { AGENTFLOW_DB_PATH: config.dbPath };
    return {
        transport: 'stdio',
        serverName: config.serverName,
        command,
        args: config.args,
        env,
        // mcp-client's StdioConfig requires cwd; an empty string keeps the
        // child process working directory at the DSH process's own cwd.
        cwd: '',
        toolCallTimeoutMs: config.toolCallTimeoutMs,
        failOnStartupError: config.failOnStartupError,
        ...(config.reconnect !== undefined ? { reconnect: config.reconnect } : {}),
    };
}
