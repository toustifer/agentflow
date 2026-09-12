# Pack skills/agentflow into dist/skill.tgz (Windows PowerShell)
param(
  [string]$OutDir
)

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not $OutDir) {
  $OutDir = Join-Path $Root "dist"
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$Stage = Join-Path $env:TEMP ("agentflow-pack-" + [guid]::NewGuid().ToString("n"))
New-Item -ItemType Directory -Force -Path $Stage | Out-Null

try {
  $StageAgentflow = Join-Path $Stage "agentflow"
  New-Item -ItemType Directory -Force -Path $StageAgentflow | Out-Null

  $Src = Join-Path $Root "skills\agentflow"
  if (-not (Test-Path $Src)) {
    throw "missing $Src"
  }

  Write-Host "==> copying skills/agentflow"
  Copy-Item -Recurse -Force (Join-Path $Src "*") $StageAgentflow
  $junk = @("__pycache__", "bin", "node_modules", ".DS_Store")
  foreach ($j in $junk) {
    Get-ChildItem $StageAgentflow -Recurse -Filter $j -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
  }
  Get-ChildItem $StageAgentflow -Recurse -Filter "*.pyc" -File -Force -ErrorAction SilentlyContinue | Remove-Item -Force
  Get-ChildItem $StageAgentflow -Recurse -Filter "*.exe" -File -Force -ErrorAction SilentlyContinue | Remove-Item -Force

  # Ensure bt_service is fresh from root
  Write-Host "==> packaging bt_service"
  $destBt = Join-Path $StageAgentflow "bt_service"
  if (Test-Path $destBt) { Remove-Item -Recurse -Force $destBt }
  Copy-Item -Recurse -Force (Join-Path $Root "bt_service") $destBt
  Get-ChildItem $destBt -Recurse -Filter "__pycache__" -Directory -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
  Get-ChildItem $destBt -Recurse -Filter "*.pyc" -File -Force -ErrorAction SilentlyContinue | Remove-Item -Force
  $testDir = Join-Path $destBt "tests"
  if (Test-Path $testDir) { Remove-Item -Recurse -Force $testDir }

  # Ensure trees is fresh from root
  Write-Host "==> packaging trees"
  $destTrees = Join-Path $StageAgentflow "trees"
  if (Test-Path $destTrees) { Remove-Item -Recurse -Force $destTrees }
  Copy-Item -Recurse -Force (Join-Path $Root "trees") $destTrees
  Get-ChildItem $destTrees -Recurse -Filter "__pycache__" -Directory -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
  Get-ChildItem $destTrees -Recurse -Filter "*.pyc" -File -Force -ErrorAction SilentlyContinue | Remove-Item -Force

  # Copy requirements.txt
  Copy-Item -Force (Join-Path $Root "requirements.txt") (Join-Path $StageAgentflow "requirements.txt")

  # Sanity checks
  if (-not (Test-Path (Join-Path $StageAgentflow "bt_service"))) {
    throw "ERROR: skill package missing bt_service/ — refuse pack"
  }
  if (-not (Test-Path (Join-Path $StageAgentflow "trees"))) {
    throw "ERROR: skill package missing trees/ — refuse pack"
  }
  if (-not (Test-Path (Join-Path $StageAgentflow "requirements.txt"))) {
    throw "ERROR: skill package missing requirements.txt — refuse pack"
  }
  $modeLib = Join-Path $StageAgentflow "hooks\mode-lib.js"
  if (-not (Select-String -Path $modeLib -Pattern "MCP GATE" -Quiet)) {
    throw "ERROR: hooks/mode-lib.js missing MCP GATE — refuse pack"
  }
  if (-not (Test-Path (Join-Path $StageAgentflow "VERSION"))) {
    throw "ERROR: VERSION missing — refuse pack"
  }
  if (-not (Test-Path (Join-Path $StageAgentflow "hooks\version-check.js"))) {
    throw "ERROR: hooks/version-check.js missing — refuse pack"
  }
  if (-not (Test-Path (Join-Path $StageAgentflow "flows\update.md"))) {
    throw "ERROR: flows/update.md missing — refuse pack"
  }

  $outTgz = Join-Path $OutDir "skill.tgz"
  tar -czf $outTgz -C $Stage agentflow
  $size = (Get-Item $outTgz).Length
  $ver = (Get-Content (Join-Path $StageAgentflow "VERSION")).Trim()

  Write-Host "packed $outTgz ($size bytes)"
  Write-Host "skill VERSION=$ver"
} finally {
  Remove-Item -Recurse -Force $Stage -ErrorAction SilentlyContinue
}
