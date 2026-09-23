# preset-drift.ps1 — guard the live <-> repo mirror of DSH agent presets.
#
# Live side   : %USERPROFILE%\.dsh\.agent-presets\<name>\   (what DSH actually loads)
# Repo mirror : skills\agentflow\agents\<name>\             (what git tracks)
# Derived     : dsh-agentflow\src|lib\skills\agentflow\     (copied by copy-skill.mjs)
#
#   -Check   compare the two sides; print per-pair differences; exit non-zero on drift
#   -Export  live -> repo mirror (LF-normalized), then run copy-skill.mjs
#
# ---------------------------------------------------------------------------
# WHY THE COMPARISON IS ON LINE-ENDING-NORMALIZED CONTENT AND NOT ON RAW SHA256
# ---------------------------------------------------------------------------
# This repository runs with core.autocrlf=true on Windows. That means:
#   commit  : CRLF in the work tree  ->  LF in the blob
#   checkout: LF in the blob        ->  CRLF in the work tree
# The live preset files, by contrast, are whatever DSH itself wrote and their
# line endings are NOT uniform across presets (agentflow-leader and
# agentflow-dev-leader are LF-only; agentflow-worker and agentflow-dev are
# CRLF). So after ANY `git checkout` / clone / `git checkout-index`, every
# mirror file comes back CRLF, while two of the four live files are LF.
#
# Measured on 2026-09-23 (fresh `git checkout-index -a` vs live):
#   agentflow-leader      live LF(479)   fresh CRLF(479)   sha256 DIFFERS
#   agentflow-dev-leader  live LF(202)   fresh CRLF(202)   sha256 DIFFERS
#   agentflow-worker      live CRLF(244) fresh CRLF(244)   sha256 equal (luck)
#   agentflow-dev         live CRLF(148) fresh CRLF(148)   sha256 equal (luck)
#
# A raw byte/sha256 predicate would therefore report "presets drifted" on two of
# four presets immediately after a clean checkout, with zero real drift — a
# false positive that fires exactly when the guard is supposed to say "healthy",
# which trains people to ignore it. The invariant that actually matters is
# "the same bytes modulo line endings", so that is what we compare. -Export in
# turn always writes LF so the repo side stops carrying a CRLF/LF coin flip.
# ---------------------------------------------------------------------------

[CmdletBinding()]
param(
  [switch]$Check,
  [switch]$Export,

  # Live DSH preset root. Override for tests / fresh-tree verification.
  [string]$LiveRoot,

  # Repository root (defaults to the parent of this script's directory).
  [string]$RepoRoot,

  # Max differing lines printed per pair.
  [int]$MaxDiffLines = 20
)

$ErrorActionPreference = "Stop"

# Exit codes (documented in docs/dsh-setup.md):
#   0 = in sync
#   1 = drift / missing mirror file
#   2 = usage error
#   3 = environment error (root missing, unreadable file, bad arguments)
#   4 = export failed (copy-skill.mjs)
$EXIT_OK = 0
$EXIT_DRIFT = 1
$EXIT_USAGE = 2
$EXIT_ENV = 3
$EXIT_EXPORT = 4

$CordisFile = "agent.cordis.yml"
$PresetFile = "preset.yml"

# Backup / scratch names that must never travel from live into the repo mirror.
$ExcludePatterns = @("*.bak", "*.bak-*", "*~", "*.tmp", "*.orig", "*.rej")

function Fail([int]$Code, [string]$Message) {
  Write-Host "ERROR: $Message" -ForegroundColor Red
  exit $Code
}

function Get-AbsPath([string]$Path) {
  return (Resolve-Path -LiteralPath $Path).Path
}

# Read a file as text with BOM stripped and CRLF/CR folded to LF.
# This is the ONLY text-reading path used for comparison, so the whole tool is
# line-ending agnostic by construction.
function Get-NormalizedText([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "missing file: $Path"
  }
  $abs = (Resolve-Path -LiteralPath $Path).Path
  $bytes = [System.IO.File]::ReadAllBytes($abs)
  $text = [System.Text.Encoding]::UTF8.GetString($bytes)
  if ($text.Length -gt 0 -and $text[0] -eq [char]0xFEFF) { $text = $text.Substring(1) }
  $text = $text -replace "`r`n", "`n"
  $text = $text -replace "`r", "`n"
  return $text
}

