param([switch]$SilentWizard, [switch]$NoClicks, [ValidateRange(1,65535)][int]$FirstRevision = 1, [ValidateRange(1,65535)][int]$SecondRevision = 2, [switch]$Candidate)
# Uses actual default Windows known folders. Refuses existing installation/user data.
# No test override of LOCALAPPDATA or PRISM_WORKSPACE_ROOT; every side effect is on this fresh product installation.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$scriptsRoot=Split-Path $env:PRISM_INSTALLER_VERIFY -Parent
. (Join-Path $scriptsRoot 'powershell-host.ps1')
$root=Split-Path $scriptsRoot -Parent
if ($NoClicks) { $SilentWizard = $true }
if ($SecondRevision -le $FirstRevision) { throw 'Upgrade verification requires a strictly newer second revision.' }
$local=[Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
$data=Join-Path $local 'PrismBrowser'
$install=Join-Path $local 'Programs/PrismBrowserPreview'
$registry='HKCU:/Software/Microsoft/Windows/CurrentVersion/Uninstall/PrismBrowserPreview'
$evidence=Join-Path $root 'output/goal/T03'
if ($NoClicks) { $evidence=Join-Path $root 'output/goal/V1-final/install-no-clicks' }
$steps=[Collections.Generic.List[object]]::new()
$encoding=[Text.UTF8Encoding]::new($false)
$releaseDirectory=Join-Path $root 'build/releases'
if ($Candidate) { $releaseDirectory=Join-Path $releaseDirectory 'v1-candidate' }
$installer1=Join-Path $releaseDirectory "prism-browser-0.3.0-preview.$FirstRevision-windows-amd64-setup.exe"
$installer2=Join-Path $releaseDirectory "prism-browser-0.3.0-preview.$SecondRevision-windows-amd64-setup.exe"
if (Test-Path $data) { throw 'Existing default Prism data detected. Refusing to modify/delete it. Use a fresh Windows user or VM.' }
if ((Test-Path $install) -or (Test-Path $registry)) { throw 'Existing Prism installation detected. Use a fresh Windows user or VM.' }
foreach($folder in @((Get-PrismDesktopDirectory),[Environment]::GetFolderPath('Programs',[Environment+SpecialFolderOption]::DoNotVerify))) {
  if (Test-Path -LiteralPath (Join-Path $folder '棱镜浏览器 · 开发预览.lnk')) { throw 'Existing Prism shortcut detected. Refusing to replace or remove an unknown/daily shortcut.' }
}
if (!(Test-Path $installer1) -or !(Test-Path $installer2)) { throw 'Build both selected revisions before verifying installation.' }
if ($env:PRISM_WORKSPACE_ROOT) { throw 'Remove PRISM_WORKSPACE_ROOT before actual default-directory installation verification.' }
$sourceCommit=(& git -C $root rev-parse HEAD).Trim()
if($LASTEXITCODE -or [bool](& git -C $root status --porcelain)){throw 'No-click candidate verification requires the exact committed source used to build its fixture.'}
$releaseManifests=@{}
$requiredFiles=@('prism-browser.exe','prism-maintenance.exe','LICENSE','THIRD_PARTY_NOTICES.md','GO-THIRD-PARTY-NOTICES.txt','FRONTEND-THIRD-PARTY-NOTICES.txt','NSIS-LICENSE.txt','INSTALLATION.md','USER_GUIDE.md')
foreach($revision in @($FirstRevision,$SecondRevision)) {
  $version="0.3.0-preview.$revision"
  $manifest=Get-Content (Join-Path $releaseDirectory "prism-browser-$version-release.json") -Raw -Encoding UTF8|ConvertFrom-Json
  $expectedChannel=if($Candidate){'v1-candidate'}else{'development-preview'}
  if($manifest.version -ne $version -or $manifest.channel -ne $expectedChannel -or $manifest.sourceCommit -ne $sourceCommit -or $manifest.sourceDirty -ne $false -or $manifest.kernelIncluded -ne $false -or $manifest.architecture -ne 'amd64'){throw 'Candidate/source/channel/architecture identity mismatch.'}
  if(@(Compare-Object @($requiredFiles|Sort-Object) @($manifest.files.name|Sort-Object)).Count){throw 'Candidate does not contain the exact required distributed files.'}
  $package=Join-Path $releaseDirectory $manifest.installer.name
  if($manifest.installer.name -ne "prism-browser-$version-windows-amd64-setup.exe" -or (Get-FileHash -LiteralPath $package -Algorithm SHA256).Hash.ToLowerInvariant() -ne $manifest.installer.sha256){throw 'External candidate installer digest mismatch.'}
  $sum=Get-Content (Join-Path $releaseDirectory "prism-browser-$version-SHA256SUMS.txt") -Raw -Encoding UTF8
  if($sum.Trim() -ne "$($manifest.installer.sha256)  $($manifest.installer.name)"){throw 'Candidate SHA256SUMS differs from its release manifest.'}
  $releaseManifests[$revision]=$manifest
}
[IO.Directory]::CreateDirectory($evidence) | Out-Null
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class PrismInstallerWindow {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
}
'@

function Run-Installer([string]$File,[string]$Arguments,[int]$ExpectedCode=0,[switch]$Wizard) {
  [Console]::WriteLine("Verify $(Split-Path $File -Leaf): expected exit $ExpectedCode; wizard=$([bool]$Wizard)")
  $options=@{FilePath=$File;PassThru=$true}
  if ($Arguments) { $options.ArgumentList=$Arguments }
  $p=Start-Process @options
  $handle=$p.Handle
  try {
    if ($Wizard) {
      $sawKeepData=$false
      for ($i=0;$i -lt 300 -and !$p.HasExited;$i++) {
        $p.Refresh()
        if ($p.MainWindowHandle -ne 0) {
          $w=[Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
          $nodes=$w.FindAll([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.Condition]::TrueCondition)
          foreach($n in $nodes) {
            if($n.Current.AutomationId -eq '1201' -and $n.Current.Name -like '*全部棱镜数据*') {
              $state=[PrismInstallerWindow]::SendMessage([IntPtr]$n.Current.NativeWindowHandle,0x00F0,[IntPtr]::Zero,[IntPtr]::Zero).ToInt32()
              if($state -ne 0){throw 'Uninstaller selected destructive data removal by default.'}
              $sawKeepData=$true
            }
          }
          $next=$nodes | Where-Object {$_.Current.AutomationId -eq '1' -and $_.Current.IsEnabled -and $_.Current.NativeWindowHandle -ne 0} | Select-Object -First 1
          if($next -and ($File -notlike '*uninstall.exe' -or $sawKeepData)){[PrismInstallerWindow]::SendMessage([IntPtr]$next.Current.NativeWindowHandle,0x00F5,[IntPtr]::Zero,[IntPtr]::Zero)|Out-Null}
        }
        Start-Sleep -Milliseconds 200
      }
      if($File -like '*uninstall.exe' -and !$sawKeepData){throw 'Uninstall data-choice page was not observed.'}
    }
    if(!$p.WaitForExit(60000)){throw 'Installer did not exit.'}
    if($p.ExitCode -ne $ExpectedCode){throw "Installer returned $($p.ExitCode); expected $ExpectedCode."}
    $steps.Add(@{ executable=(Split-Path $File -Leaf); pid=$p.Id; expectedExit=$ExpectedCode; actualExit=$p.ExitCode; wizard=[bool]$Wizard })
    [IO.File]::WriteAllText((Join-Path $evidence 'installer-step-checkpoint.json'),($steps.ToArray()|ConvertTo-Json -Depth 6),$encoding)
  } finally {
    if(!$p.HasExited){Stop-Process -Id $p.Id -ErrorAction SilentlyContinue}
  }
}
function Installed-Exe([int]$Revision) { return (Join-Path $install "versions/0.3.0-preview.$Revision/prism-browser.exe") }
function Assert-Installed([int]$Revision) {
  if(!(Test-Path (Installed-Exe $Revision))){throw 'Installed executable absent.'}
  if((Get-ItemProperty $registry).DisplayVersion -ne "0.3.0-preview.$Revision"){throw 'Registered version mismatch.'}
  $shell=New-Object -ComObject Shell.Application
  foreach($folder in @((Get-PrismDesktopDirectory),[Environment]::GetFolderPath('Programs',[Environment+SpecialFolderOption]::DoNotVerify))) {
    $path=Join-Path $folder '棱镜浏览器 · 开发预览.lnk'
    if(!(Test-Path -LiteralPath $path)){throw 'Installed shortcut missing in the physical Windows known folder.'}
    # WScript.Shell uses ANSI link filenames and silently reads a different file
    # for our Chinese label on English Windows. Shell.Application reads Unicode.
    $namespace=$shell.NameSpace($folder)
    $shortcut=$namespace.ParseName([IO.Path]::GetFileName($path)).GetLink
    $targetEmpty=[string]::IsNullOrEmpty($shortcut.Path)
    $targetExists=(!$targetEmpty -and (Test-Path -LiteralPath $shortcut.Path))
    if(!$targetExists -or [IO.Path]::GetFullPath($shortcut.Path) -ne [IO.Path]::GetFullPath((Installed-Exe $Revision))){throw "Shortcut target mismatch; fileExists=$targetExists; targetEmpty=$targetEmpty; desktopFolder=$($folder -eq (Get-PrismDesktopDirectory))."}
    if($shortcut.WorkingDirectory -like '*ns*.tmp*'){throw 'Shortcut retains the removed installer temporary working directory.'}
  }
  $installedManifest=Get-Content (Join-Path $install "versions/0.3.0-preview.$Revision/release.json") -Raw -Encoding UTF8|ConvertFrom-Json
  if($installedManifest.version -ne "0.3.0-preview.$Revision"){throw 'Payload release version mismatch.'}
  $external=$releaseManifests[$Revision]
  if($installedManifest.sourceCommit -ne $external.sourceCommit -or $installedManifest.channel -ne $external.channel -or $installedManifest.sourceDirty -ne $false){throw 'Installed payload source/channel identity mismatch.'}
  if(@(Compare-Object @($requiredFiles|Sort-Object) @($installedManifest.files.name|Sort-Object)).Count){throw 'Installed payload lacks required notices/guides or includes unrecognized files.'}
  foreach($file in $external.files) {
    $actual=(Get-FileHash (Join-Path $install "versions/0.3.0-preview.$Revision/$($file.name)") -Algorithm SHA256).Hash.ToLowerInvariant()
    if($actual -ne $file.sha256){throw 'Installed executable checksum mismatch.'}
  }
}
function Verify-InstalledUI([int]$Revision,[switch]$ReadOnly) {
  if ($NoClicks) { return (Verify-InstalledWithoutClicks $Revision -ReadOnly:$ReadOnly) }
  $env:PRISM_VERIFY_SCRIPT=Join-Path $scriptsRoot 'verify-desktop-ui.ps1'
  $env:PRISM_VERIFY_EXE=Installed-Exe $Revision
  $env:PRISM_VERIFY_DATABASE_ROOT=$data
  $env:PRISM_VERIFY_EVIDENCE=Join-Path $evidence "preview-$Revision"
  $env:PRISM_VERIFY_READ_ONLY=if($ReadOnly){'1'}else{'0'}
  $env:PRISM_VERIFY_EXPECTED_VERSION="0.3.0-preview.$Revision"
  $env:PRISM_VERIFY_LAUNCH_SHORTCUTS='1'
  [IO.Directory]::CreateDirectory($env:PRISM_VERIFY_EVIDENCE)|Out-Null
  $command='& ([scriptblock]::Create([IO.File]::ReadAllText($env:PRISM_VERIFY_SCRIPT)))'
  & powershell.exe -NoProfile -NonInteractive -OutputFormat Text -ExecutionPolicy Bypass -EncodedCommand ([Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command)))
  if($LASTEXITCODE){throw 'Actual installed-exe UI verification failed.'}
  return (Get-Content (Join-Path $env:PRISM_VERIFY_EVIDENCE 'uia-verification.json') -Raw -Encoding UTF8|ConvertFrom-Json)
}
function Verify-InstalledWithoutClicks([int]$Revision,[switch]$ReadOnly) {
  $processes=[Collections.Generic.List[object]]::new()
  foreach($folder in @((Get-PrismDesktopDirectory),[Environment]::GetFolderPath('Programs',[Environment+SpecialFolderOption]::DoNotVerify))) {
    $shortcut=Join-Path $folder '棱镜浏览器 · 开发预览.lnk'
    $p=Start-Process -FilePath $shortcut -PassThru
    try {
      $deadline=[DateTime]::UtcNow.AddSeconds(30)
      do {
        $p.Refresh()
        if($p.HasExited){throw 'Installed application exited before its workspace window was available.'}
        if($p.MainWindowHandle -ne 0 -and $p.MainWindowTitle -like "*0.3.0-preview.$Revision*"){break}
        Start-Sleep -Milliseconds 100
      } while([DateTime]::UtcNow -lt $deadline)
      if($p.MainWindowHandle -eq 0 -or $p.MainWindowTitle -notlike "*0.3.0-preview.$Revision*"){throw 'Installed workspace window/version was not observed.'}
      if($Candidate -and $p.MainWindowTitle -notlike '*首版候选*'){throw 'Installed candidate window did not identify itself as a candidate.'}
      if([IO.Path]::GetFullPath($p.Path) -ne [IO.Path]::GetFullPath((Installed-Exe $Revision))){throw 'Shortcut opened a different executable.'}
      $expectRecord=$ReadOnly -or $processes.Count -gt 0
      $observed=Read-InstalledPageWithoutClicks $p.MainWindowHandle -ExpectRecord:$expectRecord
      if(!$p.CloseMainWindow() -or !$p.WaitForExit(30000) -or $p.ExitCode -ne 0){throw 'Normal installed-application close was not confirmed.'}
      $processes.Add(@{pid=$p.Id;revision=$Revision;normalWindowClose=$true;exitCode=$p.ExitCode;uiClicks=$false;pageObservation=$observed})
    } finally {
      if(!$p.HasExited){throw 'Owned application did not close; fixture retained for manual recovery. No forced process termination was used.'}
      $p.Dispose()
    }
    if(!$ReadOnly -and $processes.Count -eq 1) {
      # The default product/data roots were absent at entry. A test-only native
      # API helper requires this ownership nonce and refuses nonempty workspaces.
      $script:fixtureNonce=[Guid]::NewGuid().ToString()
      $marker=[IO.File]::Open((Join-Path $data '.prism-installer-fixture'),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
      try{$bytes=$encoding.GetBytes($script:fixtureNonce);$marker.Write($bytes,0,$bytes.Length);$marker.Flush($true)}finally{$marker.Dispose()}
      $env:PRISM_INSTALLER_FIXTURE_NONCE=$script:fixtureNonce
      $env:GOPATH=Join-Path $root '.tools/gopath'
      $env:GOCACHE=Join-Path $root '.tools/gocache'
      $env:GOTOOLCHAIN='local'
      $env:CGO_ENABLED='0'
      try {
        & (Join-Path $root '.tools/go/bin/go.exe') test -p 1 -count=1 -v ./internal/workspace -run '^TestCandidateInstallerWorkspaceFixture$' | Out-Host
        if($LASTEXITCODE){throw 'Owned synthetic native API fixture failed; no UI interactions were used.'}
      } finally { Remove-Item Env:PRISM_INSTALLER_FIXTURE_NONCE -ErrorAction SilentlyContinue }
    }
  }
  return @{database=(Read-Record);processes=$processes.ToArray()}
}
function Read-InstalledPageWithoutClicks([IntPtr]$Handle,[switch]$ExpectRecord) {
  $deadline=[DateTime]::UtcNow.AddSeconds(30)
  do {
    $window=[Windows.Automation.AutomationElement]::FromHandle($Handle)
    $nodes=$window.FindAll([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.Condition]::TrueCondition)
    $names=@($nodes|ForEach-Object{$_.Current.Name})
    $text=$names -join "`n"
    $native=$text.Contains('本机桌面') -and $text.Contains('环境名称 / 编号')
    $issue=$text.Contains('工作区需要处理')
    $record=$text.Contains('SYNTHETIC-INSTALLER-ONLY')
    if($native -and !$issue -and (!$ExpectRecord -or $record)){return @{readOnlyUIA=$true;nativeWorkspaceLoaded=$true;workspaceError=$false;syntheticRecordVisible=$record;recordRequired=[bool]$ExpectRecord}}
    Start-Sleep -Milliseconds 150
  } while([DateTime]::UtcNow -lt $deadline)
  throw 'Read-only accessibility inspection did not confirm native workspace readiness/synthetic record; no clicks or input were performed.'
}
function Read-Record {
  $result=& node --disable-warning=ExperimentalWarning (Join-Path $scriptsRoot 'verify-desktop-db.mjs') (Join-Path $data 'app.db')
  if($LASTEXITCODE){throw 'Installed SQLite verification failed.'}
  return ($result|ConvertFrom-Json)
}
function Run-Uninstaller([switch]$RemoveData,[switch]$Wizard) {
  # _?= normally executes the original in place. Run a byte-for-byte test copy so
  # the real installation's uninstaller can be removed and we can wait its PID.
  $copy=Join-Path $evidence "uninstall-step-$($steps.Count)/uninstall.exe"
  [IO.Directory]::CreateDirectory((Split-Path $copy -Parent))|Out-Null
  Copy-Item (Join-Path $install 'uninstall.exe') $copy
  $arguments=if($Wizard){"_?=$install"}elseif($RemoveData){"/S /REMOVE-DATA=CONFIRMED _?=$install"}else{"/S _?=$install"}
  Run-Installer $copy $arguments -Wizard:$Wizard
}
try {
  Run-Installer $installer1 $(if($SilentWizard){'/S'}else{''}) -Wizard:(!$SilentWizard)
  Assert-Installed $FirstRevision
  $first=Verify-InstalledUI $FirstRevision
  $saved=($first.database|ConvertTo-Json -Depth 12 -Compress)
  Run-Installer $installer2 '/S'
  Assert-Installed $SecondRevision
  if((Read-Record|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Upgrade changed SQLite identity/preferences.'}
  $upgraded=Verify-InstalledUI $SecondRevision -ReadOnly
  if(($upgraded.database|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Upgraded UI changed identity/preferences.'}
  Run-Installer $installer1 '/S' 25
  Assert-Installed $SecondRevision
  # Real uninstall-choice UI locally; CI may use documented /S retain-data default.
  Run-Uninstaller -Wizard:(!$SilentWizard)
  if((Test-Path $registry) -or (Test-Path (Installed-Exe $SecondRevision))){throw 'Uninstall left installed registration/executable.'}
  if((Read-Record|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Default uninstall changed retained SQLite data.'}
  Run-Installer $installer2 '/S'
  Assert-Installed $SecondRevision
  $reinstalled=Verify-InstalledUI $SecondRevision -ReadOnly
  if(($reinstalled.database|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Reinstall did not rediscover the retained workspace.'}
  # This fresh product root contains ONLY this script's synthetic data; explicit destructive option.
  if($NoClicks) {
    $marker=[IO.File]::ReadAllText((Join-Path $data '.prism-installer-fixture'),$encoding)
    if($marker -ne $script:fixtureNonce -or (Read-Record|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Refusing explicit deletion: synthetic ownership/data identity is no longer confirmed.'}
  }
  Run-Uninstaller -RemoveData
  if((Test-Path $data) -or (Test-Path $registry) -or (Test-Path (Installed-Exe $SecondRevision))){throw 'Explicit data removal or uninstall incomplete.'}
  $driver=if($NoClicks){'Actual NSIS + shortcut launches/normal window close + native API fixture/read-only SQLite; no UI clicks'}else{'Windows UI Automation + actual NSIS installation'}
  $proof=@{verifiedAt=[DateTime]::UtcNow.ToString('o');platform='windows/amd64';windows=(Get-CimInstance Win32_OperatingSystem).Caption;driver=$driver;sourceCommit=$sourceCommit;candidateDigests=@($releaseManifests[$FirstRevision].installer)+@($releaseManifests[$SecondRevision].installer);freshWindowsUser=$false;freshProductStateAtStart=$true;unknownShortcutsAbsentAtStart=$true;actualDefaultKnownFolders=$true;environmentOverrides=$false;uiClicks=(!$NoClicks);firstRevision=$FirstRevision;secondRevision=$SecondRevision;installerSteps=$steps.ToArray();database=$first.database;appProcesses=@($first.processes)+@($upgraded.processes)+@($reinstalled.processes);checks=@{firstAndSecondStart=$true;upgradeIdentityPreserved=$true;downgradeRejected=$true;defaultUninstallRetained=$true;reinstallFoundSameData=$true;explicitRemoveData=$true;defaultDataCheckboxUnchecked=(!$SilentWizard)}}
  [IO.File]::WriteAllText((Join-Path $evidence 'installer-verification.json'),($proof|ConvertTo-Json -Depth 14),$encoding)
  Write-Output "PASS: actual per-user install/native configuration/reopen/upgrade/retain-uninstall/reinstall/owned synthetic delete completed. No-click mode=$NoClicks; clean Windows user not implied."
} finally {
  # Never recursively delete roots on failure; retain the owned synthetic fixture for diagnosis.
  Remove-Item Env:PRISM_VERIFY_READ_ONLY,Env:PRISM_VERIFY_DATABASE_ROOT,Env:PRISM_VERIFY_LAUNCH_SHORTCUTS -ErrorAction SilentlyContinue
}
