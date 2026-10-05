[CmdletBinding()]
param(
    [string]$Level,
    [string]$Suite,
    [string]$Feature,
    [string]$Concern,
    [string]$Scenario,
    [string]$Profile = 'core',
    [switch]$Plan
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$node = Get-Command node -ErrorAction Stop
$arguments = @((Join-Path $root 'scripts/testing/runner.mjs'))
foreach ($name in @('Level', 'Suite', 'Feature', 'Concern', 'Scenario', 'Profile')) {
    $value = Get-Variable -Name $name -ValueOnly
    if ($value) { $arguments += @(('--' + $name.ToLowerInvariant()), $value) }
}
if ($Plan) { $arguments += '--plan' }
& $node.Source @arguments
if ($LASTEXITCODE -ne 0) { throw ('Test runner failed with exit code {0}' -f $LASTEXITCODE) }