function Get-NormalizedHash([string]$Text) {
  $sha = [System.Security.Cryptography.SHA256]::Create()
  try {
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($Text)
    $hash = $sha.ComputeHash($bytes)
    return (($hash | ForEach-Object { $_.ToString("x2") }) -join "")
  } finally {
    $sha.Dispose()
  }
}

# Write text as UTF-8 (no BOM) with LF line endings only.
function Write-NormalizedFile([string]$Path, [string]$Text) {
  $dir = Split-Path -Parent $Path
  if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }
  $enc = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $Text, $enc)
}

function Test-Excluded([string]$Name) {
  foreach ($p in $ExcludePatterns) {
    if ($Name -like $p) { return $true }
  }
  return $false
}

function Write-LineDiff([string]$LeftPath, [string]$RightPath, [string]$LeftText, [string]$RightText) {
  $left = $LeftText -split "`n"
  $right = $RightText -split "`n"
  $max = [Math]::Max($left.Length, $right.Length)
  $shown = 0
  $total = 0
  for ($i = 0; $i -lt $max; $i++) {
    $l = if ($i -lt $left.Length) { $left[$i] } else { "<no such line>" }
    $r = if ($i -lt $right.Length) { $right[$i] } else { "<no such line>" }
    if ($l -ne $r) {
      $total++
      if ($shown -lt $MaxDiffLines) {
        Write-Host ("      line {0,4}: live  | {1}" -f ($i + 1), $l)
        Write-Host ("      line {0,4}: repo  | {1}" -f ($i + 1), $r)
        $shown++
      }
    }
  }
  if ($total -gt $shown) {
    Write-Host ("      ... {0} more differing line(s) suppressed" -f ($total - $shown))
  }
  return $total
}

# ---------------------------------------------------------------------------
# Argument resolution
# ---------------------------------------------------------------------------

if ($Check.IsPresent -eq $Export.IsPresent) {
  Write-Host "usage: scripts/preset-drift.ps1 -Check | -Export [-LiveRoot <dir>] [-RepoRoot <dir>]"
  Write-Host "  -Check   live <-> repo mirror comparison (line-ending normalized)"
  Write-Host "  -Export  live -> repo mirror, then dsh-agentflow/scripts/copy-skill.mjs"
  exit $EXIT_USAGE
}

if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
  $RepoRoot = Join-Path $PSScriptRoot ".."
}
if (-not (Test-Path -LiteralPath $RepoRoot -PathType Container)) {
  Fail $EXIT_ENV "repo root not found: $RepoRoot"
}
$RepoRoot = Get-AbsPath $RepoRoot

if ([string]::IsNullOrWhiteSpace($LiveRoot)) {
  if ([string]::IsNullOrWhiteSpace($env:USERPROFILE)) {
    Fail $EXIT_ENV "USERPROFILE is not set; pass -LiveRoot explicitly"
  }
  $LiveRoot = Join-Path $env:USERPROFILE ".dsh\.agent-presets"
}
if (-not (Test-Path -LiteralPath $LiveRoot -PathType Container)) {
  Fail $EXIT_ENV "live preset root does not exist: $LiveRoot (nothing to compare against; refusing to report 'in sync')"
}
$LiveRoot = Get-AbsPath $LiveRoot

$AgentsDir = Join-Path $RepoRoot "skills\agentflow\agents"
if (-not (Test-Path -LiteralPath $AgentsDir -PathType Container)) {
  Fail $EXIT_ENV "repo mirror root does not exist: $AgentsDir"
}
$AgentsDir = Get-AbsPath $AgentsDir

# A directory counts as a preset iff it holds agent.cordis.yml or preset.yml.
function Get-PresetDirs([string]$Root) {
  $result = @{}
  Get-ChildItem -LiteralPath $Root -Directory -Force | Sort-Object Name | ForEach-Object {
    $hasCordis = Test-Path -LiteralPath (Join-Path $_.FullName $CordisFile) -PathType Leaf
    $hasPreset = Test-Path -LiteralPath (Join-Path $_.FullName $PresetFile) -PathType Leaf
    if ($hasCordis -or $hasPreset) { $result[$_.Name] = $_.FullName }
  }
  return $result
}

$liveDirs = Get-PresetDirs $LiveRoot
$repoDirs = Get-PresetDirs $AgentsDir

