param([switch]$InspectOnly)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path $PSScriptRoot -Parent
$taskCache = Join-Path $taskRoot '.tools/fingerprint-chromium/148.0.7778.215'
$taskName = 'ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip'
$taskArchive = Join-Path $taskCache $taskName
$taskChecksum = '9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579'
if (!(Test-Path -LiteralPath $taskArchive)) {
  & (Join-Path $PSScriptRoot 'setup-kernel-fixture.ps1')
}
if ((Get-FileHash -LiteralPath $taskArchive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $taskChecksum) {
  throw 'The bundled 148 archive checksum does not match the pinned official build.'
}
& (Join-Path $PSScriptRoot 'collect-kernel148-notices.ps1')
foreach ($taskNotice in @('LICENSE.fingerprint-chromium.txt','LICENSE.chromium.txt','CHROMIUM-CREDITS.html')) {
  if (!(Test-Path -LiteralPath (Join-Path $taskCache $taskNotice) -PathType Leaf)) {
    throw "The exact 148 distribution notice is missing: $taskNotice"
  }
}
if ($InspectOnly) { return }
$taskDestination = Join-Path $taskRoot 'build/bin/bundled'
New-Item -ItemType Directory -Path $taskDestination -Force | Out-Null
Copy-Item -LiteralPath $taskArchive -Destination (Join-Path $taskDestination $taskName)
foreach ($taskNotice in @('LICENSE.fingerprint-chromium.txt','LICENSE.chromium.txt','CHROMIUM-CREDITS.html')) {
  Copy-Item -LiteralPath (Join-Path $taskCache $taskNotice) -Destination (Join-Path $taskDestination $taskNotice)
}
$taskManifest = [ordered]@{
  version = '148.0.7778.215'
  architecture = 'windows-x64'
  archive = $taskName
  sha256 = $taskChecksum
  source = 'https://github.com/adryfish/fingerprint-chromium/releases/download/148.0.7778.215/ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip'
  notices = @('LICENSE.fingerprint-chromium.txt','LICENSE.chromium.txt','CHROMIUM-CREDITS.html')
}
[IO.File]::WriteAllText((Join-Path $taskDestination 'manifest.json'), ($taskManifest | ConvertTo-Json -Depth 3), [Text.UTF8Encoding]::new($false))
Write-Output 'Bundled the verified 148.0.7778.215 ZIP and its distribution notices.'
