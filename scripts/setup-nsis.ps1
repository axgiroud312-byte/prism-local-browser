# Portable, checksum-pinned compiler. No system installation or PATH change.
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'powershell-host.ps1')
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$root = Split-Path $PSScriptRoot -Parent
$version = '3.13'
$sha256 = 'ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8'
$zip = Join-Path $root ".tools/downloads/nsis-$version.zip"
$compiler = Join-Path $root ".tools/nsis/nsis-$version/makensis.exe"
[IO.Directory]::CreateDirectory((Split-Path $zip -Parent)) | Out-Null
if (!(Test-Path -LiteralPath $zip)) {
  $url = "https://downloads.sourceforge.net/project/nsis/NSIS%203/$version/nsis-$version.zip"
  & curl.exe --fail --location --retry 2 --output $zip $url
  if ($LASTEXITCODE) { throw 'Official NSIS download failed. No compiler executed.' }
}
if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sha256) {
  throw 'NSIS ZIP checksum mismatch. No compiler executed; retain the file for inspection.'
}
if (!(Test-Path -LiteralPath $compiler)) {
  Expand-Archive -LiteralPath $zip -DestinationPath (Join-Path $root '.tools/nsis')
}
$actual = & $compiler /VERSION
if ($LASTEXITCODE -or $actual.Trim() -ne "v$version") { throw 'NSIS compiler version mismatch.' }
Write-Output "Verified portable NSIS $version; official ZIP SHA-256 $sha256"
