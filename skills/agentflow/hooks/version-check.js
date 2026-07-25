// Version check for skill + MCP binary vs GitHub latest release.
// Used by /agentflow update and mode-cli.js update.

const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");
const { execFileSync } = require("child_process");
const { probeAgentflowMcpConfig } = require("./mode-lib");

const REPO = process.env.AGENTFLOW_REPO || "toustifer/agentflow";
const DEFAULT_RELEASE_API =
  process.env.AGENTFLOW_RELEASE_API ||
  `https://api.github.com/repos/${REPO}/releases/latest`;

function skillRoot() {
  // hooks/ -> skill root
  return path.resolve(__dirname, "..");
}

function readSkillVersion(root) {
  const base = root || skillRoot();
  const candidates = [
    path.join(base, "VERSION"),
    path.join(base, "version"),
  ];
  for (const p of candidates) {
    try {
      if (fs.existsSync(p)) {
        const v = fs.readFileSync(p, "utf8").trim();
        if (v) return { version: normalizeVersion(v), path: p, ok: true };
      }
    } catch (_) {}
  }
  return {
    version: null,
    path: path.join(base, "VERSION"),
    ok: false,
    reason: "VERSION file missing in skill install",
  };
}

function normalizeVersion(v) {
  if (!v) return null;
  let s = String(v).trim();
  // strip leading "agentflow " etc
  s = s.replace(/^agentflow\s+/i, "").trim();
  if (!s.startsWith("v") && /^\d+\.\d+/.test(s)) s = "v" + s;
  return s;
}

function compareSemver(a, b) {
  // return -1 if a<b, 0 equal, 1 if a>b; null if unparsable
  const pa = parseSemver(a);
  const pb = parseSemver(b);
  if (!pa || !pb) return null;
  for (let i = 0; i < 3; i++) {
    if (pa[i] < pb[i]) return -1;
    if (pa[i] > pb[i]) return 1;
  }
  return 0;
}

function parseSemver(v) {
  if (!v) return null;
  const m = String(v)
    .trim()
    .replace(/^v/i, "")
    .match(/^(\d+)\.(\d+)\.(\d+)/);
  if (!m) return null;
  return [parseInt(m[1], 10), parseInt(m[2], 10), parseInt(m[3], 10)];
}

function expandUserPath(p) {
  if (!p || typeof p !== "string") return p;
  if (p.startsWith("~/")) return path.join(os.homedir(), p.slice(2));
  return p;
}

function resolveBinaryFromMcp() {
  const mcp = probeAgentflowMcpConfig();
  if (!mcp.configured || !mcp.sources || !mcp.sources.length) {
    return { path: null, mcp, reason: "agentflow MCP not configured" };
  }
  for (const src of mcp.sources) {
    const candidates = [];
    if (src.command) candidates.push(src.command);
    if (Array.isArray(src.args)) {
      for (const a of src.args) {
        if (typeof a === "string" && /agentflow/i.test(a)) candidates.push(a);
      }
    }
    for (const c of candidates) {
      const exp = expandUserPath(c);
      // skip pure launchers
      if (/^(node|python|python3|npx|cmd|powershell)$/i.test(path.basename(exp))) {
        continue;
      }
      try {
        if (fs.existsSync(exp) && fs.statSync(exp).isFile()) {
          return { path: exp, mcp, source: src.source };
        }
      } catch (_) {}
    }
  }
  // fallback skill bin
  const skillBin = path.join(
    skillRoot(),
    "bin",
    process.platform === "win32" ? "agentflow.exe" : "agentflow"
  );
  if (fs.existsSync(skillBin)) {
    return { path: skillBin, mcp, source: "skill_bin_fallback" };
  }
  return { path: null, mcp, reason: "binary path not found on disk" };
}

function readBinaryVersion(binPath) {
  if (!binPath) {
    return { ok: false, version: null, reason: "no binary path" };
  }
  const run = (args) =>
    execFileSync(binPath, args, {
      encoding: "utf8",
      timeout: 2500,
      windowsHide: true,
      killSignal: "SIGKILL",
    });
  try {
    const out = run(["version-json"]);
    const j = JSON.parse(out);
    return {
      ok: true,
      version: normalizeVersion(j.version),
      commit: j.commit || null,
      date: j.date || null,
      path: binPath,
      raw: j,
    };
  } catch (e1) {
    try {
      const out = run(["version"]);
      // "agentflow v0.2.2 (commit abc, built ...)"
      const m = String(out).match(/agentflow\s+(\S+)/i);
      return {
        ok: !!m,
        version: m ? normalizeVersion(m[1]) : null,
        path: binPath,
        raw_text: String(out).trim(),
        reason: m ? null : "could not parse version output",
      };
    } catch (e2) {
      const msg = e2 && e2.message ? e2.message : String(e2);
      const timedOut = /ETIMEDOUT|timed out|TIMEOUT/i.test(msg);
      return {
        ok: false,
        version: null,
        path: binPath,
        reason: timedOut
          ? "binary has no version command (pre-update builds hang on unknown args) — reinstall latest Release so MCP can report version"
          : "binary version failed: " + msg,
      };
    }
  }
}

