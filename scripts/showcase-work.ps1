[CmdletBinding()]
param(
    [ValidateSet('prepare', 'illustrate', 'inspect')]
    [string]$Phase = 'prepare',
    [string]$Server = 'http://127.0.0.1:7331',
    [string]$Executable = (Join-Path $PSScriptRoot '..\dist\aicp.exe')
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$binary = (Resolve-Path -LiteralPath $Executable).Path
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$requestRoot = Join-Path ([IO.Path]::GetTempPath()) ('aicp-showcase-work-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $requestRoot | Out-Null
function Invoke-Aicp {
    param([string[]]$Arguments)
    $output = & $binary --server $Server --json @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw ($output -join "`n") }
    return ($output -join "`n") | ConvertFrom-Json
}
function Request {
    param([string]$Name, [object]$Body)
    $path = Join-Path $requestRoot ($Name + '.json')
    $Body | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $path -Encoding UTF8
    return $path
}
try {
    $page = Invoke-Aicp @('item', 'list', '--limit', '100')
    if (@($page.items).Count -ne 7 -or @($page.items | Where-Object { $_.dedupe_key -notlike 'showcase:*' }).Count -gt 0) {
        throw 'This script requires the seven-Item fictional showcase workspace.'
    }
    $item = @($page.items | Where-Object { $_.dedupe_key -eq 'showcase:launch-review' })[0]
    if ($Phase -in @('prepare', 'inspect')) {
        $activation = @($page.items | Where-Object { $_.dedupe_key -eq 'showcase:activation' })[0]
        if ($activation.content_version -eq 1 -or $Phase -eq 'inspect') {
            $watches = Invoke-Aicp @('watch', 'list', '--all', '--limit', '100')
            Invoke-Aicp @('run', 'start', '--runner-label', 'Northstar agent loop · local Slack/email fixtures', '--watch', ($watches.items.id -join ','), '--force') | Out-Null
            foreach ($watch in $watches.items) {
                # Read each bounded fixture before reporting its coverage.
                if ($watch.slug -eq 'northstar-launch') {
                    Get-Content -LiteralPath (Join-Path $repo 'examples\showcase\slack-launch.md') -Encoding UTF8 -Raw | Out-Null
                } elseif ($watch.slug -eq 'northstar-customers') {
                    Get-Content -LiteralPath (Join-Path $repo 'examples\showcase\email-pilot.md') -Encoding UTF8 -Raw | Out-Null
                } elseif ($watch.slug -eq 'northstar-health') {
                    Get-Content -LiteralPath (Join-Path $repo 'examples\showcase\service-review.md') -Encoding UTF8 -Raw | Out-Null
                } else { throw 'Unexpected showcase source scope.' }
                $updates = @()
                if ($watch.id -eq $activation.watch_id) {
                    $metrics = Get-Content -LiteralPath (Join-Path $repo 'examples\showcase\weekly-metrics.md') -Encoding UTF8 -Raw
                    if ($activation.content_version -eq 1) { $updates = @([ordered]@{
                        dedupe_key=$activation.dedupe_key; expected_content_version=$activation.content_version
                        kind=$activation.kind; title='First-week activation reached 51%'
                        summary='Activation improved from 47% to 51%; the quarter target is 55%.'
                        interests=$activation.interests; sources=$activation.sources
                        context_md='Second agent inspection of the fictional weekly cohort fixture. Same Item; updated evidence.'
                        report=@{schema_version=1; body_md="## Activation trend`n`nActivation moved from 47% to 51%. Target: **55%**.`n`n$metrics"}
                    }) }
                }
                Invoke-Aicp @('run', 'submit', $watch.id, '--file', (Request ('coverage-' + $watch.slug) ([ordered]@{
                    request_id='showcase-loop-' + [guid]::NewGuid().ToString('N'); expected_watch_revision=$watch.revision; status='success'
                    coverage=@{cursor_before=$watch.cursor; cursor_after=@{cycle=([int]$watch.cursor.cycle + 1)}; observed_through=[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'); limitations=@('Only local fictional Slack, email, metrics, and service-review fixtures were inspected.')}
                    items=$updates
                }))) | Out-Null
            }
            $summary = if ($activation.content_version -eq 1) { 'Returning agent loop: read local source fixtures and updated the same activation Outcome from 47% to 51%; user state retained.' } else { 'Returning agent loop: re-read local source fixtures; no new evidence, stable Items and retained user state.' }
            Invoke-Aicp @('run', 'finish', '--summary', $summary) | Out-Null
        }
        if ($Phase -eq 'inspect') { Invoke-Aicp @('brief'); return }
        $item = Invoke-Aicp @('item', 'get', $item.id)
        if ($null -ne $item.PSObject.Properties['delegations'] -and @($item.delegations).Count -gt 0) {
            throw 'A delegation is already recorded. Reuse its Item and continuation rather than recreating it.'
        }
        $slack = Get-Content -LiteralPath (Join-Path $repo 'examples\showcase\slack-launch.md') -Encoding UTF8 -Raw
        $email = Get-Content -LiteralPath (Join-Path $repo 'examples\showcase\email-pilot.md') -Encoding UTF8 -Raw
        $context = "Fictional demo. Assess launch scope from these exact source fixtures. Do not contact anyone or inspect live accounts.`n`n$slack`n`n$email`n`nDeliver a version-2 report with an evidence table, Mermaid dependency diagram, and a material scope decision with a recommendation. Preserve source facts; do not imply the booked rollback is tested or that export has an owner. Leave the delegation pending for primary-agent review."
        Invoke-Aicp @('item', 'work', $item.id, '--file', (Request 'prepare-handoff' ([ordered]@{
            request_id='showcase-delegation-prepare'; expected_content_version=$item.content_version
            context_md=$context
            sources=@(
                @{id='slack-fixture'; url='https://example.com/northstar/slack/launch-readiness'; label='Slack #launch-readiness · fictional fixture'; observed_at=[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')},
                @{id='email-fixture'; url='https://example.com/northstar/email/meridian-pilot'; label='Email: Meridian pilot · fictional fixture'; observed_at=[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')}
            )
            delegations=@(@{id='launch-scope-assessment'; executor='launch-scope-researcher'; status='pending'; instructions_md='Assess the existing launch Item from its Slack and email evidence. Compare bounded pilot and broader rollout; publish a recommendation and review controls.'; context_md='Input v1 and local fixture evidence. Pending external dispatch; primary agent must save the returned continuation reference.'})
        }))) | Out-Null
    } else {
        if (@($item.delegations | Where-Object {
            $reference = $_.PSObject.Properties['external_ref']
            $null -ne $reference -and $reference.Value -like 'codex-collaboration:*'
        }).Count -gt 0) {
            throw 'This Item has a real executor reference. Reuse that handoff; illustrated replay must not replace it.'
        }
        $reportPath = (Resolve-Path -LiteralPath (Join-Path $repo 'examples\showcase\launch-report.json')).Path
        $report = Get-Content -LiteralPath $reportPath -Encoding UTF8 -Raw | ConvertFrom-Json
        Invoke-Aicp @('item', 'work', $item.id, '--file', (Request 'illustrated-result' ([ordered]@{
            request_id='showcase-illustrated-result'; expected_content_version=$item.content_version; report=$report
            delegations=@(@{id='launch-scope-assessment'; status='closed'; external_ref='local-fictional-result'; context_md='Illustrated handoff: sample executor result imported from examples/showcase/launch-report.json and checked against the local fixtures. This replay does not launch a live executor.'})
        }))) | Out-Null
    }
    Invoke-Aicp @('item', 'get', $item.id)
}
finally {
    $resolved = (Resolve-Path -LiteralPath $requestRoot).Path
    $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolved.StartsWith($temp, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unexpected request cleanup path.' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
