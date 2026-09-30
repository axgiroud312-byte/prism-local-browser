param([ValidateRange(1,65535)][int]$PreviewRevision = 1)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'powershell-host.ps1')
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$root = Split-Path $PSScriptRoot -Parent
$version = "0.3.0-preview.$PreviewRevision"
$encoding = [Text.UTF8Encoding]::new($false)
function Read-PEArchitecture([string]$Path) {
  $reader=[IO.BinaryReader]::new([IO.File]::OpenRead($Path))
  try {
    $reader.BaseStream.Position=0x3c
    $offset=$reader.ReadInt32()
    $reader.BaseStream.Position=$offset
    if($reader.ReadUInt32() -ne 0x00004550){throw 'Invalid PE signature.'}
    switch($reader.ReadUInt16()) {0x8664{return 'amd64'};0x14c{return 'i386'};default{throw 'Unexpected PE architecture.'}}
  } finally {$reader.Dispose()}
}
function Read-PEVersion([string]$Path) {
  # Go's fixed VERSIONINFO may omit the optional FileVersion string table.
  $info=(Get-Item -LiteralPath $Path).VersionInfo
  if(!$info.FileMajorPart -and !$info.FileMinorPart -and !$info.FileBuildPart -and !$info.FilePrivatePart){return $null}
  return "$($info.FileMajorPart).$($info.FileMinorPart).$($info.FileBuildPart).$($info.FilePrivatePart)"
}
& (Join-Path $PSScriptRoot 'setup-nsis.ps1')
& (Join-Path $PSScriptRoot 'desktop.ps1') -Action build -PreviewRevision $PreviewRevision
$payload = Join-Path $root 'build/bin'
$output = Join-Path $root 'build/releases'
[IO.Directory]::CreateDirectory($output) | Out-Null
Copy-Item (Join-Path $root '.tools/nsis/nsis-3.13/COPYING') (Join-Path $payload 'NSIS-LICENSE.txt')
Copy-Item (Join-Path $root 'docs/INSTALLATION.md') (Join-Path $payload 'INSTALLATION.md')
$commit = (& git -C $root rev-parse HEAD).Trim()
$dirty = [bool](& git -C $root status --porcelain)
$manifest = @{ version=$version; windowsPEVersion=(Read-PEVersion (Join-Path $payload 'prism-browser.exe')); architecture='amd64'; channel='development-preview'; sourceCommit=$commit; sourceDirty=$dirty; builtAt=[DateTime]::UtcNow.ToString('o'); nsisVersion='3.13'; kernelIncluded=$false; dataRoot='%LOCALAPPDATA%/PrismBrowser'; minimumWebView2='94.0.992.31'; files=@() }
foreach ($file in @('prism-browser.exe','prism-maintenance.exe','LICENSE','THIRD_PARTY_NOTICES.md','GO-THIRD-PARTY-NOTICES.txt','NSIS-LICENSE.txt','INSTALLATION.md')) {
  $path=Join-Path $payload $file
  $signature=if($file.EndsWith('.exe')){(Get-AuthenticodeSignature -FilePath $path).Status.ToString()}else{'not-code'}
  $entry=@{ name=$file; sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant(); authenticode=$signature; bytes=(Get-Item -LiteralPath $path).Length }
  if($file.EndsWith('.exe')){$entry.peArchitecture=Read-PEArchitecture $path;$entry.windowsPEVersion=Read-PEVersion $path}
  $manifest.files += $entry
}
[IO.File]::WriteAllText((Join-Path $payload 'release.json'),($manifest | ConvertTo-Json -Depth 8),$encoding)
$installer=Join-Path $output "prism-browser-$version-windows-amd64-setup.exe"
$compiler=Join-Path $root '.tools/nsis/nsis-3.13/makensis.exe'
& $compiler /V2 /INPUTCHARSET UTF8 "/DPAYLOAD_DIR=$payload" "/DOUTPUT_FILE=$installer" "/DRELEASE_VERSION=$version" "/DPREVIEW_REVISION=$PreviewRevision" (Join-Path $root 'build/windows/installer/prism.nsi')
if ($LASTEXITCODE) { throw 'NSIS installer build failed.' }
$manifest.installer=@{ name=(Split-Path $installer -Leaf); sha256=(Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLowerInvariant(); authenticode=(Get-AuthenticodeSignature -FilePath $installer).Status.ToString(); bytes=(Get-Item -LiteralPath $installer).Length; windowsPEVersion=(Read-PEVersion $installer); peArchitecture=(Read-PEArchitecture $installer) }
[IO.File]::WriteAllText((Join-Path $output "prism-browser-$version-release.json"),($manifest | ConvertTo-Json -Depth 8),$encoding)
[IO.File]::WriteAllText((Join-Path $output "prism-browser-$version-SHA256SUMS.txt"),"$($manifest.installer.sha256)  $($manifest.installer.name)`n",$encoding)
Write-Output "Built $version Windows amd64 installer. Signature: $($manifest.installer.authenticode). No real browser kernel included."