function fetchLatestRelease(apiUrl) {
  const url = apiUrl || DEFAULT_RELEASE_API;
  return new Promise((resolve) => {
    const req = https.get(
      url,
      {
        headers: {
          "User-Agent": "agentflow-update-check",
          Accept: "application/vnd.github+json",
        },
        timeout: 8000,
      },
      (res) => {
        let body = "";
        res.on("data", (c) => (body += c));
        res.on("end", () => {
          if (res.statusCode && res.statusCode >= 400) {
            resolve({
              ok: false,
              reason: `GitHub HTTP ${res.statusCode}`,
              status: res.statusCode,
            });
            return;
          }
          try {
            const j = JSON.parse(body);
            const tag = normalizeVersion(j.tag_name || j.name || "");
            resolve({
              ok: !!tag,
              version: tag,
              tag_name: j.tag_name,
              html_url: j.html_url,
              published_at: j.published_at,
              name: j.name,
            });
          } catch (e) {
            resolve({ ok: false, reason: "invalid JSON from GitHub" });
          }
        });
      }
    );
    req.on("error", (e) =>
      resolve({ ok: false, reason: "network: " + e.message })
    );
    req.on("timeout", () => {
      req.destroy();
      resolve({ ok: false, reason: "network timeout" });
    });
  });
}

function statusFor(local, latest) {
  if (!local) return "unknown_local";
  if (!latest) return "unknown_remote";
  const c = compareSemver(local, latest);
  if (c === null) return "unparsable";
  if (c < 0) return "outdated";
  if (c > 0) return "ahead";
  return "up_to_date";
}

async function checkVersions(opts) {
  opts = opts || {};
  const skill = readSkillVersion(opts.skillRoot);
  const binLoc = resolveBinaryFromMcp();
  const binary = readBinaryVersion(binLoc.path);
  let latest = { ok: false, reason: "skipped" };
  if (opts.skipNetwork) {
    latest = { ok: false, reason: "skip_network", skipped: true };
  } else {
    latest = await fetchLatestRelease(opts.releaseApi);
  }

  const skillStatus = statusFor(skill.version, latest.ok ? latest.version : null);
  const mcpStatus = statusFor(binary.version, latest.ok ? latest.version : null);

  const needsUpdate =
    skillStatus === "outdated" ||
    mcpStatus === "outdated" ||
    (skill.ok && binary.ok && skill.version && binary.version && skill.version !== binary.version);

  const mismatch =
    skill.ok &&
    binary.ok &&
    skill.version &&
    binary.version &&
    skill.version !== binary.version;

  const target = latest.ok ? latest.version : "v0.2.2";
  const installCmdUnix = `curl -fsSL https://raw.githubusercontent.com/${REPO}/master/scripts/install.sh | VERSION=${target} bash -s -- --write-config`;
  const installCmdWin = `$env:VERSION='${target}'; irm https://raw.githubusercontent.com/${REPO}/master/scripts/install.ps1 | iex`;

  return {
    ok: true,
    checked_at: new Date().toISOString(),
    skill: {
      version: skill.version,
      path: skill.path,
      skill_root: opts.skillRoot || skillRoot(),
      ok: skill.ok,
      reason: skill.reason || null,
      status: skillStatus,
    },
    mcp: {
      version: binary.version,
      commit: binary.commit || null,
      date: binary.date || null,
      binary_path: binary.path || binLoc.path,
      ok: binary.ok,
      reason: binary.reason || binLoc.reason || null,
      status: mcpStatus,
      config: binLoc.mcp || null,
    },
    latest: latest,
    mismatch_skill_mcp: !!mismatch,
    needs_update: !!needsUpdate,
    install: {
      unix: installCmdUnix,
      windows: installCmdWin,
      release_url: latest.html_url || `https://github.com/${REPO}/releases`,
      docs: "https://hub.stifer.xyz/agent-setup.md",
    },
    next_steps: buildNextSteps({
      skillStatus,
      mcpStatus,
      mismatch,
      skill,
      binary,
      latest,
      needsUpdate,
      installCmdUnix,
      installCmdWin,
    }),
  };
}

