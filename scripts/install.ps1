#requires -version 5.1
<#
.SYNOPSIS
    Install cc-probeline on Windows (PowerShell 5.1+).

.DESCRIPTION
    Copies the cc-probeline binary to the destination path, optionally merges
    the statusLine block into ~/.claude/settings.json via the binary itself
    (Q7=a: cc-probeline install --merge-settings), and warns when the
    destination directory is not in PATH.

    Settings JSON is handled natively by the binary; ConvertFrom-Json /
    ConvertTo-Json (available in PowerShell 5.1+) are available as a fallback
    if the binary-driven path is unavailable.

.PARAMETER Dest
    Full path for the installed binary.
    Default: $env:LOCALAPPDATA\Programs\cc-probeline\cc-probeline.exe

.PARAMETER NoSettings
    Skip the settings merge step (--merge-settings).

.PARAMETER Force
    Pass --force to the binary settings merge command.

.PARAMETER RefreshInterval
    Refresh interval in seconds passed to --merge-settings. Default: 5.

.EXAMPLE
    .\install.ps1
    .\install.ps1 -Dest "$env:LOCALAPPDATA\Programs\cc-probeline\cc-probeline.exe"
    .\install.ps1 -NoSettings
    .\install.ps1 -Force -RefreshInterval 10
#>
[CmdletBinding()]
param(
    [string]$Dest = "$env:LOCALAPPDATA\Programs\cc-probeline\cc-probeline.exe",
    [switch]$NoSettings,
    [switch]$Force,
    [int]$RefreshInterval = 5
)

$ErrorActionPreference = 'Stop'

# 1. Arch detection (MVP: AMD64 only, Windows ARM deferred to Phase 7)
if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
    Write-Error "Unsupported arch: $env:PROCESSOR_ARCHITECTURE (MVP: AMD64 only, Windows ARM deferred to Phase 7)"
    exit 1
}

# 2. Locate the binary relative to this script.
#    PS 5.1 quirk: $PSScriptRoot is empty outside functions; use
#    $MyInvocation.MyCommand.Path instead.
$selfDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projDir = (Resolve-Path (Join-Path $selfDir '..')).Path

$src = Join-Path $projDir 'cc-probeline.exe'
if (-not (Test-Path $src)) {
    $src = Join-Path $projDir 'cc-probeline-windows-amd64.exe'
}
if (-not (Test-Path $src)) {
    Write-Error "Binary not found near $selfDir. Build with: go build -o cc-probeline.exe .\cmd\cc-probeline\"
    exit 1
}

# 3. Create destination directory and copy atomically.
#    Split-Path / Move-Item handle paths with spaces correctly in PowerShell.
$destDir = Split-Path -Parent $Dest
if (-not (Test-Path $destDir)) {
    New-Item -ItemType Directory -Path $destDir | Out-Null
}

$tmp = "$Dest.tmp"
Copy-Item -Force $src $tmp
Move-Item -Force $tmp $Dest

# 4. Verify the binary executes correctly.
& $Dest --version | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Error "Binary verification failed (--version exited $LASTEXITCODE)"
    exit 1
}

# 5. PATH check (warn-only; does not abort install).
#    Compares case-insensitively to tolerate Windows path casing variants.
$pathDirs = $env:Path -split ';' | ForEach-Object { $_.TrimEnd('\') }
$destDirNorm = $destDir.TrimEnd('\')
$inPath = $pathDirs | Where-Object { $_ -ieq $destDirNorm }
if (-not $inPath) {
    Write-Host "Note: $destDir is not in PATH."
    Write-Host "      Add it permanently: setx PATH `"$destDir;%PATH%`""
}

# 6. Settings merge via binary (Q7=a).
#    The binary handles JSON natively (Go); ConvertFrom-Json / ConvertTo-Json
#    (PowerShell 5.1+ builtins) serve as documentation and optional fallback.
if (-not $NoSettings) {
    $mergeArgs = @(
        'install',
        '--merge-settings',
        '--binary-path', $Dest,
        '--refresh-interval', [string]$RefreshInterval
    )
    if ($Force) {
        $mergeArgs += '--force'
    }
    & $Dest @mergeArgs
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}

# 7. Smoke check — send a minimal JSON payload to the binary via stdin.
$payload = '{"transcript_path":"NUL","session_id":"00000000-0000-0000-0000-000000000000","model":{"id":"claude-3-5-sonnet"},"cwd":"."}'
$payload | & $Dest | Out-Null

Write-Host "cc-probeline: installed at $Dest"
