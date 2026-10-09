$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path $PSScriptRoot -Parent
$taskCache = [IO.Path]::GetFullPath((Join-Path $taskRoot '.tools/fingerprint-chromium/148.0.7778.215'))
$taskArchive = Join-Path $taskCache 'ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip'
if ((Get-FileHash -LiteralPath $taskArchive -Algorithm SHA256).Hash.ToLowerInvariant() -ne '9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579') {
  throw 'Cannot collect notices from an unverified 148 archive.'
}
$taskLicenses = @{
  'LICENSE.fingerprint-chromium.txt' = 'https://raw.githubusercontent.com/adryfish/fingerprint-chromium/148.0.7778.215/LICENSE'
  'LICENSE.chromium.txt' = 'https://raw.githubusercontent.com/chromium/chromium/148.0.7778.215/LICENSE'
}
foreach ($taskLicense in $taskLicenses.GetEnumerator()) {
  $taskLicensePath = Join-Path $taskCache $taskLicense.Key
  if (!(Test-Path -LiteralPath $taskLicensePath -PathType Leaf)) {
    Invoke-WebRequest -UseBasicParsing -Uri $taskLicense.Value -OutFile $taskLicensePath
  }
}
$taskCredits = Join-Path $taskCache 'CHROMIUM-CREDITS.html'
if (Test-Path -LiteralPath $taskCredits -PathType Leaf) {
  $taskCreditsText = [IO.File]::ReadAllText($taskCredits)
  if ($taskCreditsText.Length -gt 100000 -and $taskCreditsText.Contains('Redistribution and use')) { return }
  throw 'The cached Chromium credits do not contain the complete component notices.'
}
$taskExtraction = Join-Path $taskCache ('notices-runtime-' + [guid]::NewGuid().ToString())
$taskProcess = $null
try {
  Expand-Archive -LiteralPath $taskArchive -DestinationPath $taskExtraction
  $taskChrome = Join-Path $taskExtraction 'ungoogled-chromium_148.0.7778.215-1.1_windows_x64/chrome.exe'
  if ((Get-FileHash -LiteralPath $taskChrome -Algorithm SHA256).Hash.ToLowerInvariant() -ne '1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915') {
    throw 'The exact 148 credits runtime checksum does not match.'
  }
  $taskProfile = Join-Path $taskExtraction 'credits-profile'
  $taskArguments = @('--headless=new','--allow-chrome-scheme-url','--no-first-run','--no-default-browser-check','--disable-background-networking','--disable-component-update','--disable-sync','--proxy-server=http://127.0.0.1:1',('--user-data-dir="' + $taskProfile + '"'),'--dump-dom','chrome://credits')
  $taskProcess = Start-Process -FilePath $taskChrome -ArgumentList $taskArguments -WindowStyle Hidden -RedirectStandardOutput $taskCredits -RedirectStandardError (Join-Path $taskExtraction 'credits-errors.log') -PassThru
  if (!$taskProcess.WaitForExit(60000)) { throw 'The 148 component notice export timed out.' }
  $taskCreditsText = [IO.File]::ReadAllText($taskCredits)
  if ($taskProcess.ExitCode -ne 0 -or $taskCreditsText.Length -lt 100000 -or !$taskCreditsText.Contains('Redistribution and use')) {
    throw 'The 148 component notice export did not produce complete credits.'
  }
} finally {
  if ($taskProcess -and !$taskProcess.HasExited) { Stop-Process -Id $taskProcess.Id -Force }
  $taskResolvedExtraction = [IO.Path]::GetFullPath($taskExtraction)
  if (!$taskResolvedExtraction.StartsWith($taskCache + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or (Split-Path $taskResolvedExtraction -Leaf) -notmatch '^notices-runtime-[0-9a-f-]{36}$') {
    throw 'Unexpected temporary credits runtime path; cleanup refused.'
  }
  if (Test-Path -LiteralPath $taskResolvedExtraction) {
    Remove-Item -LiteralPath $taskResolvedExtraction -Recurse -Force
  }
}
