# Codex setup and troubleshooting

This is a Codex-native skill + local stdio MCP adapter. It reuses the engine, not a DSH child CLI. Installation requires a trusted agentflow source checkout, Python 3.11+, Codex CLI, Git, and Go when building the binary. The engine's Python sidecar needs no third-party runtime dependencies.

From the agentflow checkout:

```text
python scripts/install-codex.py
```

Or pass `--binary <trusted-engine-binary>` to reuse a source-built binary. On Windows, use the native `codex.exe` path with `--codex-command` if it cannot be found; do not pass a .cmd/.ps1 shim. Read `--help` before customizing destinations.

Defaults:

- Skill: `~/.agents/skills/agentflow`.
- MCP config: `$CODEX_HOME/config.toml`, normally `~/.codex/config.toml`.
- Persistent database: `$CODEX_HOME/agentflow/data/agentflow.db`.
- Versioned runtime: `$CODEX_HOME/agentflow/runtime/<installation-id>`.
- Config backups: `$CODEX_HOME/agentflow/backups/<installation-id>`.
- Skill backups: `.agentflow-backup-<installation-id>` beside the skill directory, so updates also work when the skill and Codex home are on different disks.

The installer sets `AGENTFLOW_DB_PATH`, `AGENTFLOW_BT_DIR`, and `AGENTFLOW_PYTHON`. It does not import or modify existing DSH/Claude databases. If an unmanaged skill or agentflow MCP entry already exists, inspect its paths and database first; do not casually replace it. An explicit replacement retains a backup.

Use `--codex-home <temporary-dir> --skill-dir <temporary-other-dir>` for isolated tests. `--dry-run --binary <path>` only describes the proposed changes.

After installation, restart/reload Codex if necessary. Run `codex mcp get agentflow --json` to check configuration, then in a new Codex session invoke `$agentflow setup`. Discover and call the actual `flow_ping` tool. Only this last step confirms model-visible MCP access. The current conversation may have an unchanged tool catalog after installation.

If Python/Go is missing, fix the prerequisite; do not disable sandbox/approval controls. If engine startup fails, inspect its logs and the configured runtime paths without printing credentials from the broader Codex configuration. An absent subagent API prevents execution, not read-only inspection.

Updating uses the same installer and a new runtime directory. Existing database files remain untouched and previous runtimes/backups remain available. Do not delete the state directory to repair installation. For rollback, restore the saved agentflow MCP entry and skill after closing affected sessions; preserve the persistent database and unrelated Codex settings.

On registration failure, the installer restores the prior skill. It restores configuration only if unrelated settings still match the original snapshot; conflicting or invalid configuration is retained with a warning for manual inspection. Do not overwrite simultaneous settings changes with a whole-file backup.

This package does not install a persistent hook, a custom slash command, or a model provider. It uses the user's configured Codex model and authorization.
