# Codex adapter implementation plan

Approved scope: native Codex skill, independent installer, persistent MCP runtime, delegation guidance, isolated verification then local install. Reuse Go engine; no changes to DSH or Claude.

- [x] Add tests in tests/codex/test_install.py for dry-run, spaced paths, config preservation, replacement backup, failure rollback, duplicate install and DB preservation.
- [x] Add codex-agentflow/skills/agentflow/{SKILL.md,agents/openai.yaml,references/workflow.md,references/setup.md}. Host calls use actual available delegation API, real IDs, prepare/start ordering, matching runtime provider, no Claude hooks promises.
- [x] Add scripts/install-codex.py. Python3.11 stdlib; source-build engine or accept binary; isolated --codex-home and --skill-dir; CLI-only registration; no shell interpolation. Owned runtime versions are immutable directories; persistent DB separate. Default skill path ~/.agents/skills/agentflow.
- [x] Run tests: python -m unittest discover -s tests/codex -v. Build: go build -o dist/codex/agentflow.exe ./cmd/agentflow. Run relevant Go tests.
- [x] Run installer against temporary Codex home and skill directory using real Codex CLI. Exercise initialize/tools/list/flow_ping via diagnostic probe, with an isolated DB.
- [x] Review implementation, fix findings, install locally after tests. Do not bypass live-session tool gate; clearly distinguish config, protocol and model-visible MCP checks.
- [x] Add docs/codex-setup.md and README entry. No publish/tag/remote push without further request.