function buildNextSteps(x) {
  const steps = [];
  if (!x.skill.ok) {
    steps.push("Skill VERSION missing — reinstall skill.tgz from latest Release.");
  }
  if (!x.binary.ok) {
    steps.push(
      "MCP binary version unreadable — check ~/.claude.json command path, then reinstall binary."
    );
  }
  if (x.mismatch) {
    steps.push(
      `Skill (${x.skill.version}) and MCP binary (${x.binary.version}) differ — reinstall BOTH with the same VERSION.`
    );
  }
  if (x.skillStatus === "outdated" || x.mcpStatus === "outdated") {
    steps.push("Run the install command below, then fully quit and restart Claude Code.");
    steps.push("macOS/Linux: " + x.installCmdUnix);
    steps.push("Windows: " + x.installCmdWin);
    steps.push("After restart: call flow_ping and confirm version field matches latest.");
  }
  if (!x.needsUpdate && x.skill.ok && x.binary.ok && !x.mismatch) {
    steps.push("Both skill and MCP binary look current (or remote check skipped). No upgrade required.");
  }
  if (x.latest && x.latest.skipped) {
    steps.push("Remote latest skipped; pass network check for upgrade advice.");
  } else if (x.latest && !x.latest.ok && !x.latest.skipped) {
    steps.push(
      "Could not reach GitHub latest release (" +
        (x.latest.reason || "unknown") +
        "). Check https://github.com/" +
        REPO +
        "/releases manually."
    );
  }
  steps.push("Verify locations: skill VERSION file + MCP binary path in /mcp config.");
  return steps;
}

function formatHumanReport(report) {
  const lines = [];
  lines.push("agentflow update check");
  lines.push("======================");
  lines.push(
    `skill:  ${report.skill.version || "?"}  [${report.skill.status}]  (${report.skill.path})`
  );
  lines.push(
    `mcp:    ${report.mcp.version || "?"}  [${report.mcp.status}]  (${report.mcp.binary_path || "n/a"})`
  );
  if (report.mcp.commit) lines.push(`        commit ${report.mcp.commit}  built ${report.mcp.date || "?"}`);
  if (report.latest && report.latest.ok) {
    lines.push(`latest: ${report.latest.version}  ${report.latest.html_url || ""}`);
  } else {
    lines.push(`latest: unavailable (${(report.latest && report.latest.reason) || "?"})`);
  }
  if (report.mismatch_skill_mcp) {
    lines.push("WARN: skill and MCP binary versions differ — reinstall both.");
  }
  lines.push(`needs_update: ${report.needs_update ? "YES" : "no"}`);
  lines.push("");
  lines.push("Where to check:");
  lines.push(`  - skill VERSION: ${report.skill.path}`);
  lines.push(`  - MCP binary:    ${report.mcp.binary_path || "(not found)"}`);
  lines.push(`  - releases:      ${report.install.release_url}`);
  lines.push(`  - docs:          ${report.install.docs}`);
  lines.push("");
  lines.push("Next:");
  for (const s of report.next_steps) lines.push("  - " + s);
  if (report.needs_update) {
    lines.push("");
    lines.push("Upgrade (macOS/Linux):");
    lines.push("  " + report.install.unix);
    lines.push("Upgrade (Windows PowerShell):");
    lines.push("  " + report.install.windows);
    lines.push("Then fully quit Claude Code and restart. Re-run /agentflow update.");
  }
  return lines.join("\n");
}

module.exports = {
  checkVersions,
  formatHumanReport,
  readSkillVersion,
  readBinaryVersion,
  resolveBinaryFromMcp,
  normalizeVersion,
  compareSemver,
  skillRoot,
};

if (require.main === module) {
  const skipNetwork = process.argv.includes("--offline");
  const asJson = process.argv.includes("--json");
  checkVersions({ skipNetwork })
    .then((r) => {
      if (asJson) {
        process.stdout.write(JSON.stringify(r, null, 2) + "\n");
      } else {
        process.stdout.write(formatHumanReport(r) + "\n");
      }
      // exit 1 if update needed (handy for CI/scripts)
      process.exit(r.needs_update ? 1 : 0);
    })
    .catch((e) => {
      process.stderr.write(String(e && e.stack ? e.stack : e) + "\n");
      process.exit(2);
    });
}
