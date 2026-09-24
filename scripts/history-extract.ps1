# History Decision Point Extractor CLI for Agentflow
# Conforms to PROPOSAL_HISTORY_REPLAY.md §4.1 and schemas/decision-point.schema.json

[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [string]$DbPath = "C:\Users\15775\.dsh\agentflow\agentflow.db",

    [Parameter(Mandatory = $true)]
    [string]$OutPath,

    [Parameter(Mandatory = $false)]
    [string]$RepoPath = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,

    [Parameter(Mandatory = $false)]
    [switch]$Validate
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $DbPath)) {
    Write-Error "Database file not found: $DbPath"
    exit 2
}

# 1. Record pre-extraction DB integrity
$preItem = Get-Item -LiteralPath $DbPath
$preHash = (Get-FileHash -LiteralPath $DbPath -Algorithm SHA256).Hash
$preMtime = $preItem.LastWriteTimeUtc.ToString("o")

# 2. Check global %TEMP%\agentflow.db pre-state
$tempGlobalDb = Join-Path $env:TEMP "agentflow.db"
$tempGlobalPreExists = Test-Path -LiteralPath $tempGlobalDb
$tempGlobalPreMtime = $null
if ($tempGlobalPreExists) {
    $tempGlobalPreMtime = (Get-Item -LiteralPath $tempGlobalDb).LastWriteTimeUtc.ToString("o")
}

# 3. Call python extractor
$scriptPy = Join-Path $PSScriptRoot "history_extract.py"
$pyArgs = @(
    $scriptPy,
    "--db-path", $DbPath,
    "--out-path", $OutPath,
    "--repo-path", $RepoPath
)
if ($Validate) {
    $pyArgs += "--validate"
}

& python @pyArgs
$extractExit = $LASTEXITCODE

# 4. Record post-extraction DB integrity
$postItem = Get-Item -LiteralPath $DbPath
$postHash = (Get-FileHash -LiteralPath $DbPath -Algorithm SHA256).Hash
$postMtime = $postItem.LastWriteTimeUtc.ToString("o")

if ($preHash -ne $postHash -or $preMtime -ne $postMtime) {
    Write-Error "CRITICAL: Live database was modified during extraction! Pre=$preHash Post=$postHash"
    exit 3
}

# 5. Verify global %TEMP%\agentflow.db was not modified
if ($tempGlobalPreExists) {
    $tempGlobalPostMtime = (Get-Item -LiteralPath $tempGlobalDb).LastWriteTimeUtc.ToString("o")
    if ($tempGlobalPreMtime -ne $tempGlobalPostMtime) {
        Write-Error "CRITICAL: Global %TEMP%\agentflow.db was modified during extraction!"
        exit 4
    }
} else {
    if (Test-Path -LiteralPath $tempGlobalDb) {
        Write-Error "CRITICAL: Global %TEMP%\agentflow.db was created during extraction!"
        exit 4
    }
}

if ($extractExit -ne 0) {
    exit $extractExit
}

Write-Host "Extractor finished cleanly with exit code 0."
exit 0
