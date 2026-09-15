[CmdletBinding()]
param(
    [ValidateSet('baseline', 'update')]
    [string]$Phase = 'baseline',
    [string]$Server = 'http://127.0.0.1:7331',
    [string]$Executable = (Join-Path $PSScriptRoot '..\dist\aicp.exe')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$resolvedExecutable = (Resolve-Path -LiteralPath $Executable).Path
$serverUrl = $Server.TrimEnd('/')
$simulationDirectory = Join-Path ([IO.Path]::GetTempPath()) ("aicp-okr-{0}" -f [guid]::NewGuid().ToString('N'))
$requestDirectory = Join-Path $simulationDirectory 'requests'
New-Item -ItemType Directory -Path $requestDirectory -Force | Out-Null

function Write-Request {
    param(
        [string]$Name,
        [object]$Value
    )
    $path = Join-Path $requestDirectory ("{0}.json" -f $Name)
    $Value | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $path -Encoding UTF8
    return $path
}

function Invoke-Aicp {
    param([string[]]$CommandArguments)
    $output = & $resolvedExecutable '--server' $serverUrl '--json' @CommandArguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "aicp $($CommandArguments -join ' ') failed: $($output -join "`n")"
    }
    return ($output -join "`n") | ConvertFrom-Json
}

function New-RequestID {
    param([string]$Label)
    return "okr-simulation-$Label-$([guid]::NewGuid().ToString('N'))"
}

$interestTitle = 'OKR simulation'
$watchLocator = 'fixtures/okr-simulation'
$metrics = @(
    [ordered]@{
        key = 'simulation:okr:activation'
        title = 'Activation rate'
        objective = 'Objective: Make new workspaces successful in their first week'
        label = 'Activation rate'
        baseline_previous = '42%'
        baseline_current = '47%'
        baseline_series = @('38%', '40%', '42%', '47%')
        update_previous = '47%'
        update_current = '51%'
        update_series = @('42%', '45%', '47%', '51%')
        target = '55%'
    },
    [ordered]@{
        key = 'simulation:okr:first-value'
        title = 'Time to first value'
        objective = 'Objective: Help new teams reach a useful first result sooner'
        label = 'Median time to first value'
        baseline_previous = '3.8 days'
        baseline_current = '2.6 days'
        baseline_series = @('4.6 days', '4.2 days', '3.8 days', '2.6 days')
        update_previous = '2.6 days'
        update_current = '2.1 days'
        update_series = @('3.8 days', '3.2 days', '2.6 days', '2.1 days')
        target = '1.5 days'
    },
    [ordered]@{
        key = 'simulation:okr:active-teams'
        title = 'Weekly active teams'
        objective = 'Objective: Grow sustained use of the control plane'
        label = 'Weekly active teams'
        baseline_previous = '18 teams'
        baseline_current = '27 teams'
        baseline_series = @('14 teams', '16 teams', '18 teams', '27 teams')
        update_previous = '27 teams'
        update_current = '34 teams'
        update_series = @('18 teams', '23 teams', '27 teams', '34 teams')
        target = '40 teams'
    },
    [ordered]@{
        key = 'simulation:okr:error-free'
        title = 'Error-free session rate'
        objective = 'Objective: Make everyday agent work feel dependable'
        label = 'Error-free session rate'
        baseline_previous = '97.6%'
        baseline_current = '98.4%'
        baseline_series = @('96.8%', '97.1%', '97.6%', '98.4%')
        update_previous = '98.4%'
        update_current = '99.1%'
        update_series = @('97.6%', '98.0%', '98.4%', '99.1%')
        target = '99.5%'
    },
    [ordered]@{
        key = 'simulation:okr:cost-to-serve'
        title = 'Cost to serve'
        objective = 'Objective: Scale useful work without scaling waste'
        label = 'Cost to serve per active team'
        baseline_previous = '$18.40'
        baseline_current = '$14.90'
        baseline_series = @('$21.30', '$19.80', '$18.40', '$14.90')
        update_previous = '$14.90'
        update_current = '$12.80'
        update_series = @('$18.40', '$16.20', '$14.90', '$12.80')
        target = '$10.00'
    }
)

function New-MetricItem {
    param(
        [System.Collections.IDictionary]$Metric,
        [object]$Existing,
        [string]$CurrentPhase,
        [string]$ObservedThrough
    )
    $isBaseline = $CurrentPhase -eq 'baseline'
    $previous = if ($isBaseline) { $Metric.baseline_previous } else { $Metric.update_previous }
    $current = if ($isBaseline) { $Metric.baseline_current } else { $Metric.update_current }
    $values = if ($isBaseline) { @($Metric.baseline_series) } else { @($Metric.update_series) }
    $expectedVersion = if ($isBaseline) { 0 } else { [int64]$Existing.content_version }
    $summary = '{0} moved from {1} to {2}; target is {3}.' -f $Metric.label, $previous, $current, $Metric.target
    $tableRows = for ($index = 0; $index -lt $values.Count; $index++) {
        '| Week {0} | {1} |' -f ($index + 1), $values[$index]
    }
    $table = $tableRows -join "`n"
    $body = @"
## $($Metric.objective)

Metric attention: $($Metric.label)

$summary

Target: **$($Metric.target)**.

| Period | Value |
| --- | ---: |
$table
"@
    [ordered]@{
        dedupe_key = $Metric.key
        expected_content_version = $expectedVersion
        kind = 'outcome'
        title = $Metric.title
        summary = $summary
        sources = @([ordered]@{
            id = 'okr-fixture'
            url = 'https://example.com/fixtures/okr-simulation'
            label = 'OKR simulation fixture'
            observed_at = $ObservedThrough
        })
        context_md = "Fixture cycle: $CurrentPhase. $($Metric.objective). Values are simulated observations; no external source was fetched."
        report = [ordered]@{
            schema_version = 1
            body_md = $body
        }
    }
}

try {
    $interestPage = Invoke-Aicp @('interest', 'list', '--limit', '100')
    $interest = @($interestPage.items) | Where-Object { $_.title -eq $interestTitle } | Select-Object -First 1
    if (-not $interest) {
        $interest = Invoke-Aicp @('interest', 'create', '--file', (Write-Request 'interest-create' ([ordered]@{
            request_id = New-RequestID 'interest'
            title = $interestTitle
            instructions_md = 'Compare the simulated OKR periods and retain the latest evidence for each key result.'
        })))
    }

    $watchPage = Invoke-Aicp @('watch', 'list', '--limit', '100')
    $watch = @($watchPage.items) | Where-Object {
        $_.interest_id -eq $interest.id -and $_.source.locator -eq $watchLocator
    } | Select-Object -First 1
    if (-not $watch) {
        $watch = Invoke-Aicp @('watch', 'create', '--file', (Write-Request 'watch-create' ([ordered]@{
            request_id = New-RequestID 'watch'
            interest_id = $interest.id
            source = [ordered]@{ kind = 'fixture'; locator = $watchLocator }
            instructions_md = 'Compare each OKR metric with its previous period and target. Preserve the direction and unit.'
            interval_seconds = 7200
            lookback_seconds = 604800
        })))
    }

    $itemPage = Invoke-Aicp @('item', 'list', '--limit', '100')
    $existingItems = @($itemPage.items | Where-Object {
        $null -ne $_ -and $_.dedupe_key -like 'simulation:okr:*'
    })
    $existingByKey = @{}
    foreach ($item in $existingItems) {
        $existingByKey[$item.dedupe_key] = $item
    }

    if ($Phase -eq 'baseline' -and $existingItems.Count -gt 0) {
        throw 'The OKR simulation already has items. Run -Phase update to simulate the next cycle.'
    }
    if ($Phase -eq 'update') {
        $missing = @($metrics | Where-Object { -not $existingByKey.ContainsKey($_.key) })
        if ($missing.Count -gt 0) {
            throw "Cannot update the OKR simulation; baseline items are missing: $($missing.key -join ', ')"
        }
    }

    $observedThrough = if ($Phase -eq 'baseline') { '2026-09-15T08:00:00Z' } else { '2026-09-16T08:00:00Z' }
    $cycle = if ($Phase -eq 'baseline') { 1 } else { 2 }
    $items = @($metrics | ForEach-Object {
        $existing = if ($existingByKey.ContainsKey($_.key)) { $existingByKey[$_.key] } else { $null }
        New-MetricItem -Metric $_ -Existing $existing -CurrentPhase $Phase -ObservedThrough $observedThrough
    })
    $run = Invoke-Aicp @('run', 'start', '--file', (Write-Request 'run-start' ([ordered]@{
        request_id = New-RequestID "run-$Phase"
        runner_label = 'okr-simulation'
        watch_ids = @($watch.id)
        force = $true
    })))
    $publish = Invoke-Aicp @('run', 'publish', $run.run.id, $watch.id, '--file', (Write-Request 'run-publish' ([ordered]@{
        request_id = New-RequestID "publish-$Phase"
        expected_watch_revision = [int64]$watch.revision
        status = 'success'
        coverage = [ordered]@{
            cursor_before = $watch.cursor
            cursor_after = [ordered]@{ cycle = $cycle }
            observed_through = $observedThrough
            limitations = @()
        }
        items = $items
    })))
    Invoke-Aicp @('run', 'finish', $run.run.id, '--file', (Write-Request 'run-finish' ([ordered]@{
        request_id = New-RequestID "finish-$Phase"
        summary = "Published the $Phase OKR simulation cycle for $($metrics.Count) metrics."
    }))) | Out-Null

    [ordered]@{
        status = 'passed'
        phase = $Phase
        metrics = $metrics.Count
        item_ids = @($publish.items | ForEach-Object { $_.id })
        content_versions = @($publish.items | ForEach-Object { $_.content_version })
        server = $serverUrl
    } | ConvertTo-Json -Depth 8
}
finally {
    if (Test-Path -LiteralPath $simulationDirectory) {
        Remove-Item -LiteralPath $simulationDirectory -Recurse -Force
    }
}
