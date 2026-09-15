[CmdletBinding()]
param(
    [string]$ProjectDirectory = (Split-Path -Parent $PSScriptRoot),
    [string]$PromptPath = (Join-Path $PSScriptRoot 'heartbeat-prompt.md'),
    [string]$CodexPath = 'codex'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$project = (Resolve-Path -LiteralPath $ProjectDirectory).Path
$prompt = (Resolve-Path -LiteralPath $PromptPath).Path
$lockPath = Join-Path ([IO.Path]::GetTempPath()) 'aicp-heartbeat.lock'
$lock = $null
try {
    try {
        $lock = [IO.File]::Open($lockPath, 'OpenOrCreate', 'ReadWrite', 'None')
    } catch [IO.IOException] {
        Write-Output 'aicp heartbeat already running; skipped'
        exit 0
    }
    Set-Location -LiteralPath $project
    Get-Content -LiteralPath $prompt -Encoding UTF8 -Raw | & $CodexPath exec -
    exit $LASTEXITCODE
} finally {
    if ($null -ne $lock) { $lock.Dispose() }
}
