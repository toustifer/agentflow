# Download-first install for Windows (no Go, no git clone).
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.ps1 | iex
#   $env:VERSION='v0.2.1'; .\install.ps1
#   .\install.ps1 -WriteConfig

param(
  [string]$Version = $(if ($env:VERSION) { $env:VERSION } else { "v0.2.1" }),
  [string]$Repo = "toustifer/agentflow",
  [string]$Dest = $(Join-Path $env:USERPROFILE ".claude\skills\agentflow"),
  [switch]$WriteConfig
)

$ErrorActionPreference = "Stop"
$Base = "https://github.com/$Repo/releases/download/$Version"
$BinName = "agentflow-windows-amd64.exe"
$Tmp = Join-Path $env:TEMP ("agentflow-install-" + [guid]::NewGuid().ToString("n"))
New-Item -ItemType Directory -Force -Path $Tmp | Out-Null

try {
  Write-Host "==> agentflow install $Version"
  Write-Host "    -> $Dest"

  $skillUrl = "$Base/skill.tgz"
  $binUrl = "$Base/$BinName"
  $skillPath = Join-Path $Tmp "skill.tgz"
  $binPath = Join-Path $Tmp $BinName

  Write-Host "==> download skill.tgz"
  Invoke-WebRequest -Uri $skillUrl -OutFile $skillPath -UseBasicParsing
  Write-Host "==> download $BinName"
  Invoke-WebRequest -Uri $binUrl -OutFile $binPath -UseBasicParsing

  if (Test-Path $Dest) {
    $bak = "$Dest.bak-$(Get-Date -Format yyyyMMddHHmmss)"
    Write-Host "==> backup -> $bak"
    Copy-Item -Recurse -Force $Dest $bak
  }

  New-Item -ItemType Directory -Force -Path $Dest | Out-Null
  # Expand tgz: use tar (Windows 10+)
  $extract = Join-Path $Tmp "extract"
  New-Item -ItemType Directory -Force -Path $extract | Out-Null
  tar -xzf $skillPath -C $extract
  $src = Join-Path $extract "agentflow"
  if (-not (Test-Path $src)) { throw "skill.tgz missing agentflow/ top-level dir" }

  Get-ChildItem $Dest -Force | Where-Object { $_.Name -ne "bin" } | Remove-Item -Recurse -Force
  Copy-Item -Recurse -Force (Join-Path $src "*") $Dest

  $binDir = Join-Path $Dest "bin"
  New-Item -ItemType Directory -Force -Path $binDir | Out-Null
  $absBin = Join-Path $binDir "agentflow.exe"
  Copy-Item -Force $binPath $absBin

  $modeLib = Join-Path $Dest "hooks\mode-lib.js"
  if (-not (Select-String -Path $modeLib -Pattern "MCP GATE" -Quiet)) {
    throw "installed skill missing MCP GATE"
  }

  $absInject = Join-Path $Dest "hooks\mode-inject.js"
  $absStatus = Join-Path $Dest "hooks\statusline.js"

  Write-Host "==> installed binary: $absBin"
  Write-Host "==> MCP GATE present"

  if ($WriteConfig) {
    $claudeJson = Join-Path $env:USERPROFILE ".claude.json"
    if (-not (Test-Path $claudeJson)) { "{}" | Set-Content -Path $claudeJson -Encoding utf8 }
    Copy-Item $claudeJson "$claudeJson.bak-$(Get-Date -Format yyyyMMddHHmmss)"
    $data = Get-Content $claudeJson -Raw | ConvertFrom-Json
    if (-not $data.mcpServers) {
      $data | Add-Member -NotePropertyName mcpServers -NotePropertyValue ([pscustomobject]@{}) -Force
    }
    $entry = [pscustomobject]@{
      command = $absBin
      args    = @("stdio")
      type    = "stdio"
    }
    $data.mcpServers | Add-Member -NotePropertyName agentflow -NotePropertyValue $entry -Force
    ($data | ConvertTo-Json -Depth 10) | Set-Content -Path $claudeJson -Encoding utf8
    Write-Host "==> wrote mcpServers.agentflow in $claudeJson"
  } else {
    Write-Host ""
    Write-Host "==> add to $env:USERPROFILE\.claude.json :"
    Write-Host @"
{
  "mcpServers": {
    "agentflow": {
      "command": "$($absBin -replace '\\','\\')",
      "args": ["stdio"],
      "type": "stdio"
    }
  }
}
"@
  }

  Write-Host ""
  Write-Host "==> sticky hooks — merge into $env:USERPROFILE\.claude\settings.json :"
  Write-Host @"
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "node $($absInject -replace '\\','\\')",
            "timeout": 5
          }
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "node $($absStatus -replace '\\','\\')",
    "refreshInterval": 5
  }
}
"@

  Write-Host ""
  Write-Host "Next: restart Claude Code; /mcp agentflow not failed; call mcp__agentflow__flow_ping in-session."
}
finally {
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
