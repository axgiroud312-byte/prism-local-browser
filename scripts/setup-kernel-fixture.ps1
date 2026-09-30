# Explicit test fixture version, not a product default or automatic fallback.
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$directory=Join-Path $root '.tools/fingerprint-chromium/148.0.7778.215'
$archive=Join-Path $directory 'ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip'
$expected='9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579'
New-Item -ItemType Directory -Force -Path $directory | Out-Null
if(!(Test-Path -LiteralPath $archive)){
  $temporary="$archive.download"
  & curl.exe --fail --location --silent --show-error --retry 2 --output $temporary 'https://github.com/adryfish/fingerprint-chromium/releases/download/148.0.7778.215/ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip'
  if($LASTEXITCODE){throw 'The exact selected real test ZIP could not be obtained; no fallback version was selected.'}
  if((Get-FileHash -LiteralPath $temporary -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected){throw 'Downloaded test ZIP digest mismatch; do not execute.'}
  Move-Item -LiteralPath $temporary -Destination $archive
}
if((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected){throw 'Existing selected test ZIP digest mismatch; not replaced or executed.'}
Write-Output "PASS: explicit 148.0.7778.215 real fixture SHA-256 $expected"
