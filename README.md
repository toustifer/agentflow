# agentflow

Lightweight task state machine engine with MCP stdio interface. Powers the [agent-company](https://github.com/stifer/agent-company) multi-agent orchestration skill for Claude Code.

## What it does

- **Task lifecycle state machine**: `assigned → executing → review_pending → done | rework_needed`
- **Markovian oracle**: `available_transitions` tells each role what they can do — no need to memorize state machines
- **Role-based transition enforcement**: server-side validation that `actor_role` matches allowed transitions
- **MCP stdio protocol**: JSON-RPC 2.0 over stdin/stdout with Content-Length framing
- **SQLite persistence**: in-memory or file-backed storage

## Quick start

```bash
go build -o agentflow ./cmd/agentflow/
./agentflow stdio   # MCP mode — Claude Code connects here
./agentflow          # HTTP mode — health + API on :9600
```

## MCP tools

| Tool | Description |
|------|-------------|
| `namespace_create` | Create an isolated namespace |
| `task_create` | Create a task (returns `available_transitions`) |
| `task_get` | Get task with `available_transitions` |
| `task_list` | List tasks in a namespace |
| `task_transition` | Apply a transition with `actor_role` validation |
| `task_history` | View task event history |
| `flow_ping` | Health check |

## Roles & transitions

| Role | Allowed transitions |
|------|-------------------|
| `leader` | `start`, `resume`, `reassign`, `cancel` |
| `worker` | `submit` |
| `reviewer` | `pass`, `rework` |

Pass `actor_role` in every `task_transition` call. Invalid roles or transitions are rejected server-side.

## Tests

```bash
go test ./pkg/...                          # Unit tests
go run ./smoke/mcp_comm_check.go           # Full MCP communication test
```
