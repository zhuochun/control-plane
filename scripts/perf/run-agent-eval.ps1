param(
    [string]$OutputRoot = (Join-Path ([IO.Path]::GetTempPath()) ('aicp-agent-eval-' + [guid]::NewGuid().ToString('N')))
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
$go = Get-Command go -ErrorAction Stop
$null = Get-Command node -ErrorAction Stop
if (Test-Path -LiteralPath $OutputRoot) {
    throw "Output root already exists: $OutputRoot"
}
$outputRootPath = [IO.Path]::GetFullPath($OutputRoot)
$null = New-Item -Path $outputRootPath -ItemType Directory
$cliBinary = Join-Path $outputRootPath 'aicp.exe'

$cases = @(
    @{ Name = 'mature-1-quiet'; Profile = 'mature'; Attention = 1; Mode = 'quiet' },
    @{ Name = 'mature-20-quiet'; Profile = 'mature'; Attention = 20; Mode = 'quiet' },
    @{ Name = 'mature-20-backlog'; Profile = 'mature'; Attention = 20; Mode = 'backlog' },
    @{ Name = 'fresh-reconcile'; Profile = 'fresh'; Attention = 1; Mode = 'reconcile' },
    @{ Name = 'fresh-multi-interest'; Profile = 'fresh'; Attention = 1; Mode = 'multi-interest' },
    @{ Name = 'fresh-partial-retry'; Profile = 'fresh'; Attention = 1; Mode = 'partial-retry' }
)

Push-Location -LiteralPath $repoRoot
try {
    & $go.Source build -o $cliBinary ./cmd/aicp
    if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' }
    foreach ($case in $cases) {
        $dataDir = Join-Path $outputRootPath $case.Name
        $output = Join-Path $outputRootPath ($case.Name + '.json')
        & $go.Source run ./scripts/perf/seed.go -data-dir $dataDir -profile $case.Profile -attention-percent $case.Attention
        if ($LASTEXITCODE -ne 0) { throw "Fixture seed failed: $($case.Name)" }
        & $go.Source run ./scripts/perf/agent-eval -data-dir $dataDir -mode $case.Mode -output $output
        if ($LASTEXITCODE -ne 0) { throw "Evaluation failed: $($case.Name)" }
    }
    foreach ($format in @('json', 'default')) {
        $name = 'mature-20-backlog-cli-' + $format
        $dataDir = Join-Path $outputRootPath $name
        $output = Join-Path $outputRootPath ($name + '.json')
        & $go.Source run ./scripts/perf/seed.go -data-dir $dataDir -profile mature -attention-percent 20
        if ($LASTEXITCODE -ne 0) { throw "Fixture seed failed: $name" }
        & $go.Source run ./scripts/perf/cli-eval -data-dir $dataDir -mode backlog -format $format -binary $cliBinary -output $output
        if ($LASTEXITCODE -ne 0) { throw "CLI evaluation failed: $name" }
    }
} finally {
    Pop-Location
}

Write-Output "Deterministic results: $outputRootPath"
