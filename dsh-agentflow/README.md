# @stifer/dsh-agentflow

[![dshfind](https://dshfind.com/api/badge/toustifer/agentflow)](https://dshfind.com/en/plugins/toustifer/agentflow?ref=badge)
[![npm version](https://img.shields.io/npm/v/@stifer/dsh-agentflow.svg)](https://www.npmjs.com/package/@stifer/dsh-agentflow)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> Autonomous Multi-Agent Engineering Orchestrator with Live-Spec 4D Canvas & Team Hub Federation for DeepSeek Harness.

Enables full Agentflow lifecycle orchestration within DeepSeek Harness (DSH). Installing this plugin:
- Automatically mounts the core [agentflow](https://github.com/toustifer/agentflow) MCP server (exposing 61+ atomic scheduling tools: `mcp__agentflow__*`).
- Automatically synchronizes the latest `agentflow` skill (with version guarding and downgrade protection) into the DSH skill catalog.
- Integrates seamlessly with Live-Spec 4D interactive topological canvas for time-series simulation and state management.

---

## ⚡ 30-Second Quick Start

### 1. Install Plugin
```bash
dsh plugin --profile web add @stifer/dsh-agentflow
```

### 2. Configure Activation
Append the plugin configuration in `<dshHome>/profiles/web/cordis.patch.yml`:

```yaml
- insert:
    - id: agentflow
      name: '@stifer/dsh-agentflow'
```

### 3. Restart & Verify
Restart DSH (or open a new session in Web UI) to experience:
- Type `/agentflow` to invoke the interactive orchestration guide.
- Call `mcp__agentflow__flow_ping` to test connectivity (returns `{ "ok": true }`).
- Call `mcp__agentflow__project_init(workdir="...")` to bind your project repository.

---

## 🏛️ Dual-Engine Architecture

Agentflow is engineered around a decoupled dual-engine architecture to ensure high reliability and sandbox security:

```
┌─────────────────────────────────────────────────────────────┐
│                    DeepSeek Harness (DSH)                   │
│   ┌───────────────────────┐       ┌─────────────────────┐   │
│   │  @stifer/dsh-agentflow     │ ────> │ Live-Spec 4D Canvas │   │
│   └───────────────────────┘       └─────────────────────┘   │
└───────────────┬─────────────────────────────────────────────┘
                │ stdio (Auto-adaptive Content-Length / NDJSON)
┌───────────────▼─────────────────────────────────────────────┐
│ 1. Go State Machine Core                                    │
│    - Single Source of Truth (SSOT): SQLite persistence      │
│    - Physical Sandbox Isolation: Task-scoped Git Worktrees  │
│    - Delivery Gate Transitions: ready → exec → review → pass│
│    - 61+ atomic MCP tools (mcp__agentflow__*)               │
└───────────────▲─────────────────────────────────────────────┘
                │ RPC / Local Socket
┌───────────────▼─────────────────────────────────────────────┐
│ 2. Python Behavior Tree Engine (bt_service)                 │
│    - Zero third-party pip dependencies (Standard Library)   │
│    - Autonomous BT loops: Leader / Worker / Reviewer        │
│    - Shared Blackboard & persistent reflection Diaries      │
└─────────────────────────────────────────────────────────────┘
```

1. **Go State Machine Core**:
   - Acts as the authoritative **Single Source of Truth**, managing project metadata, DAG dependencies, and task transitions backed by SQLite.
   - **Git Worktree Physical Isolation**: Every task executes in its own dedicated Git worktree sandbox, eliminating multi-agent code overwriting and workspace contamination.
   - **Adaptive Dual-Mode Framing**: Automatically detects and handles both Content-Length headers and newline-delimited JSON (NDJSON) framing over stdio, matching DSH's official MCP client.
2. **Python Behavior Tree Engine (bt_service)**:
   - Built purely on Python standard library with zero external dependencies.
   - Powers autonomous behavior trees for Leader (dispatch & monitor), Worker (implementation & git commit), and Reviewer (diff inspection & gate decision).

---

## Requirements

- DeepSeek Harness (DSH) with any initialized profile.
- **agentflow binary v0.2.8+** (recommended: **v0.2.9**):
  Starting from v0.2.8, agentflow natively supports auto-adaptive dual-mode stdio framing. Earlier builds may experience protocol timeouts due to framing mismatches.
  Verify local installation:
  ```bash
  agentflow version    # Output: agentflow v0.2.9 (commit ...)
  ```

---

## Configuration

All fields are optional; defaults are shown below:

| Field | Default | Description |
|---|---|---|
| `serverName` | `agentflow` | Tool-name namespace prefix (`mcp__<serverName>__*`) |
| `command` | `''` → `$AGENTFLOW_BIN` → `agentflow` | Path to agentflow executable |
| `args` | `['stdio']` | Arguments passed to the binary |
| `dbPath` | `''` | Custom SQLite database path (`AGENTFLOW_DB_PATH`) |
| `syncSkill` | `true` | Auto-sync bundled skill with version guard |
| `toolCallTimeoutMs` | `60000` | Per-tool-call execution timeout (ms) |
| `failOnStartupError` | `false` | Fail plugin activation if initial connect fails |
| `reconnect` | mcp-client defaults | Auto-reconnect retry policy |

Example configuration with explicit path:

```yaml
- insert:
    - id: agentflow
      name: '@stifer/dsh-agentflow'
      config:
        command: 'D:\myprogram\agentflow\bin\agentflow.exe'
        dbPath: 'C:\Users\me\.dsh\agentflow\agentflow.db'
```

---

## Verification

1. **Skill Ready**: Check that `agentflow` appears in the DSH skill catalog.
2. **MCP Tools**: Verify `mcp__agentflow__flow_ping` and other tools appear in the tool list.
3. **Smoke Call**: Execute `mcp__agentflow__flow_ping` and verify `{ "ok": true }`.
