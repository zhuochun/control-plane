[CmdletBinding()]
param(
    [string]$Archive
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($Archive)) {
    $Archive = (Get-ChildItem -LiteralPath (Join-Path $root 'dist') -Filter '*windows_amd64.zip' | Select-Object -First 1).FullName
}
$archivePath = (Resolve-Path -LiteralPath $Archive).Path
$checksumPath = Join-Path (Split-Path -Parent $archivePath) 'checksums.txt'
$expectedLine = Get-Content -LiteralPath $checksumPath -Encoding UTF8 | Where-Object { $_ -match ('  ' + [regex]::Escape((Split-Path -Leaf $archivePath)) + '$') }
if ($null -eq $expectedLine) { throw 'archive is missing from checksums.txt' }
$expectedHash = ($expectedLine -split '  ')[0].ToLowerInvariant()
$actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualHash -ne $expectedHash) { throw 'archive checksum does not match' }
$smokeDirectory = Join-Path ([IO.Path]::GetTempPath()) ('aicp-package-smoke-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $smokeDirectory | Out-Null
Expand-Archive -LiteralPath $archivePath -DestinationPath $smokeDirectory
$executable = (Resolve-Path -LiteralPath (Join-Path $smokeDirectory 'aicp.exe')).Path
& $executable version
if ($LASTEXITCODE -ne 0) { throw 'packaged executable did not start' }

$dataDirectory = Join-Path $smokeDirectory 'data'
$serverLog = Join-Path $smokeDirectory 'server.log'
$server = Start-Process -FilePath $executable -ArgumentList @('serve', '--data-dir', ('"{0}"' -f $dataDirectory)) -RedirectStandardError $serverLog -WindowStyle Hidden -PassThru
try {
    $ready = $false
    foreach ($attempt in 1..50) {
        if ($server.HasExited) { throw "packaged server exited early; see $serverLog" }
        try {
            Invoke-RestMethod -Uri 'http://127.0.0.1:7331/healthz' -TimeoutSec 1 | Out-Null
            $ready = $true
            break
        } catch {
            Start-Sleep -Milliseconds 100
        }
    }
    if (-not $ready) { throw 'packaged server did not become ready' }
    $page = Invoke-WebRequest -Uri 'http://127.0.0.1:7331/items/deep-link' -UseBasicParsing
    if ($page.StatusCode -ne 200 -or $page.Content -notmatch '<div id="root"></div>') {
        throw 'embedded portal is missing from packaged executable'
    }
    [ordered]@{status='passed'; archive=$archivePath; smoke_directory=$smokeDirectory} | ConvertTo-Json
} finally {
    if (-not $server.HasExited) { Stop-Process -Id $server.Id }
}