if ($liveDirs.Count -eq 0) {
  Fail $EXIT_ENV "no preset directories found under $LiveRoot (a stale/empty live root must not read as 'in sync')"
}

Write-Host "live  : $LiveRoot"
Write-Host "mirror: $AgentsDir"
Write-Host "mode  : $(if ($Check) { 'Check (compare line-ending-normalized content)' } else { 'Export (write LF into the mirror)' })"
Write-Host ""

# ---------------------------------------------------------------------------
# -Check
# ---------------------------------------------------------------------------

if ($Check) {
  $problems = New-Object System.Collections.Generic.List[string]
  $names = @($liveDirs.Keys + $repoDirs.Keys | Sort-Object -Unique)

  foreach ($name in $names) {
    $liveDir = $liveDirs[$name]
    $repoDir = $repoDirs[$name]
    $liveFile = if ($liveDir) { Join-Path $liveDir $CordisFile } else { $null }
    $repoFile = if ($repoDir) { Join-Path $repoDir $CordisFile } else { $null }

    if (-not $liveDir) {
      $problems.Add("$name : exists in the repo mirror only (no live preset at $LiveRoot\$name)")
      Write-Host "[DRIFT] $name"
      Write-Host "      repo-only mirror; live directory missing"
      continue
    }
    if (-not (Test-Path -LiteralPath $liveFile -PathType Leaf)) {
      $problems.Add("$name : live $CordisFile missing")
      Write-Host "[ERROR] $name"
      Write-Host "      live $CordisFile missing at $liveDir"
      continue
    }
    if (-not $repoDir) {
      $problems.Add("$name : no repo mirror directory")
      Write-Host "[DRIFT] $name"
      Write-Host "      live preset has no mirror at $AgentsDir\$name"
      continue
    }
    if (-not (Test-Path -LiteralPath $repoFile -PathType Leaf)) {
      $problems.Add("$name : mirror $CordisFile missing")
      Write-Host "[ERROR] $name"
      Write-Host "      mirror $CordisFile missing at $repoDir"
      continue
    }

    try {
      $liveText = Get-NormalizedText $liveFile
      $repoText = Get-NormalizedText $repoFile
    } catch {
      $problems.Add("$name : $($_.Exception.Message)")
      Write-Host "[ERROR] $name"
      Write-Host "      $($_.Exception.Message)"
      continue
    }

    $liveHash = Get-NormalizedHash $liveText
    $repoHash = Get-NormalizedHash $repoText

    if ($liveHash -eq $repoHash) {
      Write-Host ("[ OK  ] {0,-22} normalized-sha256 {1}" -f $name, $liveHash.Substring(0, 16))
      # Mirror completeness, both directions: agent.cordis.yml agrees, but a
      # preset is a whole directory (preset.yml / README.md / skills/<n>/SKILL.md).
      Get-ChildItem -LiteralPath $liveDir -Recurse -File -Force | ForEach-Object {
        $rel = $_.FullName.Substring($liveDir.Length + 1)
        if (-not (Test-Excluded $_.Name) -and -not (Test-Path -LiteralPath (Join-Path $repoDir $rel) -PathType Leaf)) {
          $problems.Add("$name : live file not mirrored: $rel")
          Write-Host "      [DRIFT] live file missing from mirror: $rel"
        }
      }
      Get-ChildItem -LiteralPath $repoDir -Recurse -File -Force | ForEach-Object {
        $rel = $_.FullName.Substring($repoDir.Length + 1)
        if (-not (Test-Excluded $_.Name) -and -not (Test-Path -LiteralPath (Join-Path $liveDir $rel) -PathType Leaf)) {
          $problems.Add("$name : mirror-only file $rel")
          Write-Host "      [DRIFT] mirror-only file (not in live): $rel"
        }
      }
    } else {
      $problems.Add("$name : content differs after line-ending normalization")
      Write-Host "[DRIFT] $name"
      Write-Host "      live  $liveFile  normalized-sha256 $liveHash"
      Write-Host "      repo  $repoFile  normalized-sha256 $repoHash"
      $n = Write-LineDiff $liveFile $repoFile $liveText $repoText
      Write-Host "      $n differing line(s) total"
    }
    Write-Host ""
  }

  if ($problems.Count -gt 0) {
    Write-Host "RESULT: DRIFT — $($problems.Count) problem(s):" -ForegroundColor Red
    foreach ($p in $problems) { Write-Host "  - $p" -ForegroundColor Red }
    Write-Host ""
    Write-Host "Fix: run scripts/preset-drift.ps1 -Export (live -> repo) and commit the mirror."
    exit $EXIT_DRIFT
  }

  Write-Host "RESULT: in sync — $($liveDirs.Count) preset(s) match their mirrors (line-ending normalized)." -ForegroundColor Green
  exit $EXIT_OK
}

