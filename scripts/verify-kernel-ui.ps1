# Dot-sourced by verify-desktop-ui.ps1. Reuses its validated UIA/normal-close helpers.
Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class PrismKernelInput {
  private delegate bool EnumProc(IntPtr window, IntPtr data);
  [DllImport("user32.dll")] private static extern bool EnumChildWindows(IntPtr parent, EnumProc callback, IntPtr data);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetClassName(IntPtr window, StringBuilder name, int size);
  [DllImport("user32.dll")] private static extern bool PostMessage(IntPtr window, uint message, IntPtr key, IntPtr data);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] private static extern bool SetForegroundWindow(IntPtr window);
  [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr window, IntPtr dc, uint flags);
  public static void RestoreUserWindow(IntPtr previous,int ownedProcess){
    uint process;GetWindowThreadProcessId(GetForegroundWindow(),out process);
    if(previous!=IntPtr.Zero && process==(uint)ownedProcess)SetForegroundWindow(previous);
  }
  [DllImport("user32.dll", CharSet=CharSet.Unicode, EntryPoint="SendMessageW")] private static extern IntPtr SetWindowTextMessage(IntPtr window, uint message, IntPtr key, string text);
  [DllImport("user32.dll", CharSet=CharSet.Unicode, EntryPoint="SendMessageW")] private static extern IntPtr ReadWindowTextMessage(IntPtr window, uint message, IntPtr size, StringBuilder text);
  public static void SetText(IntPtr edit,string text){
    if(SetWindowTextMessage(edit,0x000C,IntPtr.Zero,text)==IntPtr.Zero)throw new InvalidOperationException("Native file-name control rejected text.");
    // GetWindowText cannot read an Edit control in another process; WM_GETTEXT can.
    var actual=new StringBuilder(32768);ReadWindowTextMessage(edit,0x000D,(IntPtr)actual.Capacity,actual);
    if(actual.ToString()!=text)throw new InvalidOperationException("Native file-name control Unicode readback differs.");
  }
  public static void Click(IntPtr button){if(!PostMessage(button,0x00F5,IntPtr.Zero,IntPtr.Zero))throw new InvalidOperationException("Owned dialog button click failed.");}
  public static void Key(IntPtr parent, int key) {
    IntPtr renderer=IntPtr.Zero;
    EnumChildWindows(parent,(window,data)=>{var name=new StringBuilder(256);GetClassName(window,name,256);if(name.ToString()=="Chrome_RenderWidgetHostHWND"){renderer=window;return false;}return true;},IntPtr.Zero);
    if(renderer==IntPtr.Zero)throw new InvalidOperationException("Owned WebView renderer HWND was not found.");
    if(!PostMessage(renderer,0x0100,(IntPtr)key,(IntPtr)1) || !PostMessage(renderer,0x0101,(IntPtr)key,(IntPtr)unchecked((int)0xC0000001)))throw new InvalidOperationException("Targeted UI keyboard event failed.");
  }
}
'@
function Open-KernelDesktop {
  $previous=[PrismKernelInput]::GetForegroundWindow()
  Open-Desktop
  [PrismKernelInput]::RestoreUserWindow($previous,$script:process.Id)
}
function Set-Value([string]$Name,[string]$Value) {
  # ValuePattern does not need keyboard focus. Avoid exposing inputs to unrelated
  # keystrokes on a shared desktop; always verify the value actually changed.
  $element=Wait-Element $Name 'Edit|ComboBox|Spinner'
  $provider=[Windows.Automation.ValuePattern]$element.GetCurrentPattern([Windows.Automation.ValuePattern]::Pattern)
  $provider.SetValue($Value)
  for($i=0;$i -lt 100;$i++){
    if((Get-Value $Name) -eq $Value){return}
    Start-Sleep -Milliseconds 50
  }
  throw "UI input readback differs: $Name"
}
function Capture-Window([string]$File) {
  if($env:PRISM_VERIFY_NO_SCREENSHOT -eq '1'){return}
  # Never copy screen pixels: another application may occlude a shared desktop.
  # Print only the owned HWND, without raising it or changing foreground focus.
  $r=$script:window.Current.BoundingRectangle
  $image=[Drawing.Bitmap]::new([int]$r.Width,[int]$r.Height)
  $graphics=[Drawing.Graphics]::FromImage($image)
  try {
    $dc=$graphics.GetHdc()
    try{$captured=[PrismKernelInput]::PrintWindow($script:process.MainWindowHandle,$dc,2)}finally{$graphics.ReleaseHdc($dc)}
    if($captured){$image.Save((Join-Path $env:PRISM_VERIFY_EVIDENCE $File))}
  } finally {$graphics.Dispose();$image.Dispose()}
}
function Read-KernelDatabase([switch]$Hash) {
  $arguments=@('--disable-warning=ExperimentalWarning',(Join-Path $script:scriptsRoot 'verify-kernel-db.mjs'),$env:PRISM_WORKSPACE_ROOT)
  if($Hash){$arguments+='--hash'}
  $result = & node @arguments
  if($LASTEXITCODE){throw 'Independent kernel SQLite/files inspection failed.'}
  return ($result | ConvertFrom-Json)
}
function Invoke-Named([string]$Name,[string]$Type='Button|Hyperlink') {
  $element=Wait-Element $Name $Type
  ([Windows.Automation.InvokePattern]$element.GetCurrentPattern([Windows.Automation.InvokePattern]::Pattern)).Invoke()
}
function Select-Combo([string]$Name,[int]$Index) {
  $element=Wait-Element $Name 'ComboBox'
  # ValuePattern on Chromium select returns S_OK without changing selection.
  # Target normal key messages to this owned WebView HWND, never global SendKeys.
  $previous=[PrismKernelInput]::GetForegroundWindow()
  $element.SetFocus()
  [PrismKernelInput]::RestoreUserWindow($previous,$script:process.Id)
  [PrismKernelInput]::Key($script:process.MainWindowHandle,0x24)
  for($i=0;$i -lt $Index;$i++){[PrismKernelInput]::Key($script:process.MainWindowHandle,0x28)}
}
function Wait-KernelTask([string]$ExpectedState,[string]$Reason='', [int]$TimeoutSeconds=180) {
  $deadline=[DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
  while([DateTime]::UtcNow -lt $deadline){
    $database=Read-KernelDatabase
    $task=$database.operations | Select-Object -First 1
    if($task -and $task.id -ne $script:lastKernelTask -and $task.state -in 'completed','failed','cancelled'){
      if($task.state -ne $ExpectedState){throw "Kernel task unexpectedly ended $($task.state): $($task.error | ConvertTo-Json -Compress)"}
      if($Reason -and $task.error.details.reason -ne $Reason){throw "Kernel failure reason differs: $($task.error.details.reason) expected $Reason"}
      # Ensure UI has consumed the terminal result, not merely the independent database reader.
      if($ExpectedState -eq 'completed'){Wait-Element '内核任务已实际完成；环境正常启停仍待接入。' 'Text' | Out-Null}
      else {Wait-Element "$($task.error.code)：$($task.error.message)（$Reason）" 'Text' | Out-Null}
      $script:lastKernelTask=$task.id
      return $database
    }
    Start-Sleep -Milliseconds 300
  }
  throw 'Kernel task did not finish within the actual desktop verification timeout.'
}
function Choose-LocalArchive {
  Invoke-Button '选择可信 ZIP'
  $desktop=[Windows.Automation.AutomationElement]::RootElement
  $condition=[Windows.Automation.AndCondition]::new(
    [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::NameProperty,'选择可信 fingerprint-chromium ZIP'),
    [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::ProcessIdProperty,$script:process.Id),
    [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::ControlTypeProperty,[Windows.Automation.ControlType]::Window))
  $dialog=$null
  for($i=0;$i -lt 100;$i++){$dialog=$desktop.FindFirst([Windows.Automation.TreeScope]::Descendants,$condition);if($dialog){break};Start-Sleep -Milliseconds 100}
  if(!$dialog){throw 'Native file-picker dialog did not open.'}
  $controls=$dialog.FindAll([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.Condition]::TrueCondition)|Where-Object{$_.Current.ControlType -notin [Windows.Automation.ControlType]::DataItem,[Windows.Automation.ControlType]::ListItem,[Windows.Automation.ControlType]::TreeItem}
  $diagnostic=@("Dialog $($dialog.Current.ControlType.ProgrammaticName) class $($dialog.Current.ClassName) hwnd $($dialog.Current.NativeWindowHandle)")+@($controls|ForEach-Object{"$($_.Current.ControlType.ProgrammaticName) | id $($_.Current.AutomationId) class $($_.Current.ClassName) patterns $($_.GetSupportedPatterns().ProgrammaticName -join ',')"})
  [IO.File]::WriteAllText((Join-Path $env:PRISM_VERIFY_EVIDENCE 'kernel-picker-controls.txt'),($diagnostic -join "`n"),[Text.UTF8Encoding]::new($false))
  # Wails' common dialog exposes legacy HWND controls as Pane without a Value
  # provider on some hosts. Match its locale-independent ID AND actual class.
  $filename=$dialog.FindFirst([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.AndCondition]::new([Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::AutomationIdProperty,'1148'),[Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::ClassNameProperty,'Edit')))
  if(!$filename){throw 'Native file-picker filename control was not found.'}
  [PrismKernelInput]::SetText([IntPtr]$filename.Current.NativeWindowHandle,$env:PRISM_KERNEL_ARCHIVE)
  $open=$dialog.FindFirst([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.AndCondition]::new([Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::AutomationIdProperty,'1'),[Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::ClassNameProperty,'Button')))
  if(!$open -or $open.Current.ClassName -ne 'Button'){throw 'Owned native open-file button was not found.'}
  [PrismKernelInput]::Click([IntPtr]$open.Current.NativeWindowHandle)
  Wait-Element ([IO.Path]::GetFileName($env:PRISM_KERNEL_ARCHIVE)) 'Text' | Out-Null
  $checkbox=Wait-Element '我已核对来源和摘要，确认此 ZIP 可信，允许执行其中的内核进行诊断。' 'CheckBox'
  ([Windows.Automation.TogglePattern]$checkbox.GetCurrentPattern([Windows.Automation.TogglePattern]::Pattern)).Toggle()
}
function Invoke-KernelButton([string]$Name,[int]$Index=0,[switch]$Inspect) {
  $condition=[Windows.Automation.AndCondition]::new([Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::NameProperty,$Name),[Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::ControlTypeProperty,[Windows.Automation.ControlType]::Button))
  $items=$script:window.FindAll([Windows.Automation.TreeScope]::Descendants,$condition)
  if($items.Count -le $Index){throw "Missing kernel action $Name at $Index"}
  $element=$items.Item($Index)
  if(!$Inspect){([Windows.Automation.InvokePattern]$element.GetCurrentPattern([Windows.Automation.InvokePattern]::Pattern)).Invoke()}
  return $element
}
function Select-ExactKernel([string]$KernelID) {
  $database=Read-KernelDatabase
  $ids=[string[]](@('kernel-pending')+@($database.records|ForEach-Object{$_.id}))
  [Array]::Sort($ids,[StringComparer]::Ordinal) # Workspace.Read returns kernels ORDER BY id.
  $index=[Array]::IndexOf($ids,$KernelID)
  if($index -lt 0){throw 'The explicitly selected kernel ID is absent.'}
  Select-Combo '固定内核版本' $index
  # The production UI includes the short ID to distinguish identical versions.
  for($i=0;$i -lt 100;$i++){
    if((Get-Value '固定内核版本').Contains($KernelID.Substring(0,8))){return}
    Start-Sleep -Milliseconds 100
  }
  throw 'The UI did not display the explicitly selected exact kernel ID.'
}
try {
  Open-KernelDesktop
  Invoke-Named '内核管理'
  $initial=Read-KernelDatabase
  $resumeBound=$env:PRISM_KERNEL_RESUME -eq 'after-local'
  if($resumeBound){
    if($initial.records.Count -ne 2 -or $initial.environments.Count -ne 1 -or !$initial.environments[0].configuration.name.StartsWith('精确内核合成环境')){throw 'Retained synthetic fixture does not match the local/binding checkpoint.'}
    $officialRecord=$initial.records|Where-Object{$_.source.kind -eq 'official'}|Select-Object -First 1
    $localRecord=$initial.records|Where-Object{$_.source.kind -eq 'local'}|Select-Object -First 1
    if(!$officialRecord -or !$localRecord){throw 'Retained fixture lacks its distinct official/local builds.'}
    $script:lastKernelTask=$initial.operations[0].id
    $original=$initial.environments[0]
  } else {
  if($env:PRISM_KERNEL_RESUME -eq 'after-official'){
    if($initial.records.Count -ne 1 -or $initial.environments.Count -or $initial.records[0].source.kind -ne 'official' -or !($initial.operations | Where-Object {$_.error.details.reason -eq 'asset-unavailable'})){throw 'Retained synthetic fixture does not match the explicitly selected resume checkpoint.'}
    $official=$initial
    $script:lastKernelTask=$initial.operations[0].id
  } else {
  if($initial.records.Count -or $initial.environments.Count){throw 'Kernel verification requires its own empty synthetic workspace; retained fixture is not reset.'}

  # An exact missing tag must fail without an implicit fallback.
  Set-Value '精确发行版本' '150.0.7871.186'
  Set-Value '预期归档 SHA-256' ('a'*64)
  Invoke-Button '安装并核验'
  $missing=Wait-KernelTask 'failed' 'asset-unavailable'
  if($missing.records.Count){throw 'Unavailable exact tag silently installed another version.'}

  Set-Value '精确发行版本' '148.0.7778.215'
  Set-Value '预期归档 SHA-256' '9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579'
  Invoke-Button '安装并核验'
  $official=Wait-KernelTask 'completed'
  }
  if($official.records.Count -ne 1 -or $official.records[0].source.kind -ne 'official' -or $official.records[0].source.commit -ne '13b89eae304123f0710f2d33fd0816a2f61d7ffc'){throw 'Official exact source/tag/commit did not persist.'}
  $officialRecord=$official.records[0]
  Capture-Window 'kernel-official-installed.png'

  Select-Combo '归档来源' 1
  Choose-LocalArchive
  Set-Value '预期归档 SHA-256' ('a'*64)
  Invoke-Button '安装并核验'
  $mismatch=Wait-KernelTask 'failed' 'hash-mismatch'
  if($mismatch.records.Count -ne 1 -or $mismatch.records[0].id -ne $officialRecord.id){throw 'Bad local archive digest affected the existing kernel.'}

  Set-Value '预期归档 SHA-256' '9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579'
  Invoke-Button '安装并核验'
  $local=Wait-KernelTask 'completed'
  if($local.records.Count -ne 2 -or $local.records[0].id -eq $officialRecord.id -or $local.records[0].source.kind -ne 'local'){throw 'Local install did not allocate another immutable exact ID.'}
  $localRecord=$local.records[0]
  $verified=Read-KernelDatabase -Hash
  }

  if($resumeBound -and $original.kernelId -eq $officialRecord.id -and $original.configuration.name -eq '精确内核合成环境'){
    # Binding already passed before a later driver failure; do not save it again.
    $bound=$initial
    $seed=$original.seed
  } else {
  if($resumeBound){
    Invoke-Named '浏览器环境 1'
    Invoke-Button "$($original.configuration.name) 更多操作"
    Invoke-Button '编辑环境'
    $original=$initial.environments[0]
  } else {
    Invoke-Named '浏览器环境 0'
    Invoke-Button '新建环境'
    Set-Value '环境名称' '精确内核合成环境'
  }
  Select-ExactKernel $officialRecord.id
  Invoke-Button '设备指纹'
  Invoke-Button '生成并查看预览'
  $seed=Get-Value '固定指纹种子'
  if($resumeBound -and $original.configuration.name -ne '精确内核合成环境'){
    Invoke-Button '基础与代理'
    Set-Value '环境名称' '精确内核合成环境'
  }
  if($resumeBound){Invoke-Button '保存配置';Wait-Element '环境配置已保存' 'Text'|Out-Null}else{Invoke-Button '创建环境';Wait-Element '已创建 1 个环境' 'Text'|Out-Null}
  $bound=Read-KernelDatabase
  }
  if($bound.environments.Count -ne 1 -or $bound.environments[0].kernelId -ne $officialRecord.id -or $bound.environments[0].seed -ne $seed -or $bound.environments[0].configuration.name -ne '精确内核合成环境'){throw 'UI selection did not persist the exact name/kernel ID/seed.'}
  $saved=$bound.environments[0]
  if($resumeBound -and ($saved.id -ne $original.id -or $saved.seed -ne $original.seed)){throw 'Explicit kernel editing changed the fixed identity.'}
  Invoke-Named '内核管理'
  # Newest local card is first, referenced official card second.
  $protected=Invoke-KernelButton '移除此构建' 1 -Inspect
  if($protected.Current.IsEnabled){throw 'Referenced kernel removal was not disabled.'}
  Wait-Element '精确内核合成环境' 'ListItem' | Out-Null
  Capture-Window 'kernel-reference-protected.png'
  Close-Desktop

  Open-KernelDesktop
  Invoke-Named '内核管理'
  $reopened=Read-KernelDatabase -Hash
  if(($reopened.environments[0]|ConvertTo-Json -Depth 10 -Compress) -ne ($saved|ConvertTo-Json -Depth 10 -Compress)){throw 'Normal reopen changed the fixed identity/reference.'}
  if($reopened.records.Count -ne 2){throw 'Normal reopen lost precise kernel evidence.'}
  Invoke-KernelButton '重新核验' 1 | Out-Null
  $reverified=Wait-KernelTask 'completed'
  if($reverified.operations[0].report.observations.Count -ne 3){throw 'Reverify did not perform new real probe sessions.'}
  Invoke-KernelButton '移除此构建' 0 | Out-Null
  Invoke-Button '确认移除'
  $removed=Wait-KernelTask 'completed'
  if($removed.records.Count -ne 1 -or $removed.records[0].id -ne $officialRecord.id){throw 'Deletion affected another exact build.'}
  if(Test-Path -LiteralPath (Join-Path $env:PRISM_WORKSPACE_ROOT $localRecord.installPath)){throw 'Removed unused kernel directory remains.'}
  $final=Read-KernelDatabase -Hash
  Capture-Window 'kernel-reopened-verified.png'
  Close-Desktop
  $evidence=@{verifiedAt=[DateTime]::UtcNow.ToString('o');platform='windows/amd64';driver='production Windows UI Automation and independent read-only SQLite/files';processes=$script:processes.ToArray();officialRecord=$officialRecord;localRecord=$localRecord;final=$final;checks=@{exactUnavailableTagNoFallback=$true;officialSourceTagCommitAndRealVersion=$true;localPickerAndTrustConfirmation=$true;badArchiveHashKeepsOldKernel=$true;separateImmutableIDs=$true;explicitEnvironmentReference=$true;referencedKernelRemovalDisabled=$true;normalCloseReopenKeepsIdentity=$true;realReverify=$true;unusedKernelRemovalIsolated=$true;allInstalledFilesAndStagingIndependentlyChecked=$true;noDebugTCPOrSandboxDisable=$true}}
  [IO.File]::WriteAllText((Join-Path $env:PRISM_VERIFY_EVIDENCE 'kernel-uia-verification.json'),($evidence|ConvertTo-Json -Depth 30),[Text.UTF8Encoding]::new($false))
} catch {
  if($script:window -and $script:process -and !$script:process.HasExited){
    try{Capture-Window 'kernel-failed.png';$names=$script:window.FindAll([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.Condition]::TrueCondition)|ForEach-Object{"$($_.Current.ControlType.ProgrammaticName): $($_.Current.Name)"};[IO.File]::WriteAllText((Join-Path $env:PRISM_VERIFY_EVIDENCE 'kernel-failed-controls.txt'),($names -join "`n"),[Text.UTF8Encoding]::new($false))}catch{}
  }
  throw
} finally {
  if($script:process -and !$script:process.HasExited){Stop-Process -Id $script:process.Id -ErrorAction SilentlyContinue}
}
