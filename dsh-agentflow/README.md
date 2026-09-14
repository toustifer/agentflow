# @toustifer/dsh-agentflow

agentflow for DeepSeek Harness. Installing this plugin mounts the
[agentflow](https://github.com/toustifer/agentflow) MCP server — exposing the
`mcp__agentflow__*` tools (61 tools: project_init, dag_create, task_create,
leader_tick, flow_ping, ...) — and syncs the `agentflow` skill into your
DSH skill root.

## Requirements

- DSH (DeepSeek Harness) with the `web` (or any) profile already initialized.
- **agentflow binary v0.2.8+**: Starting from v0.2.8, agentflow natively supports auto-adaptive dual-mode stdio framing (Content-Length and NDJSON / newline-delimited JSON). Because DSH relies on the TypeScript MCP SDK which defaults to NDJSON framing over stdio, builds from v0.2.7 or earlier will hang or time out due to framing mismatch. Ensure you upgrade to v0.2.8+. Verify:

  ```bash
  agentflow version    # must print a version (v0.2.8+), not a server banner
  ```

## Install

```bash
dsh plugin --profile web add @toustifer/dsh-agentflow
```

Then add one row to `<dshHome>/profiles/web/cordis.patch.yml`:

```yaml
- insert:
    - id: agentflow
      name: '@toustifer/dsh-agentflow'
```

Restart DSH (or open a new session). The skill catalog gains `agentflow`, and
the tool list gains `mcp__agentflow__*`.

## Configuration

All fields are optional; defaults are shown.

| Field | Default | Meaning |
|---|---|---|
| `serverName` | `agentflow` | Tool-name namespace: `mcp__agentflow__*` |
| `command` | `''` → `$AGENTFLOW_BIN` → `agentflow` | Binary path |
| `args` | `['stdio']` | Passed to the binary |
| `dbPath` | `''` | Sets `AGENTFLOW_DB_PATH` when non-empty |
| `syncSkill` | `true` | Sync the bundled skill (version-guarded) |
| `toolCallTimeoutMs` | `60000` | Per-tool-call timeout |
| `failOnStartupError` | `false` | Fail activation on initial connect error |
| `reconnect` | mcp-client defaults | Reconnect policy |

Example with overrides:

```yaml
- insert:
    - id: agentflow
      name: '@toustifer/dsh-agentflow'
      config:
        command: 'D:\myprogram\agentflow\bin\agentflow.exe'
        dbPath: 'C:\Users\me\.dsh\agentflow\agentflow.db'
```

## How it works

`apply()` resolves the binary path, best-effort syncs the bundled skill to
`$DSH_AGENTS_HOME` (default `~/.agents`), then mounts
`@deepseek-ai/dsh-mcp-client` with `transport: 'stdio'` and
`args: ['stdio']`.

## Verify

- `skill` loads `agentflow` (`<skill_content name="agentflow">`).
- Tool list contains `mcp__agentflow__flow_ping`.
- Calling `mcp__agentflow__flow_ping` returns `ok: true`.
