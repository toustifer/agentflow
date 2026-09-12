# Build release binaries for all target platforms + pack skill.tgz
param(
  [string]$Version = "v0.2.7",
  [string]$OutDir
)

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not $OutDir) {
  $OutDir = Join-Path $Root "dist"
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$Commit = (git rev-parse --short HEAD 2>$null)
if (-not $Commit) { $Commit = "unknown" }
$Date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$LdFlags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.date=$Date"

function Build-One {
  param([string]$Goos, [string]$Goarch, [string]$OutName)
  Write-Host ">> building $OutName (GOOS=$Goos GOARCH=$Goarch)"
  $env:CGO_ENABLED = "0"
  $env:GOOS = $Goos
  $env:GOARCH = $Goarch
  $targetPath = Join-Path $OutDir $OutName
  go build -trimpath -ldflags "$LdFlags" -o $targetPath ./cmd/agentflow/
  if (-not (Test-Path $targetPath)) {
    throw "failed to build $targetPath"
  }
  $size = (Get-Item $targetPath).Length
  Write-Host "   -> built $OutName ($size bytes)"
}

Build-One "windows" "amd64" "agentflow-windows-amd64.exe"
Build-One "linux" "amd64" "agentflow-linux-amd64"
Build-One "darwin" "amd64" "agentflow-darwin-amd64"
Build-One "darwin" "arm64" "agentflow-darwin-arm64"

Write-Host ">> packing skill.tgz"
& (Join-Path $PSScriptRoot "pack-skill.ps1") -OutDir $OutDir

Write-Host ""
Write-Host "Release artifacts in $OutDir :"
Get-ChildItem (Join-Path $OutDir "agentflow-*"), (Join-Path $OutDir "skill.tgz") | Select-Object Name, Length, LastWriteTime
