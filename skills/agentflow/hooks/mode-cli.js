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
  probeAgentflowMcpConfig,
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
    const mcp = probeAgentflowMcpConfig();
    const warnings = [];
    if (!mcp.configured) {
      warnings.push(
        "MCP GATE: agentflow is not in ~/.claude.json. Mode is ON but work is blocked until /mcp shows agentflow connected. Fix MCP first — do not Bash-bridge."
      );
    } else if (!mcp.paths_ok) {
      warnings.push(
        "MCP GATE: agentflow config has missing paths (" +
          (mcp.missing_paths || []).join(", ") +
          "). Fix paths and restart Claude Code before any project work."
      );
    } else {
      warnings.push(
        "MCP config present, but still verify THIS session has mcp__agentflow__* tools (open /mcp; call flow_ping). claude mcp list Connected is not enough."
      );
    }
    process.stdout.write(
      JSON.stringify(
        {
          ok: true,
          action: "on",
          path: result.path,
          mode: result.data,
          readiness: stickyModeReadiness(),
          mcp,
          warnings,
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
          mcp: probeAgentflowMcpConfig(),
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
