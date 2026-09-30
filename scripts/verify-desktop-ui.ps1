# Called with explicit UTF-8 decoding by verify-desktop.mjs. Operates only its own child exe.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class PrismTestWindow {
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int n);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int width, int height, uint flags);
}
'@
$script:process = $null
$script:scriptsRoot = Split-Path $env:PRISM_VERIFY_SCRIPT -Parent
$script:window = $null
$script:processes = [Collections.Generic.List[object]]::new()
function Find-Element([string]$Name, [string]$Type = '') {
  if (!$script:window) { return $null }
  $condition = [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::NameProperty, $Name)
  $items = $script:window.FindAll([Windows.Automation.TreeScope]::Descendants, $condition)
  foreach ($item in $items) { if (!$Type -or ($Type -split '\|' | Where-Object { $item.Current.ControlType.ProgrammaticName -eq "ControlType.$_" })) { return $item } }
  return $null
}
function Wait-Element([string]$Name, [string]$Type = '') {
  for ($i = 0; $i -lt 150; $i++) {
    $element = Find-Element $Name $Type
    if ($element) { return $element }
    if ($script:process.HasExited) { throw 'Desktop exited before the UI step completed.' }
    Start-Sleep -Milliseconds 100
  }
  throw "Timed out waiting for UI control: $Name ($Type)"
}
function Invoke-Button([string]$Name) {
  $element = Wait-Element $Name 'Button'
  ([Windows.Automation.InvokePattern]$element.GetCurrentPattern([Windows.Automation.InvokePattern]::Pattern)).Invoke()
}
function Set-Value([string]$Name, [string]$Value) {
  $element = Wait-Element $Name 'Edit|ComboBox|Spinner'
  $element.SetFocus()
  ([Windows.Automation.ValuePattern]$element.GetCurrentPattern([Windows.Automation.ValuePattern]::Pattern)).SetValue($Value)
}
function Get-Value([string]$Name) {
  $element = Wait-Element $Name 'Edit|ComboBox|Spinner'
  return ([Windows.Automation.ValuePattern]$element.GetCurrentPattern([Windows.Automation.ValuePattern]::Pattern)).Current.Value
}
function Read-Database {
  $result = & node --disable-warning=ExperimentalWarning (Join-Path $script:scriptsRoot 'verify-desktop-db.mjs') (Join-Path $env:PRISM_WORKSPACE_ROOT 'app.db')
  if ($LASTEXITCODE) { throw 'Synthetic SQLite inspection failed.' }
  return ($result | ConvertFrom-Json)
}
function Open-Desktop {
  $script:process = Start-Process -FilePath $env:PRISM_VERIFY_EXE -PassThru
  $script:processes.Add(@{pid=$script:process.Id; executable='prism-browser.exe'; startedAt=[DateTime]::UtcNow.ToString('o'); normalExit=$false})
  for ($i = 0; $i -lt 150; $i++) {
    $script:process.Refresh()
    if ($script:process.HasExited) { throw 'Desktop did not start.' }
    if ($script:process.MainWindowHandle -ne 0) {
      $script:window = [Windows.Automation.AutomationElement]::FromHandle($script:process.MainWindowHandle)
      if (Find-Element '本机桌面' 'Text') { return }
    }
    Start-Sleep -Milliseconds 100
  }
  throw 'Native desktop window did not finish loading.'
}
function Close-Desktop {
  # WM_CLOSE is the same normal-close path as the titlebar X, not a process kill.
  [PrismTestWindow]::PostMessage($script:process.MainWindowHandle, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
  if (!$script:process.WaitForExit(15000)) { throw 'Normal desktop close did not exit.' }
  if ($script:process.ExitCode -ne 0) { throw 'Desktop close failed.' }
  $script:processes[$script:processes.Count-1].normalExit=$true
  $script:window=$null; $script:process=$null
}
function Capture-Window([string]$File) {
  [PrismTestWindow]::ShowWindow($script:process.MainWindowHandle, 9) | Out-Null
  [PrismTestWindow]::SetForegroundWindow($script:process.MainWindowHandle) | Out-Null
  # Raise only the owned test window for its screenshot, then remove topmost immediately.
  [PrismTestWindow]::SetWindowPos($script:process.MainWindowHandle, [IntPtr](-1), 0,0,0,0,0x0003) | Out-Null
  try {
    Start-Sleep -Milliseconds 200
    $r=$script:window.Current.BoundingRectangle
    # Clip DWM's invisible outer shadow; do not capture the user's desktop around the window.
    $image=[Drawing.Bitmap]::new([int]$r.Width-20,[int]$r.Height-12)
    $graphics=[Drawing.Graphics]::FromImage($image)
    try { $graphics.CopyFromScreen([int]$r.X+10,[int]$r.Y+2,0,0,$image.Size); $image.Save((Join-Path $env:PRISM_VERIFY_EVIDENCE $File)) }
    finally { $graphics.Dispose(); $image.Dispose() }
  } finally { [PrismTestWindow]::SetWindowPos($script:process.MainWindowHandle, [IntPtr](-2),0,0,0,0,0x0003) | Out-Null }
}
try {
  Open-Desktop
  if (!(Read-Database).empty) { throw 'New native database contains demo environments.' }
  Invoke-Button '新建环境'
  Set-Value '环境名称' '桌面合成环境'
  Set-Value '环境分组' '合成初始组'
  Invoke-Button '设备指纹'
  $seed=Get-Value '固定指纹种子'
  Invoke-Button '创建环境'
  Wait-Element '已创建 1 个环境' 'Text' | Out-Null
  $created=Read-Database
  if ($created.seed -ne $seed) { throw 'Created seed differs from the native preview.' }
  Invoke-Button '桌面合成环境 更多操作'
  Invoke-Button '编辑环境'
  Set-Value '环境名称' '桌面重开验证'
  Set-Value '环境分组' '合成持久组'
  Set-Value '备注' '只用于 T02 实际桌面验证'
  Invoke-Button '浏览器偏好'
  Set-Value '启动网址' 'https://example.test/desktop'
  Set-Value '窗口宽度' '1440'
  Set-Value '窗口高度' '900'
  $restore=Wait-Element '恢复上次标签页 再次打开时继续之前的工作。' 'CheckBox'
  ([Windows.Automation.TogglePattern]$restore.GetCurrentPattern([Windows.Automation.TogglePattern]::Pattern)).Toggle()
  Invoke-Button '保存配置'
  Wait-Element '环境配置已保存' 'Text' | Out-Null
  $saved=Read-Database
  if ($saved.id -ne $created.id -or $saved.seed -ne $seed) { throw 'Ordinary edit changed the identity.' }
  Wait-Element '未就绪' 'DataItem' | Out-Null
  Invoke-Button '启动'
  Wait-Element '真实浏览器启动尚未接入，未安装内核时不能启动。' 'Text' | Out-Null
  Capture-Window 'native-created-edited.png'
  $before=($saved | ConvertTo-Json -Depth 10 -Compress)
  Close-Desktop
  if ((Read-Database | ConvertTo-Json -Depth 10 -Compress) -ne $before) { throw 'Normal close changed saved configuration.' }

  Open-Desktop
  Wait-Element '桌面重开验证' 'Button' | Out-Null
  if ((Read-Database | ConvertTo-Json -Depth 10 -Compress) -ne $before) { throw 'Reopen changed database identity/preferences.' }
  Invoke-Button '桌面重开验证 更多操作'
  Invoke-Button '编辑环境'
  Invoke-Button '设备指纹'
  if ((Get-Value '固定指纹种子') -ne $seed) { throw 'Reopened UI seed differs from the database.' }
  Invoke-Button '取消'
  Invoke-Button '刷新环境列表'
  Wait-Element '已重新读取本机 SQLite 档案' 'Text' | Out-Null
  Capture-Window 'native-reopened.png'
  # Actual Win32 resize; UI stays usable without any browser debugging flags.
  [PrismTestWindow]::SetWindowPos($script:process.MainWindowHandle,[IntPtr]::Zero,0,0,1040,1000,0x0006) | Out-Null
  $newButton=Wait-Element '新建环境' 'Button'
  if ($newButton.Current.IsOffscreen) { throw 'Create button is not reachable in the resized window.' }
  Capture-Window 'native-narrow.png'
  Close-Desktop
  $after=Read-Database
  if (($after | ConvertTo-Json -Depth 10 -Compress) -ne $before) { throw 'Second normal close changed the database.' }
  $evidence=@{verifiedAt=[DateTime]::UtcNow.ToString('o');platform='windows/amd64';driver='Windows UI Automation';processes=$script:processes.ToArray();database=$after;checks=@{emptyNativeStartup=$true;nativeCreateEdit=$true;normalCloseReopen=$true;UISeedMatchesSQLite=$true;missingKernelBlocked=$true;narrowWindow=$true;noDebugEndpoint=$true}}
  [IO.File]::WriteAllText((Join-Path $env:PRISM_VERIFY_EVIDENCE 'uia-verification.json'),($evidence | ConvertTo-Json -Depth 12),[Text.UTF8Encoding]::new($false))
  Write-Output 'Windows UI Automation and SQLite reopen verification completed.'
} catch {
  if ($script:window -and $script:process -and !$script:process.HasExited) {
    try {
      Capture-Window 'native-failed.png'
      $names=$script:window.FindAll([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.Condition]::TrueCondition) | ForEach-Object { "$($_.Current.ControlType.ProgrammaticName): $($_.Current.Name)" }
      [IO.File]::WriteAllText((Join-Path $env:PRISM_VERIFY_EVIDENCE 'failed-controls.txt'),($names -join "`n"),[Text.UTF8Encoding]::new($false))
    } catch { }
  }
  throw
} finally {
  # Only the exact child started above. This fallback is a failure, never recorded as normalClose.
  if ($script:process -and !$script:process.HasExited) { Stop-Process -Id $script:process.Id -ErrorAction SilentlyContinue }
}