# ---------------------------------------------------------------------------
# -Export
# ---------------------------------------------------------------------------

if ($Export) {
  $copied = 0
  $updated = 0
  $removed = 0
  $skipped = New-Object System.Collections.Generic.List[string]

  $names = @($repoDirs.Keys | Where-Object { -not $liveDirs.ContainsKey($_) } | Sort-Object)
  if ($names.Count -gt 0) {
    foreach ($n in $names) { Write-Host "[WARN] repo mirror '$n' has no live counterpart — left untouched" -ForegroundColor Yellow }
    Write-Host ""
  }

  foreach ($name in ($liveDirs.Keys | Sort-Object)) {
    $liveDir = $liveDirs[$name]
    $liveFile = Join-Path $liveDir $CordisFile
    if (-not (Test-Path -LiteralPath $liveFile -PathType Leaf)) {
      Fail $EXIT_ENV "preset '$name' has no $CordisFile in $liveDir — refusing to export an incomplete preset"
    }

    $repoDir = Join-Path $AgentsDir $name
    if (-not (Test-Path -LiteralPath $repoDir)) {
      New-Item -ItemType Directory -Force -Path $repoDir | Out-Null
      Write-Host "[ NEW ] $name -> $repoDir"
    }

    Write-Host "[SYNC ] $name <- $liveDir"
    Get-ChildItem -LiteralPath $liveDir -Recurse -File -Force | Sort-Object FullName | ForEach-Object {
      $rel = $_.FullName.Substring($liveDir.Length + 1)
      if (Test-Excluded $_.Name) {
        $skipped.Add("$name/$rel")
        return
      }
      $dest = Join-Path $repoDir $rel
      $text = Get-NormalizedText $_.FullName
      $existed = Test-Path -LiteralPath $dest -PathType Leaf
      $same = $false
      if ($existed) {
        try { $same = ((Get-NormalizedText $dest) -eq $text) } catch { $same = $false }
      }
      Write-NormalizedFile $dest $text
      if (-not $existed) { $copied++ } elseif (-not $same) { $updated++ }
      Write-Host ("      {0} {1}" -f $(if (-not $existed) { "add   " } elseif ($same) { "keep  " } else { "update" }), $rel)
    }

    # Mirror semantics: repo files with no live counterpart go away (never .bak).
    Get-ChildItem -LiteralPath $repoDir -Recurse -File -Force | ForEach-Object {
      $rel = $_.FullName.Substring($repoDir.Length + 1)
      if ((Test-Excluded $_.Name)) { return }
      if (-not (Test-Path -LiteralPath (Join-Path $liveDir $rel))) {
        Remove-Item -LiteralPath $_.FullName -Force
        $removed++
        Write-Host "      remove $rel (absent from live)"
      }
    }
    Write-Host ""
  }

  if ($skipped.Count -gt 0) {
    Write-Host "skipped $($skipped.Count) backup/scratch file(s) (never exported):"
    foreach ($s in $skipped) { Write-Host "  - $s" }
    Write-Host ""
  }

  # Derived copies: use the repo's own mechanism, never hand-copy.
  $mjs = Join-Path $RepoRoot "dsh-agentflow\scripts\copy-skill.mjs"
  if (-not (Test-Path -LiteralPath $mjs -PathType Leaf)) {
    Fail $EXIT_EXPORT "copy-skill.mjs not found at $mjs"
  }
  Write-Host "==> node dsh-agentflow/scripts/copy-skill.mjs"
  & node $mjs
  if ($LASTEXITCODE -ne 0) {
    Fail $EXIT_EXPORT "copy-skill.mjs failed with exit code $LASTEXITCODE (src/lib mirrors are NOT in sync)"
  }

  Write-Host ""
  Write-Host "RESULT: exported $($liveDirs.Count) preset(s) — added $copied, updated $updated, removed $removed, skipped $($skipped.Count) backup file(s)." -ForegroundColor Green
  Write-Host "Next: git status, then commit the mirror changes."
  exit $EXIT_OK
}
