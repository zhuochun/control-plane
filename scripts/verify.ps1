$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Set-Location -LiteralPath $root
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web run build
go vet ./...
go test ./...
go build -trimpath -o dist/aicp.exe ./cmd/aicp
npm --prefix web run test:e2e
& (Join-Path $root 'scripts\demo.ps1')
