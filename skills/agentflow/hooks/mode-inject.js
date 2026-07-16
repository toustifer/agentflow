#!/usr/bin/env node
// UserPromptSubmit hook: if agentflow mode is on, inject additionalContext.
// Configure in settings.json:
// {
//   "hooks": {
//     "UserPromptSubmit": [{
//       "hooks": [{
//         "type": "command",
//         "command": "node <path-to>/mode-inject.js",
//         "timeout": 5
//       }]
//     }]
//   }
// }

const fs = require("fs");
const { readMode, buildAdditionalContext, projectDir, readJsonFile } = require("./mode-lib");

function readStdin() {
  return new Promise((resolve) => {
    let data = "";
    if (process.stdin.isTTY) {
      resolve("");
      return;
    }
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => {
      data += chunk;
    });
    process.stdin.on("end", () => resolve(data));
    // safety: don't hang forever if no stdin
    setTimeout(() => resolve(data), 200);
  });
}

function readJsonFileFromText(text) {
  try {
    return JSON.parse(text);
  } catch (_) {
    return null;
  }
}

function detectProjectDir(input) {
  return (
    (input && input.workspace && input.workspace.project_dir) ||
    (input && input.cwd) ||
    projectDir()
  );
}

async function main() {
  const raw = await readStdin();
  const input = raw ? readJsonFileFromText(raw) : null;

  const root = detectProjectDir(input);
  const m = readMode(root);
  if (!m || !m.data || !m.data.enabled) {
    // mode off: silent no-op
    process.exit(0);
  }

  // Optional: skip re-injection when the user is explicitly toggling mode
  // (prompt text may be available in future; for now always inject when on).

  const additionalContext = buildAdditionalContext(m.data);
  const out = {
    hookSpecificOutput: {
      hookEventName: "UserPromptSubmit",
      additionalContext,
      sessionTitle: "agentflow:on",
    },
  };
  process.stdout.write(JSON.stringify(out));
  process.exit(0);
}

main().catch((err) => {
  // Never block the user prompt on hook failure.
  try {
    fs.appendFileSync(
      require("path").join(require("os").homedir(), ".claude", "agentflow-mode-hook.log"),
      new Date().toISOString() + " " + String(err && err.stack ? err.stack : err) + "\n"
    );
  } catch (_) {}
  process.exit(0);
});
