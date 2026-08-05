const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const versionCheck = require("../skills/agentflow/hooks/version-check");

const repoRoot = path.resolve(__dirname, "..");
const skillVersion = fs
  .readFileSync(path.join(repoRoot, "skills", "agentflow", "VERSION"), "utf8")
  .trim();

test("upgrade commands configure Claude and Codex with one version", () => {
  const commands = versionCheck.buildInstallCommands(skillVersion);

  assert.match(commands.unix, /--write-config/);
  assert.match(commands.unix, /--write-codex-config/);
  assert.match(commands.windows, /-WriteConfig/);
  assert.match(commands.windows, /-WriteCodexConfig/);
  assert.match(commands.unix, new RegExp(`VERSION=${skillVersion}`));
  assert.match(commands.windows, new RegExp(`-Version '${skillVersion}'`));
});

test("installers default to the packaged skill version and support Codex", () => {
  const installSh = fs.readFileSync(path.join(repoRoot, "scripts", "install.sh"), "utf8");
  const installPs1 = fs.readFileSync(path.join(repoRoot, "scripts", "install.ps1"), "utf8");

  assert.ok(installSh.includes(`VERSION="\${VERSION:-${skillVersion}}"`));
  assert.match(installSh, /--write-codex-config/);
  assert.match(installSh, /codex mcp add agentflow/);

  assert.ok(installPs1.includes(`else { "${skillVersion}" }`));
  assert.match(installPs1, /WriteCodexConfig/);
  assert.match(installPs1, /codex mcp add agentflow/);
});

test("setup guide installs and verifies both MCP clients", () => {
  const setup = fs.readFileSync(
    path.join(repoRoot, "skills", "agentflow", "SETUP.md"),
    "utf8"
  );

  assert.ok(setup.includes(`Release **${skillVersion}**`));
  assert.match(setup, /--write-config --write-codex-config/);
  assert.match(setup, /codex mcp list/);
  assert.match(setup, /完全退出并重启 Claude Code 和 Codex/);
});
