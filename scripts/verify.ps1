$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Set-Location -LiteralPath $root
npm --prefix web ci
if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
npm --prefix web run typecheck
if ($LASTEXITCODE -ne 0) { throw 'web typecheck failed' }
npm --prefix web run build
if ($LASTEXITCODE -ne 0) { throw 'web build failed' }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
go test ./...
if ($LASTEXITCODE -ne 0) { throw 'go test failed' }
go build -trimpath -o dist/aicp.exe ./cmd/aicp
if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
npm --prefix web run test:e2e
if ($LASTEXITCODE -ne 0) { throw 'browser tests failed' }
& (Join-Path $root 'scripts\demo.ps1')
