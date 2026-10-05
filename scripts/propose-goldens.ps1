[CmdletBinding()]
param([string]$Bundle)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$node = Get-Command node -ErrorAction Stop
$arguments = @((Join-Path $root 'scripts/testing/propose-goldens.mjs'))
if ($Bundle) { $arguments += (Resolve-Path -LiteralPath $Bundle).Path }
& $node.Source @arguments
if ($LASTEXITCODE -ne 0) { throw 'Golden proposal failed.' }
