import z from '@deepseek-ai/schemastery';
import type { ReconnectConfig } from '@deepseek-ai/dsh-mcp-client';
/** Default namespace for agentflow's model-facing tool names. */
export declare const DEFAULT_SERVER_NAME = "agentflow";
/** Default per-tool-call timeout in milliseconds. */
export declare const DEFAULT_TOOL_CALL_TIMEOUT_MS = 60000;
/** Valid `serverName`, matching dsh-mcp-client's public tool-name budget. */
export declare const SERVER_NAME_PATTERN: RegExp;
/** User-facing configuration for one agentflow MCP server. */
export interface AgentflowConfig {
    serverName: string;
    command: string;
    args: string[];
    dbPath: string;
    syncSkill: boolean;
    toolCallTimeoutMs: number;
    failOnStartupError: boolean;
    reconnect?: ReconnectConfig;
}
/** Schemastery schema; defaults are applied by the cordis loader. */
export declare const Config: z<Schemastery.ObjectS<{
    serverName: z<string, string>;
    command: z<string, string>;
    args: z<string[], string[]>;
    dbPath: z<string, string>;
    syncSkill: z<boolean, boolean>;
    toolCallTimeoutMs: z<number, number>;
    failOnStartupError: z<boolean, boolean>;
    reconnect: z<Schemastery.ObjectS<{
        enabled: z<boolean, boolean>;
        initialDelayMs: z<number, number>;
        maxDelayMs: z<number, number>;
        maxAttempts: z<number, number>;
    }>, Schemastery.ObjectT<{
        enabled: z<boolean, boolean>;
        initialDelayMs: z<number, number>;
        maxDelayMs: z<number, number>;
        maxAttempts: z<number, number>;
    }>>;
}>, Schemastery.ObjectT<{
    serverName: z<string, string>;
    command: z<string, string>;
    args: z<string[], string[]>;
    dbPath: z<string, string>;
    syncSkill: z<boolean, boolean>;
    toolCallTimeoutMs: z<number, number>;
    failOnStartupError: z<boolean, boolean>;
    reconnect: z<Schemastery.ObjectS<{
        enabled: z<boolean, boolean>;
        initialDelayMs: z<number, number>;
        maxDelayMs: z<number, number>;
        maxAttempts: z<number, number>;
    }>, Schemastery.ObjectT<{
        enabled: z<boolean, boolean>;
        initialDelayMs: z<number, number>;
        maxDelayMs: z<number, number>;
        maxAttempts: z<number, number>;
    }>>;
}>>;
/**
 * Resolve the agentflow binary path: an explicit `command` wins, then
 * `$AGENTFLOW_BIN`, then the bare `agentflow` name resolved from PATH.
 */
export declare function resolveCommand(config: {
    command: string;
}): string;
/** Resolve the agents home: `$DSH_AGENTS_HOME`, falling back to `~/.agents` (same default as skill-filesystem). */
export declare function agentsHome(): string;
/** Build the config handed to dsh-mcp-client for one agentflow stdio server. */
export declare function buildMcpConfig(config: AgentflowConfig, command: string): {
    reconnect?: ReconnectConfig | undefined;
    transport: "stdio";
    serverName: string;
    command: string;
    args: string[];
    env: Record<string, string>;
    cwd: string;
    toolCallTimeoutMs: number;
    failOnStartupError: boolean;
};
