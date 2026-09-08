# Agent Hub v0.2.2 Release Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Execute this plan task-by-task with review checkpoints.

**Goal:** Promote the current agent-hub federation, requirements, MCP, and frontend changes from the local feature branch into a tested `v0.2.2` release and deploy it to `https://hub.stifer.xyz`.

**Architecture:** Keep the existing release contract: `VERSION`/annotated tag provide the server version, `scripts/build-release.sh` creates the Linux binary plus Vite static files and migrations, and `scripts/deploy-release.sh` uploads the tarball to `storyhost`, applies migrations, restarts systemd, and runs public smoke checks. Stage only intentional source, migration, documentation, and generated static assets; leave local secrets and patch helpers out.

**Tech Stack:** Go 1.25, Gin, PostgreSQL migrations, MCP Go SDK, Vue 3/Vite, npm, GitHub Actions, SSH/systemd.

---

### Task 1: Audit and test the release scope

**Files:**
- Read: current tracked and untracked changes in `D:/myprogram/agent-hub`
- Test: Go packages under `internal/...` and the frontend package

- [ ] Run `git diff --check` and inspect all changed source, migrations, docs, and static files.
- [ ] Run `go test ./internal/... -count=1` with a workspace Go cache.
- [ ] Run `npm ci --no-audit --no-fund` and `npm run build` in `frontend`.
- [ ] Exclude `.mycompany/config.json`, patch scripts, local plans, and unrelated credentials from the release commit unless a source review proves they are required.

### Task 2: Prepare the v0.2.2 release metadata

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`
- Modify: `docs/RELEASE.md` only where examples describe the current release

- [ ] Change `VERSION` from `0.2.0` to `0.2.2`, matching production's observed `v0.2.1` and the next unreleased feature version.
- [ ] Add a `0.2.2` changelog section covering requirements API/MCP, federation UI, login compatibility, and deployment assets.
- [ ] Re-run formatting checks and confirm no secret or machine-specific config is staged.

### Task 3: Commit the reviewed source and generated static assets

**Files:**
- Stage the reviewed Go, frontend, migration, docs, and generated `static/` files.

- [ ] Create one release commit: `release: v0.2.2 requirements and federation updates`.
- [ ] Verify the commit contains no `.env`, token, `.mycompany/config.json`, patch helper, or build cache.
- [ ] Request a code review against the previous branch commit before tagging.

### Task 4: Build and publish artifacts

**Files:**
- Create: `dist/release/v0.2.2/`
- Create: `dist/release/agent-hub-v0.2.2-linux-amd64.tar.gz`

- [ ] Run `scripts/build-release.sh v0.2.2` and verify `RELEASE.txt` contains the tagged commit and `v0.2.2`.
- [ ] Create annotated tag `v0.2.2`, then push the branch and tag to `origin`.
- [ ] Confirm GitHub Actions creates the matching GitHub Release artifact.

### Task 5: Deploy and smoke-test production

**Files:**
- Use: `scripts/deploy-release.sh v0.2.2 storyhost`

- [ ] Deploy the tarball through the existing SSH/systemd script; it must apply migrations, restart `hub-server.service`, and report `DEPLOY_OK v0.2.2`.
- [ ] Verify `https://hub.stifer.xyz/health` and `/version` report `v0.2.2` and the new commit.
- [ ] Verify the MCP endpoint initializes and exposes the requirements tools, then report any remaining acceptance gap.
