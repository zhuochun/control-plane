$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Set-Location -LiteralPath $root
# Complete core gate; the shared runner verifies locked dependency/build identity
# and includes the two-cycle demo journey. Release additionally requires renderer
# and host packaging evidence in the existing release jobs.
& (Join-Path $root 'scripts/test.ps1') -Suite main
