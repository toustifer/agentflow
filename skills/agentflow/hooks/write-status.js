#!/usr/bin/env node

const fs = require("fs");
const { projectDir, writeStatus } = require("./mode-lib");

function readStdinJson() {
  try {
    if (process.stdin.isTTY) return null;
    const raw = fs.readFileSync(0, "utf8");
    if (!raw) return null;
    return JSON.parse(raw);
  } catch (_) {
    return null;
  }
}

function main() {
  const input = readStdinJson();
  if (!input || typeof input !== "object") {
    process.stderr.write("Expected JSON snapshot on stdin\n");
    process.exit(2);
  }
  const root = input.project_dir || projectDir();
  const result = writeStatus(input, root);
  process.stdout.write(JSON.stringify({ ok: true, path: result.path }, null, 2) + "\n");
}

main();
