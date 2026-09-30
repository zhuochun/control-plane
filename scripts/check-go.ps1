$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Set-Location -LiteralPath $root
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCommand) { throw 'Go is required; install the version pinned in go.mod.' }

$versionFile = Join-Path $root '.golangci-lint-version'
$expectedVersion = (Get-Content -LiteralPath $versionFile -Encoding UTF8 -Raw).Trim().TrimStart('v')
$lintCommand = Get-Command golangci-lint -ErrorAction SilentlyContinue
if (-not $lintCommand) {
    $goBin = (go env GOBIN).Trim()
    if (-not $goBin) { $goBin = Join-Path (go env GOPATH).Trim() 'bin' }
    $candidate = Join-Path $goBin 'golangci-lint.exe'
    if (Test-Path -LiteralPath $candidate) { $lintCommand = Get-Command $candidate }
}
if (-not $lintCommand) {
    throw "golangci-lint is required. Install the pinned v$expectedVersion release: https://golangci-lint.run/docs/welcome/install/local/"
}

$versionOutput = (& $lintCommand.Source version 2>&1 | Out-String)
if ($LASTEXITCODE -ne 0) { throw 'Could not read golangci-lint version' }
if ($versionOutput -notmatch "\b$([regex]::Escape($expectedVersion))\b") {
    throw "Expected golangci-lint v$expectedVersion, got: $versionOutput"
}

$formatDiff = & $lintCommand.Source fmt --diff 2>&1
$formatExitCode = $LASTEXITCODE
if ($formatDiff) {
    $formatDiff | Write-Output
    throw 'Go files need formatting; run golangci-lint fmt and review the result.'
}
if ($formatExitCode -ne 0) { throw 'Go formatting check failed' }

& $lintCommand.Source run ./...
if ($LASTEXITCODE -ne 0) { throw 'golangci-lint failed' }
