[CmdletBinding()]
param(
    [string]$Server = 'http://127.0.0.1:7331',
    [string]$Executable = (Join-Path $PSScriptRoot '..\dist\aicp.exe')
)

# Seed a fresh, separately started workspace through the public CLI.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$resolvedExecutable = (Resolve-Path -LiteralPath $Executable).Path
$requests = Join-Path ([IO.Path]::GetTempPath()) ('aicp-showcase-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $requests | Out-Null

function Invoke-Aicp {
    param([string[]]$CommandArguments)
    $output = & $resolvedExecutable --server $Server --json @CommandArguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw "aicp failed: $($output -join "`n")" }
    return ($output -join "`n") | ConvertFrom-Json
}

function Write-Request {
    param([string]$Name, [object]$Value)
    $path = Join-Path $requests ($Name + '.json')
    $Value | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $path -Encoding UTF8
    return $path
}

function New-Finding {
    param([string]$Key, [string]$Kind, [string]$Title, [string]$Summary,
          [string]$InterestID, [string]$Reason, [string]$SourceLabel, [string]$Body)
    return [ordered]@{
        dedupe_key = "showcase:$Key"
        expected_content_version = 0
        kind = $Kind
        title = $Title
        summary = $Summary
        interests = @([ordered]@{ id = $InterestID; reason = $Reason })
        sources = @([ordered]@{
            id = $Key; url = "https://example.com/northstar/$Key"
            label = $SourceLabel; observed_at = $observed
        })
        context_md = 'Fictional Northstar team. Source links and observations are illustrative; no external systems were accessed.'
        report = [ordered]@{ schema_version = 1; body_md = $Body }
    }
}

try {
    foreach ($entity in @('interest', 'watch', 'item', 'run', 'proposal')) {
        $arguments = @($entity, 'list', '--limit', '1')
        if ($entity -in @('interest', 'watch')) { $arguments += '--all' }
        if ($entity -eq 'proposal') { $arguments += @('--state', 'all') }
        $page = Invoke-Aicp $arguments
        if (@($page.items).Count -gt 0) {
            throw 'Showcase requires an empty workspace. Start serve with a new --data-dir; existing data will not be modified.'
        }
    }
    Invoke-Aicp @('config', 'set', 'Asia/Singapore') | Out-Null
    $owner = Join-Path $requests 'USER.md'
    '# Northstar product team (fictional demo)', '',
        'I lead a small B2B product team. Track launch readiness, customer onboarding, and service reliability. Surface changes that need a decision, cite evidence, and suggest a concrete next step.' |
        Set-Content -LiteralPath $owner -Encoding UTF8
    Invoke-Aicp @('config', 'user-context', 'set', '--file', $owner) | Out-Null

    $definitions = @(
        @{ key='launch'; title='Launch readiness'; instructions='Track blockers and scope changes for the October launch. Explain impact, owner, and next decision.'; locator='Northstar Slack #launch-readiness (fictional fixture)'; cadence=3600 },
        @{ key='customers'; title='Customer onboarding'; instructions='Notice onboarding friction and repeat requests. Distinguish one account from a recurring pattern.'; locator='Northstar pilot email and activation cohorts (fictional fixtures)'; cadence=14400 },
        @{ key='health'; title='Service health & cost'; instructions='Track reliability and unit cost. Compare complete periods with the same workload and flag missing evidence.'; locator='Northstar weekly service review (fictional fixture)'; cadence=86400 }
    )
    $interests = @{}
    $watches = @{}
    foreach ($definition in $definitions) {
        $interest = Invoke-Aicp @('interest', 'create', '--file', (Write-Request ('interest-' + $definition.key) ([ordered]@{
            request_id="showcase-interest-$($definition.key)"; slug=$definition.key
            title=$definition.title; instructions_md=$definition.instructions
        })))
        $interests[$definition.key] = $interest.id
        $watch = Invoke-Aicp @('watch', 'create', '--file', (Write-Request ('watch-' + $definition.key) ([ordered]@{
            request_id="showcase-watch-$($definition.key)"; slug="northstar-$($definition.key)"
            matching_policy='explicit'; interest_ids=@($interest.id)
            source=[ordered]@{kind='fixture'; locator=$definition.locator}
            instructions_md=$definition.instructions
            interval_seconds=$definition.cadence; lookback_seconds=604800
        })))
        $watches[$definition.key] = $watch
    }
    $observed = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    $findings = @{}
    $findings.launch = @(
        (New-Finding 'launch-review' 'report' 'October launch: one decision before rollout' 'SSO is ready for pilot. Audit-log export needs an owner before the October 16 rollout.' $interests.launch 'One unresolved dependency could change the launch scope.' 'Launch review · engineering & product' @'
## Launch readiness

**Recommendation:** keep the pilot on track; resolve audit-log export ownership before expanding access.

| Workstream | Status | Next step |
| --- | --- | --- |
| Enterprise SSO | Ready for pilot | Start with three design partners |
| Audit-log export | Owner needed | Maya to confirm API ownership |
| Rollback rehearsal | Scheduled | Run the checklist on October 12 |

### Decision needed

Two pilot accounts requested audit-log export. Meridian allows a small admin-only trial while reviewing export details; broader access needs an export example and field confirmation. Engineering has not committed an owner or delivery date. Agree on scope at the launch review.

### What changed

SSO passed the staging checks. The remaining risk is the export commitment, rather than authentication readiness.
'@),
        (New-Finding 'rollback' 'task' 'Rehearse rollback before widening the pilot' 'Platform has booked October 12. Confirm the support handoff and the rollback owner.' $interests.launch 'The pilot needs a tested recovery path.' 'Platform planning · rollout checklist' '## Next step

Ask Leo to confirm the rollback owner, rehearse in staging, and attach the result to the launch checklist. The booking alone does not establish that recovery works.')
    )
    $findings.customers = @(
        (New-Finding 'onboarding' 'report' 'Three pilot teams stalled at workspace setup' 'Two teams could not find the invite step; one needed a clearer admin permissions guide.' $interests.customers 'Repeated setup friction affects pilot activation.' 'Customer success · pilot check-ins' '## Pilot feedback

Three of eight teams needed help during setup. Two missed the invite step and one misunderstood the admin role.

**Suggested follow-up:** test a clearer invite prompt with the next two teams. Keep permissions guidance separate; this small sample does not establish the frequency across all customers.'),
        (New-Finding 'activation' 'outcome' 'First-week activation reached 47%' 'Activation improved from 42% to 47%; the quarter target is 55%.' $interests.customers 'Track whether onboarding improvements lead to useful first-week use.' 'Product analytics · weekly cohort review' '## Objective: Help new teams reach first value

Activation rate moved from 42% to 47% across complete weekly cohorts. Target: **55%**.

| Period | Value |
| --- | ---: |
| Week 1 | 38% |
| Week 2 | 40% |
| Week 3 | 42% |
| Week 4 | 47% |'),
        (New-Finding 'customer-call' 'note' 'Prepare the Meridian pilot check-in' 'Meridian wants an audit-log export example before inviting its operations team.' $interests.customers 'A pilot account needs a concrete answer about launch scope.' 'Meridian · customer call notes' '## Before the next call

Bring a sample export and confirm which fields are available. If the feature is deferred, offer a revised pilot scope rather than an unconfirmed date.')
    )
    $findings.health = @(
        (New-Finding 'latency' 'report' 'Search latency returned to the service target' 'P95 fell from 820 ms to 460 ms after the index fix; error rate stayed at 0.2%.' $interests.health 'Verify recovery without hiding error-rate changes.' 'Service review · search dashboard' '## Recovery check

The latest complete 24-hour window is below the **500 ms P95 target**. Traffic volume is within 3% of the comparison window; error rate remained at 0.2%.

Keep the next full window under observation before closing the incident follow-up.'),
        (New-Finding 'unit-cost' 'outcome' 'Cost per active workspace fell to $14.90' 'The comparable weekly cost fell from $18.40 to $14.90; target is $10.00.' $interests.health 'Track unit economics as pilot usage grows.' 'Finance & platform · weekly cost review' '## Objective: Grow without scaling waste

Cost per active workspace moved from $18.40 to $14.90. Target: **$10.00**.

| Period | Value |
| --- | ---: |
| Week 1 | $21.30 |
| Week 2 | $19.80 |
| Week 3 | $18.40 |
| Week 4 | $14.90 |

Both periods include the same services and seven complete days. Lower unit cost is an observation; it does not by itself prove which change caused it.')
    )
    Invoke-Aicp @('run', 'start', '--runner-label', 'Northstar showcase (fictional)') | Out-Null
    $published = @{}
    foreach ($definition in $definitions) {
        $watch = $watches[$definition.key]
        Invoke-Aicp @('run', 'submit', $watch.id, '--file', (Write-Request ('publish-' + $definition.key) ([ordered]@{
            request_id="showcase-publish-$($definition.key)"; expected_watch_revision=$watch.revision
            status='success'
            coverage=[ordered]@{cursor_before=$null; cursor_after=@{cycle=1}; observed_through=$observed; limitations=@('Fictional demonstration data; no live source inspection.')}
            items=$findings[$definition.key]
        }))) | Out-Null
    }
    Invoke-Aicp @('run', 'finish', '--summary', 'Demo: reviewed launch readiness, pilot onboarding, and service health using fictional Northstar fixtures.') | Out-Null
    $itemPage = Invoke-Aicp @('item', 'list', '--limit', '100')
    foreach ($item in $itemPage.items) { $published[$item.dedupe_key] = $item }
    foreach ($key in @('rollback', 'customer-call', 'launch-review')) {
        $item = $published["showcase:$Key"]
        Invoke-Aicp @('item', 'action', $item.id, '--file', (Write-Request ('todo-' + $key) ([ordered]@{
            request_id="showcase-todo-$Key"; expected_state_version=$item.state_version
            action=@{type='set_todo'; state='todo'}
        }))) | Out-Null
    }
    $item = Invoke-Aicp @('item', 'get', $published['showcase:launch-review'].id)
    Invoke-Aicp @('item', 'action', $item.id, '--file', (Write-Request 'reminder' ([ordered]@{
        request_id='showcase-reminder'; expected_state_version=$item.state_version
        action=@{type='set_reminder'; date=[DateTime]::Now.AddDays(1).ToString('yyyy-MM-dd'); timezone='Asia/Singapore'}
    }))) | Out-Null
    [ordered]@{status='seeded'; items=$published.Count; interests=3; watchers=3; server=$Server} | ConvertTo-Json
}
finally {
    $resolvedRequests = (Resolve-Path -LiteralPath $requests).Path
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolvedRequests.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unexpected temporary request path.' }
    Remove-Item -LiteralPath $resolvedRequests -Recurse -Force
}
