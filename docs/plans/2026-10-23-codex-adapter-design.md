# Codex native adapter design (2026-10-23)

Status: approved by user on 2026-10-23; implementing native Codex adapter and local installation.

## Evidence

- Local clean repository matches public master HEAD 0697b530f94e8a653c0d0a1de67c6d28ac555e0f.
- Existing scripts/install.ps1 and install.sh can run codex mcp add, but install Claude-oriented skills under ~/.claude and do not provide a complete Codex-native workflow.
- Shared skills refer to Claude Agent(), claude_code provider, UserPromptSubmit hooks and /mcp semantics. These cannot simply be claimed to work in Codex.
- Go server supports stdio framing, AGENTFLOW_DB_PATH, real launch tickets and task/worktree transitions; keep this engine as the single source of truth.
- The default database falls back to TEMP: adapter installation must explicitly configure a durable DB path.

## Recommended scope

1. Add a Codex-specific skill package, preserving DSH and Claude behavior. Use documented $agentflow invocation; do not claim registration of a custom /agentflow slash command. Natural-language requests can also select the skill.
2. Add a cross-platform local installer with dry-run and explicit paths. Install the skill to ~/.agents/skills/agentflow and runtime to a separate stable user directory. Configure MCP via the installed Codex CLI, preserving unrelated configuration and backing up replaced owned files. Never overwrite existing DSH/Claude state or migrate a database implicitly.
3. Set an explicit persistent AGENTFLOW_DB_PATH and BT runtime location. Source-build or accept a supplied engine binary; no silent execution of an unverified downloaded release.
4. Adapt leader/worker/reviewer instructions: prepare launch ticket, spawn a real host subagent, record its actual ID and runtime.provider=codex, then start. Use the returned task worktree; do not invent spawned IDs or mark preparation as execution. If the host lacks delegation, stop execution with a clear capability message instead of silently having the leader implement it.
5. Keep canonical task states, docs, diary, review and worktree requirements. Existing .claude project metadata paths remain engine compatibility storage, not evidence of requiring the Claude application.
6. No Claude hook emulation or unsupported sticky-mode promise. No cross-client DSH/pi bridge, cloud service, model credentials, or public release in this scope.

## Alternatives

- MCP registration only: minimal, but does not fix skill discovery and host-specific dispatch instructions.
- Codex-native skill + MCP (recommended): reuse engine while explicitly adapting workflow and installation.
- Shell bridge to DSH: adds a second model/runtime and is a separate integration rather than a native port.

## Validation

- Installer tests: paths with spaces, existing configuration preserved, duplicate installs, failed registration, durable DB location and no unexpected deletions.
- Skill validation: valid metadata, no dependence on Claude hooks/Agent() calls, accurate invocation and session-MCP gate.
- Build Go engine and run relevant tests.
- Isolated temporary Codex configuration: actual MCP initialize/tools/list/flow_ping. Protocol-level diagnostics are setup validation only, not evidence of a production worker lifecycle.
- If available, run a fresh Codex session that actually invokes flow_ping; report separately from codex mcp list configuration success.
- Install locally only within confirmed scope; new chat/restart may be required for tool discovery. Preserve production task databases and existing host settings.

## Official references checked

- https://learn.chatgpt.com/docs/build-skills — $skill invocation and ~/.agents/skills discovery.
- https://learn.chatgpt.com/docs/extend/mcp?surface=cli — stdio registration and configuration.
