# agentflow on GitHub — Committer 归属 & 分支保护

> 面向 owner 的操作记录：如何把已是本地历史的主提交改回正确的 GitHub 用户，以及如何为分支加保护。
> 适用宿主：`git` + [`gh` CLI](https://cli.github.com/)。验证于 `agentflow` 仓库（owner：`toustifer`）。

## 一、改提交归属（author/committer）

场景：本地用的 `user.name/email` 与仓库 owner 不一致，导致提交的 author/committer 显示成别的用户（例如 `stifer`，而应归属 `toustifer`）。

### 1. 先看当前归属

```bash
git show -s --format="author:  %an <%ae>%ncommitter: %cn <%ce>%n%H" HEAD
```

### 2. 改写最近一条提交的归属

用环境变量在**单次命令内**改写，不污染任意持久 git 配置：

```bash
# 只改 author（保留原 committer）
git commit --amend --no-edit --author="toustifer <toustifer@users.noreply.github.com>"

# 两者都改成目标用户（author + committer 一致）
GIT_COMMITTER_NAME="toustifer" \
GIT_COMMITTER_EMAIL="toustifer@users.noreply.github.com" \
git commit --amend --no-edit --author="toustifer <toustifer@users.noreply.github.com>"
```

> 注意：`--amend` 默认只改 committer（`--reset-author` 保留原 author）。要 author 也变，**必须显式 `--author=`**。

### 3. 确认后推送

```bash
git show -s --format="author:  %an <%ae>%ncommitter: %cn <%ce>%n%H" HEAD
# 远端仍是旧哈希；改写后哈希变化，需要 force
git push --force-with-lease origin <branch>
```

`--force-with-lease` 比 `--force` 安全：它要求远端仍等于你本地的 `remote-tracking` 引用，避免覆盖别人的推送。

### 关于邮箱

- 不想暴露真实邮箱 → 用 GitHub noreply：`<username>@users.noreply.github.com`（如 `toustifer@users.noreply.github.com`）。
- 这是 GitHub 的标准匿名格式，能关联到对应账号。

## 二、给分支加保护

分支保护**不是 git 功能**，需在 GitHub 侧配置。本仓库 owner 已用 `gh` 认证（`gh auth status` 应显示目标账号），可直接调 API。

### 1. 确认账号与仓库

```bash
gh auth status          # 确认登录的是 owner（toustifer）
gh repo view toustifer/agentflow --json defaultBranchRef,isPrivate  # 默认分支/可见性
```

### 2. 为单个分支加保护（推荐组合：线性历史 + PR 审查 + 禁 force push）

```bash
gh api -X PUT \
  repos/toustifer/agentflow/branches/master/protection \
  --input - <<'JSON'
{
  "required_status_checks": null,
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}
JSON
```

把 `master` 换成任意分支名即可为其它分支加。同一组合可套用到多个分支。

### 3. 校验是否生效

```bash
gh api repos/toustifer/agentflow/branches/master/protection \
  | python3 -c "import sys,json;d=json.load(sys.stdin);print('force_push:',d.get('allow_force_pushes',{}).get('enabled'));print('linear_history:',d.get('required_linear_history',{}).get('enabled'));print('review_req:',d.get('required_pull_request_reviews',{}).get('required_approving_review_count'));print('enforce_admins:',d.get('enforce_admins',{}).get('enabled'))"
```

### 规则含义

| 规则 | 开关 | 影响 |
|------|------|------|
| `required_linear_history` | true | 禁止 merge commit 混入，历史必须线性 |
| `allow_force_pushes` | false | 禁止 force push，保护已推送历史 |
| `required_pull_request_reviews` | 1 | 合并须 ≥1 人 approval |
| `enforce_admins` | false | 管理员是否也被强制（先 false，便于解锁调整） |

### 注意：加保护后

- 加保护**之后**，对受保护分支的**任何改动都必须走 PR**（≥1 review、线性历史），不能直接 force push。
- 若确实需要临时直接推，可先去 GitHub 仓库 Settings → Branches 关闭对应分支保护，或临时把 `enforce_admins` 设 true 再以管理员身份操作（视你权限）。
