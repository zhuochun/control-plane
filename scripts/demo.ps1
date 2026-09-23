[CmdletBinding()]
param(
    [string]$Executable = (Join-Path $PSScriptRoot '..\dist\aicp.exe')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$resolvedExecutable = (Resolve-Path -LiteralPath $Executable).Path
$demoDirectory = Join-Path ([IO.Path]::GetTempPath()) ("aicp-demo-{0}" -f [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $demoDirectory | Out-Null
$requestDirectory = Join-Path $demoDirectory 'requests'
New-Item -ItemType Directory -Path $requestDirectory | Out-Null
$serverLog = Join-Path $demoDirectory 'server.log'

function Write-Request {
    param([string]$Name, [object]$Value)
    $path = Join-Path $requestDirectory ("{0}.json" -f $Name)
    $Value | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $path -Encoding UTF8
    return $path
}

function Invoke-Aicp {
    param([string[]]$CommandArguments)
    $output = & $resolvedExecutable @CommandArguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw "aicp $($CommandArguments -join ' ') failed: $output" }
    return ($output -join "`n") | ConvertFrom-Json
}

$server = Start-Process -FilePath $resolvedExecutable -ArgumentList @('serve', '--data-dir', ('"{0}"' -f $demoDirectory)) -RedirectStandardError $serverLog -WindowStyle Hidden -PassThru
try {
    $ready = $false
    foreach ($attempt in 1..50) {
        if ($server.HasExited) { throw "demo server exited early; see $serverLog" }
        try { Invoke-RestMethod -Uri 'http://127.0.0.1:7331/healthz' -TimeoutSec 1 | Out-Null; $ready = $true; break } catch { Start-Sleep -Milliseconds 100 }
    }
    if (-not $ready) { throw 'demo server did not become ready' }

    Invoke-Aicp @('config','set','Asia/Singapore','--json') | Out-Null
    $interest = Invoke-Aicp @('interest','create','--file',(Write-Request 'interest' ([ordered]@{request_id='demo-interest';title='Delivery economics';instructions_md='Notice material cost changes and explain whether periods are comparable.'})),'--json')
    $watch = Invoke-Aicp @('watch','create','--file',(Write-Request 'watch' ([ordered]@{request_id='demo-watch';interest_id=$interest.id;source=[ordered]@{kind='fixture';locator='fixtures/demo-costs'};instructions_md='Compare the same workload and flag incomplete periods.';interval_seconds=7200;lookback_seconds=604800})),'--json')

    $run1 = Invoke-Aicp @('run','start','--runner-label','offline-fixture','--json')
    $observed = '2026-09-15T02:00:00Z'
    $source = @([ordered]@{id='costs';url='https://example.com/fixtures/costs';label='Comparable cost fixture';observed_at=$observed})
    $items = @(
        [ordered]@{dedupe_key='demo:costs:comparable';expected_content_version=0;kind='report';title='Automation cost fell 18%';summary='The comparable workload fell from $100 to $82, an 18% reduction.';sources=$source;context_md='Both periods contain the same workload. Recheck after the next complete period.';report=[ordered]@{schema_version=1;body_md="## Comparable result`n`nCost fell from **`$100 to `$82**, a source-backed **18% reduction**.`n`n| Period | Cost |`n| --- | ---: |`n| Before | `$100 |`n| After | `$82 |"}},
        [ordered]@{dedupe_key='demo:costs:incomplete';expected_content_version=0;kind='note';title='Incomplete period cannot be compared';summary='Comparison is unavailable because the current fixture covers only part of the period.';sources=$source;report=[ordered]@{schema_version=1;body_md='Comparison unavailable: the periods cover different durations. Wait for a complete period.'}}
    )
    $publish1 = [ordered]@{request_id='demo-publish-1';expected_watch_revision=1;status='success';coverage=[ordered]@{cursor_before=$null;cursor_after=[ordered]@{cycle=1};observed_through=$observed;limitations=@()};items=$items}
    $result1 = Invoke-Aicp @('run','submit',$watch.id,'--file',(Write-Request 'publish-1' $publish1),'--json')
    Invoke-Aicp @('run','finish','--summary','Saved comparable and incomplete cost findings.','--json') | Out-Null

    $itemId = $result1.items[0].id
    Invoke-Aicp @('item','action',$itemId,'--file',(Write-Request 'todo' ([ordered]@{request_id='demo-todo';expected_state_version=1;action=[ordered]@{type='set_todo';state='todo'}})),'--json') | Out-Null
    Invoke-Aicp @('item','action',$itemId,'--file',(Write-Request 'reminder' ([ordered]@{request_id='demo-reminder';expected_state_version=2;action=[ordered]@{type='set_reminder';date='2099-09-16';timezone='Asia/Singapore'}})),'--json') | Out-Null

    $run2 = Invoke-Aicp @('run','start','--runner-label','offline-fixture','--watch',$watch.id,'--force','--json')
    $items[0].expected_content_version = 1
    $items[0].summary = 'The comparable workload still shows an 18% reduction; the source now marks the review complete.'
    $items[0].report.body_md = "## Reconciled result`n`nThe same workload remains **18% lower**. Source status: review complete. The local follow-up remains yours to close."
    $publish2 = [ordered]@{request_id='demo-publish-2';expected_watch_revision=1;status='success';coverage=[ordered]@{cursor_before=[ordered]@{cycle=1};cursor_after=[ordered]@{cycle=2};observed_through='2026-09-15T04:00:00Z';limitations=@()};items=@($items[0])}
    Invoke-Aicp @('run','submit',$watch.id,'--file',(Write-Request 'publish-2' $publish2),'--json') | Out-Null
    Invoke-Aicp @('run','finish','--summary','Reconciled the completed source review.','--json') | Out-Null

    $final = Invoke-Aicp @('item','get',$itemId,'--json')
    if ($final.content_version -ne 2 -or $final.state_version -ne 3 -or $final.todo_state -ne 'todo' -or $null -eq $final.remind_at) { throw 'two-cycle state preservation check failed' }
    [ordered]@{status='passed';data_directory=$demoDirectory;cycles=2;item_id=$itemId;content_version=$final.content_version;state_version=$final.state_version;todo_state=$final.todo_state;remind_at=$final.remind_at} | ConvertTo-Json
}
finally {
    if (-not $server.HasExited) { Stop-Process -Id $server.Id }
}
