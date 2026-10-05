$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '../..')).Path
$lock = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'renderer.json') -Encoding UTF8 -Raw | ConvertFrom-Json
$directory = Join-Path $root 'test-output/dependencies'
New-Item -ItemType Directory -Path $directory -Force | Out-Null
$jar = Join-Path $directory ('plantuml-{0}.jar' -f $lock.version)
if (-not (Test-Path -LiteralPath $jar)) { Invoke-WebRequest -Uri $lock.url -OutFile $jar }
$actual = (Get-FileHash -LiteralPath $jar -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $lock.sha256) { throw ('PlantUML checksum mismatch: {0}' -f $jar) }
$env:AICP_TEST_PLANTUML_JAR = (Resolve-Path -LiteralPath $jar).Path
if ($env:GITHUB_ENV) { ('AICP_TEST_PLANTUML_JAR={0}' -f $env:AICP_TEST_PLANTUML_JAR) | Out-File -LiteralPath $env:GITHUB_ENV -Encoding UTF8 -Append }
Write-Output ('Provisioned checksum-verified PlantUML {0}: {1}' -f $lock.version, $env:AICP_TEST_PLANTUML_JAR)
