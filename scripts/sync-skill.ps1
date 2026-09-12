# Synchronize bt_service, trees, and requirements.txt into skills/agentflow/
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$Dest = Join-Path $Root "skills\agentflow"

Write-Host "==> syncing bt_service and trees to $Dest"

$destBt = Join-Path $Dest "bt_service"
$destTrees = Join-Path $Dest "trees"

if (Test-Path $destBt) { Remove-Item -Recurse -Force $destBt }
if (Test-Path $destTrees) { Remove-Item -Recurse -Force $destTrees }

Copy-Item -Recurse -Force (Join-Path $Root "bt_service") $destBt
Get-ChildItem $destBt -Recurse -Filter "__pycache__" -Directory -Force | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
Get-ChildItem $destBt -Recurse -Filter "*.pyc" -File -Force | Remove-Item -Force -ErrorAction SilentlyContinue
$testDir = Join-Path $destBt "tests"
if (Test-Path $testDir) { Remove-Item -Recurse -Force $testDir }

Copy-Item -Recurse -Force (Join-Path $Root "trees") $destTrees
Get-ChildItem $destTrees -Recurse -Filter "__pycache__" -Directory -Force | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
Get-ChildItem $destTrees -Recurse -Filter "*.pyc" -File -Force | Remove-Item -Force -ErrorAction SilentlyContinue

Copy-Item -Force (Join-Path $Root "requirements.txt") (Join-Path $Dest "requirements.txt")

Write-Host "==> sync complete"
