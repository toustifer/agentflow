#!/usr/bin/env node
// CLI: node mode-cli.js on|off|status [--namespace ID] [--dag ID] [--note TEXT]
// Writes/reads <project>/.claude/agentflow/mode.json

const {
  projectDir,
  enableMode,
  disableMode,
  readMode,
  isEnabled,
  stickyModeReadiness,
} = require("./mode-lib");

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--namespace" || a === "--namespace_id") {
      args.namespace_id = argv[++i];
    } else if (a === "--dag" || a === "--dag_id") {
      args.dag_id = argv[++i];
    } else if (a === "--note") {
      args.note = argv[++i];
    } else if (a === "--project" || a === "--cwd") {
      args.project = argv[++i];
    } else if (a.startsWith("--")) {
      // ignore unknown flags
    } else {
      args._.push(a);
    }
  }
  return args;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const cmd = (args._[0] || "status").toLowerCase();
  const root = args.project || projectDir();

  if (cmd === "on" || cmd === "enable") {
    const result = enableMode(
      {
        namespace_id: args.namespace_id,
        dag_id: args.dag_id,
        note: args.note,
      },
      root
    );
    process.stdout.write(
      JSON.stringify(
        {
          ok: true,
          action: "on",
          path: result.path,
          mode: result.data,
          readiness: stickyModeReadiness(),
        },
        null,
        2
      ) + "\n"
    );
    return;
  }

  if (cmd === "off" || cmd === "disable") {
    const result = disableMode(root);
    process.stdout.write(
      JSON.stringify(
        {
          ok: true,
          action: "off",
          path: result.path,
          mode: result.data,
          readiness: stickyModeReadiness(),
        },
        null,
        2
      ) + "\n"
    );
    return;
  }

  if (cmd === "status" || cmd === "show") {
    const m = readMode(root);
    process.stdout.write(
      JSON.stringify(
        {
          ok: true,
          action: "status",
          enabled: isEnabled(root),
          path: m ? m.path : null,
          mode: m ? m.data : null,
          project_dir: root,
          readiness: stickyModeReadiness(),
        },
        null,
        2
      ) + "\n"
    );
    return;
  }

  process.stderr.write(
    "Usage: node mode-cli.js on|off|status [--namespace ID] [--dag ID] [--note TEXT] [--project DIR]\n"
  );
  process.exit(2);
}

main();
