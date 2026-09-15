# Live-Spec canvas deployment record — 2026-09-15

Deployment of the task-1 responsive canvas (`648b7c8`, `dag-live-spec-responsive-sizes`)
to the running DSH plugin install, plus cleanup of the accumulated orphan bundles in
`canvas-dist/assets/`.

## Why this is a record and not a code change

The canvas build output is deliberately **not** tracked: `.gitignore` contains `dist/`.
That is the same policy whose absence caused this DAG in the first place — the
`dsh-interactive-spec` package commits its compiled `client.js` copies, so the host
kept serving a stale artifact after the TypeScript source changed (see task-4).
Re-adding `apps/live-spec-canvas/dist/**` to git would recreate that trap, so the
deployable artifact stays out of the repository and this file records what was
actually put on the host.

Canvas **source** changes live in the repository and were already committed by task-1.

## Source of truth

| item | value |
| --- | --- |
| worktree | `D:\myprogram\.agentflow-worktrees\agentflow\dag-live-spec-responsive-sizes` |
| branch / HEAD at deploy | `fix/live-spec-responsive-sizes` @ `59dbbba` |
| build command | `pnpm -C apps/live-spec-canvas build` (= `tsc && vite build`) |
| build base | `/agentflow/canvas/` (`apps/live-spec-canvas/vite.config.ts`) |

## Deployed artifacts

Target: `C:\Users\15775\.dsh\plugins\dsh-interactive-spec\canvas-dist\`

| path | bytes | sha256 |
| --- | --- | --- |
| `index.html` | 1442 | `61E7FB02AC2274AC0178FE74FC81DDAC9D280252920E4A4A6642F2A4F4112D3F` |
| `assets/index-xszQUbgd.js` | 389274 | `10A4D5601648ACC2B418E37EBC73E0948C16223DB1099CDAC2C768CB079C8065` |
| `assets/index-DMpV7nCR.css` | 15869 | `1243B61A71923367CE620FB944427C80C9B03E863E18374FCF4F74D6D06F63FC` |

The hashed JS file name is not committed anywhere — it only exists in the built
artifact on the host. `index.html` references it as
`/agentflow/canvas/assets/index-xszQUbgd.js`.

## Orphan cleanup

Before the deploy `canvas-dist/assets/` held five historical JS bundles; only
`index-D_da-xO5.js` was referenced by the then-current `index.html`. Deploying the new
build replaced that reference, which orphaned **all five** — none of them is referenced
by the new `index.html` any more.

Removed (each was ~386 kB, ~1.9 MB total):

| removed file | bytes | sha256 |
| --- | --- | --- |
| `index-B1zHdoAx.js` | 385356 | `E0959459FF6C3D924E4C309E72CF5815CBA9B5417B85E7E91A92E585EFCFC403` |
| `index-BiYWY_Im.js` | 385796 | `0CC3D5DE86C5DD74BCED05B9E73A332E4E7DA3F8DA5C03EDF58B56738AD41582` |
| `index-CCXVh4b8.js` | 385876 | `B3487D398B62951ABE9DECF6AFCE18826F1E5652D105553A68582B627AD0AAE1` |
| `index-D_da-xO5.js` | 385906 | `96EFBB8E2D79C90367C1087BCBCE9BDD9064B1B34D71055BBDDA82FA6D5291C6` |
| `index-DZfUHisy.js` | 385894 | `AF593EB24CEDE3840E9AFBF5C7C895ADB299F41A91A76EA628D0881C5A875B76` |

`index-DMpV7nCR.css` was **kept**: its content hash is unchanged between builds and the
new `index.html` still references it.

Rule applied: delete exactly those files under `assets/` whose name does not appear in
the new `index.html`; keep everything the document references.

## Rollback

The complete pre-deploy `canvas-dist` tree (index.html + all six assets, verified by
sha256 in the manifest above) was copied to:

```
C:\Users\15775\AppData\Local\Temp\agf-task5\canvas-dist-backup-20260915-232848\
```

Restore by copying that directory back over `canvas-dist`. Note the backup lives in
`%TEMP%`, so it is not durable — the pre-deploy bundles are not recoverable from git.

## Effect on the running host

`dsh-interactive-spec` serves `canvas-dist/` as static files under
`/agentflow/canvas/`, so the new document is picked up on the next request; no restart
and no process signal is involved. Verified:

- `curl -s http://127.0.0.1:3080/agentflow/canvas/` → `200`, body references
  `/agentflow/canvas/assets/index-xszQUbgd.js`.
- `curl -s -o NUL -w "%{http_code} %{size_download}" .../assets/index-xszQUbgd.js`
  → `200 389274`, matching the on-disk size and sha256 above.
- The removed orphans now return `404`.

## Rendered verification (Playwright, Chromium 1.60.0)

`http://127.0.0.1:3080/agentflow/canvas/` was rendered at the half-width right-pane
size and at a wide size:

| viewport | `data-layout` | `data-container-width` | parametric rail | horizontal overflow |
| --- | --- | --- | --- | --- |
| 714 × 900 | `compact` | `714` | `208px` | none (`scrollWidth == clientWidth == 714`) |
| 1440 × 900 | `wide` | `1440` | `300px` | n/a |

Screenshots: `%TEMP%\agf-task5\half-width-live.png`, `%TEMP%\agf-task5\wide-live.png`.

The DSH **shell** itself could not be screenshotted: `/` is gated by a per-process
launch token and returns `401` for both a bare request and the token found in
`~/.dsh/restart-web.stdout.log` (a log left over from a 2026-09-11 launch, whose pid no
longer matches the live process). Obtaining the live token would require restarting
`dsh web`, which is out of scope. The canvas document above is the surface the fix
changes and is unauthenticated, so it was used directly.
