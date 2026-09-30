# Test-host provisioning only; the product installer NEVER runs this script.
$ErrorActionPreference='Stop'
if ($env:GITHUB_ACTIONS -ne 'true') { throw 'This runtime provisioning script is only for disposable GitHub Windows runners.' }
$root=Split-Path $PSScriptRoot -Parent
$helper=Join-Path $root 'build/bin/prism-maintenance.exe'
$check=Start-Process -FilePath $helper -ArgumentList '--check-runtime' -PassThru -Wait
if($check.ExitCode -eq 0){Write-Output 'Runner already has compatible WebView2.';exit 0}
if($check.ExitCode -ne 20){throw 'Runner prerequisite detector did not execute correctly.'}
$setup=Join-Path $root '.tools/downloads/MicrosoftEdgeWebview2Setup.exe'
& curl.exe --fail --location --retry 2 --output $setup 'https://go.microsoft.com/fwlink/p/?LinkId=2124703'
if($LASTEXITCODE){throw 'Official Microsoft runtime bootstrapper download failed.'}
$signature=Get-AuthenticodeSignature -FilePath $setup
if($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation'){throw 'Runtime bootstrapper signature is not valid Microsoft code.'}
$install=Start-Process -FilePath $setup -ArgumentList '/silent /install' -PassThru -Wait
if($install.ExitCode -ne 0){throw 'Disposable runner runtime installation failed.'}
$check=Start-Process -FilePath $helper -ArgumentList '--check-runtime' -PassThru -Wait
if($check.ExitCode -ne 0){throw 'Installed runner runtime still fails actual WebView2 detection.'}
Write-Output 'Compatible WebView2 provisioned on the disposable test host from signed Microsoft bootstrapper.'
