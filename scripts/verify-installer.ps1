param([switch]$SilentWizard)
# Uses actual default Windows known folders. Refuses existing installation/user data.
# No test override of LOCALAPPDATA or PRISM_WORKSPACE_ROOT; every side effect is on this fresh product installation.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$scriptsRoot=Split-Path $env:PRISM_INSTALLER_VERIFY -Parent
. (Join-Path $scriptsRoot 'powershell-host.ps1')
$root=Split-Path $scriptsRoot -Parent
$local=[Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
$data=Join-Path $local 'PrismBrowser'
$install=Join-Path $local 'Programs/PrismBrowserPreview'
$registry='HKCU:/Software/Microsoft/Windows/CurrentVersion/Uninstall/PrismBrowserPreview'
$evidence=Join-Path $root 'output/goal/T03'
$steps=[Collections.Generic.List[object]]::new()
$encoding=[Text.UTF8Encoding]::new($false)
$installer1=Join-Path $root 'build/releases/prism-browser-0.3.0-preview.1-windows-amd64-setup.exe'
$installer2=Join-Path $root 'build/releases/prism-browser-0.3.0-preview.2-windows-amd64-setup.exe'
if (Test-Path $data) { throw 'Existing default Prism data detected. Refusing to modify/delete it. Use a fresh Windows user or VM.' }
if ((Test-Path $install) -or (Test-Path $registry)) { throw 'Existing Prism installation detected. Use a fresh Windows user or VM.' }
if (!(Test-Path $installer1) -or !(Test-Path $installer2)) { throw 'Build preview revisions 1 and 2 before verifying installation.' }
if ($env:PRISM_WORKSPACE_ROOT) { throw 'Remove PRISM_WORKSPACE_ROOT before actual default-directory installation verification.' }
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
  $shell=New-Object -ComObject WScript.Shell
  foreach($folder in @((Get-PrismDesktopDirectory),[Environment]::GetFolderPath('Programs',[Environment+SpecialFolderOption]::DoNotVerify))) {
    $path=Join-Path $folder '棱镜浏览器 · 开发预览.lnk'
    if(!(Test-Path -LiteralPath $path)){throw 'Installed shortcut missing in the physical Windows known folder.'}
    $shortcut=$shell.CreateShortcut($path)
    if(!$shortcut.TargetPath -or [IO.Path]::GetFullPath($shortcut.TargetPath) -ne [IO.Path]::GetFullPath((Installed-Exe $Revision))){throw "Shortcut target mismatch; fileExists=$([bool](Test-Path -LiteralPath $shortcut.TargetPath)); targetEmpty=$([string]::IsNullOrEmpty($shortcut.TargetPath)); desktopFolder=$($folder -eq (Get-PrismDesktopDirectory))."}
    if($shortcut.WorkingDirectory -like '*ns*.tmp*'){throw 'Shortcut retains the removed installer temporary working directory.'}
  }
  $installedManifest=Get-Content (Join-Path $install "versions/0.3.0-preview.$Revision/release.json") -Raw -Encoding UTF8|ConvertFrom-Json
  if($installedManifest.version -ne "0.3.0-preview.$Revision"){throw 'Payload release version mismatch.'}
  foreach($file in $installedManifest.files) {
    $actual=(Get-FileHash (Join-Path $install "versions/0.3.0-preview.$Revision/$($file.name)") -Algorithm SHA256).Hash.ToLowerInvariant()
    if($actual -ne $file.sha256){throw 'Installed executable checksum mismatch.'}
  }
}
function Verify-InstalledUI([int]$Revision,[switch]$ReadOnly) {
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
  Assert-Installed 1
  $first=Verify-InstalledUI 1
  $saved=($first.database|ConvertTo-Json -Depth 12 -Compress)
  Run-Installer $installer2 '/S'
  Assert-Installed 2
  if((Read-Record|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Upgrade changed SQLite identity/preferences.'}
  $upgraded=Verify-InstalledUI 2 -ReadOnly
  if(($upgraded.database|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Upgraded UI changed identity/preferences.'}
  Run-Installer $installer1 '/S' 25
  Assert-Installed 2
  # Real uninstall-choice UI locally; CI may use documented /S retain-data default.
  Run-Uninstaller -Wizard:(!$SilentWizard)
  if((Test-Path $registry) -or (Test-Path (Installed-Exe 2))){throw 'Uninstall left installed registration/executable.'}
  if((Read-Record|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Default uninstall changed retained SQLite data.'}
  Run-Installer $installer2 '/S'
  Assert-Installed 2
  $reinstalled=Verify-InstalledUI 2 -ReadOnly
  if(($reinstalled.database|ConvertTo-Json -Depth 12 -Compress) -ne $saved){throw 'Reinstall did not rediscover the retained workspace.'}
  # This fresh product root contains ONLY this script's synthetic data; explicit destructive option.
  Run-Uninstaller -RemoveData
  if((Test-Path $data) -or (Test-Path $registry) -or (Test-Path (Installed-Exe 2))){throw 'Explicit data removal or uninstall incomplete.'}
  $proof=@{verifiedAt=[DateTime]::UtcNow.ToString('o');platform='windows/amd64';windows=(Get-CimInstance Win32_OperatingSystem).Caption;driver='Windows UI Automation + actual NSIS installation';freshProductStateAtStart=$true;actualDefaultKnownFolders=$true;environmentOverrides=$false;installerSteps=$steps.ToArray();database=$first.database;appProcesses=@($first.processes)+@($upgraded.processes)+@($reinstalled.processes);checks=@{firstAndSecondStart=$true;upgradeIdentityPreserved=$true;downgradeRejected=$true;defaultUninstallRetained=$true;reinstallFoundSameData=$true;explicitRemoveData=$true;defaultDataCheckboxUnchecked=(!$SilentWizard)}}
  [IO.File]::WriteAllText((Join-Path $evidence 'installer-verification.json'),($proof|ConvertTo-Json -Depth 14),$encoding)
  Write-Output 'PASS: actual per-user install/create/reopen/upgrade/retain-uninstall/reinstall/explicit-delete completed in default Windows directories.'
} finally {
  # Never recursively delete roots on failure; retain the owned synthetic fixture for diagnosis.
  Remove-Item Env:PRISM_VERIFY_READ_ONLY,Env:PRISM_VERIFY_DATABASE_ROOT,Env:PRISM_VERIFY_LAUNCH_SHORTCUTS -ErrorAction SilentlyContinue
}
