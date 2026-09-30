; Independent per-user installer. No Wails template recursive data deletion.
Unicode true
RequestExecutionLevel user
ManifestDPIAware true
SetCompressor /SOLID zlib
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "WordFunc.nsh"
!include "x64.nsh"
!include "WinVer.nsh"
!include "nsDialogs.nsh"
!insertmacro VersionCompare

Name "棱镜浏览器 · 开发预览"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\PrismBrowserPreview"
VIProductVersion "0.3.0.${PREVIEW_REVISION}"
VIAddVersionKey "ProductName" "Prism Browser Development Preview"
VIAddVersionKey "ProductVersion" "${RELEASE_VERSION}"
VIAddVersionKey "FileVersion" "0.3.0.${PREVIEW_REVISION}"
VIAddVersionKey "FileDescription" "Prism Browser Windows amd64 per-user installer"
VIAddVersionKey "LegalCopyright" "MIT; third-party components retain their licenses"
!define PRODUCT_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\PrismBrowserPreview"
!define SHORTCUT_NAME "棱镜浏览器 · 开发预览.lnk"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "棱镜浏览器 · 开发预览 ${RELEASE_VERSION}"
!define MUI_WELCOMEPAGE_TEXT "仅为当前 Windows 用户安装，无需管理员权限。$\r$\n$\r$\n本包只交付桌面工作台和 SQLite 配置，不包含 fingerprint-chromium；真实浏览器、代理、Cookie 与备份尚未接入。$\r$\n$\r$\n这是未签名开发预览。安装/升级不会清除本机档案，请先正常关闭工作台。"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${PAYLOAD_DIR}\LICENSE"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
UninstPage custom un.DataPage un.DataPageLeave
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "SimpChinese"

Var MaintenanceLock
Var RemoveData
Var RemoveCheckbox

!macro Fail CODE TEXT
  MessageBox MB_OK|MB_ICONSTOP "${TEXT}" /SD IDOK
  SetErrorLevel ${CODE}
  Quit
!macroend
!macro Guard PREFIX
  SetShellVarContext current
  SetRegView 64
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\PrismBrowserPreview"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=prism-maintenance.exe "${PAYLOAD_DIR}\prism-maintenance.exe"
  ExecWait '"$PLUGINSDIR\prism-maintenance.exe" --validate-install' $0
  ${If} $0 != 0
    !insertmacro Fail 21 "程序目录包含链接或无法检查。未修改程序或用户数据，请检查目录权限后重试。"
  ${EndIf}
  ExecWait '"$PLUGINSDIR\prism-maintenance.exe" --validate-data' $0
  ${If} $0 != 0
    !insertmacro Fail 21 "工作区包含链接或无法检查。未修改用户数据，请检查数据目录后重试。"
  ${EndIf}
  ; Also protect upgrades from the older T02 binary, before lifecycle locking existed.
  System::Call 'kernel32::OpenMutexW(i 0x100000, i 0, w "wails-app-cadb5081-585a-4e92-89c7-40c8ace49dc1sim") p.r0'
  ${If} $0 != 0
    System::Call 'kernel32::CloseHandle(p r0)'
    !insertmacro Fail 22 "棱镜浏览器正在运行，请正常关闭后重试。没有结束其他程序，原数据未修改。"
  ${EndIf}
  System::Call 'kernel32::CreateFileW(w "$LOCALAPPDATA\PrismBrowser.maintenance.lock", i 0xC0000000, i 0, p 0, i 4, i 0x80, p 0) p.r0'
  StrCpy $MaintenanceLock $0
  ${If} $MaintenanceLock == -1
    !insertmacro Fail 22 "工作区正在使用或维护中。请正常关闭工作台后重试；原数据未修改。"
  ${EndIf}
!macroend

Function .onInit
  ${IfNot} ${IsNativeAMD64}
    !insertmacro Fail 24 "此开发预览仅支持 Windows x64（AMD64）。请使用匹配架构的构建。"
  ${EndIf}
  ${IfNot} ${AtLeastWin10}
    !insertmacro Fail 24 "此开发预览需要 Windows 10 或更新版本；未修改用户数据。"
  ${EndIf}
  !insertmacro Guard ""
  ExecWait '"$PLUGINSDIR\prism-maintenance.exe" --check-runtime' $0
  ${If} $0 != 0
    !insertmacro Fail 20 "需要 Microsoft Edge WebView2 Runtime（94.0.992.31 或更新版）。请从 https://developer.microsoft.com/microsoft-edge/webview2/ 安装 Evergreen Runtime 后重试。不会自动下载，原数据未修改。"
  ${EndIf}
  ReadRegStr $0 HKCU "${PRODUCT_KEY}" "NumericVersion"
  ${If} $0 != ""
    ${VersionCompare} $0 "0.3.0.${PREVIEW_REVISION}" $1
    ${If} $1 == 1
      !insertmacro Fail 25 "已安装较新的开发预览，不会自动降级。请使用相同或更新版本；原数据未修改。"
    ${EndIf}
  ${EndIf}
FunctionEnd

Section "Install"
  ; Stage only in the installer's private temporary directory. The narrow Go helper
  ; pins real known-folder paths, verifies every payload hash and publishes integration.
  SetOutPath "$PLUGINSDIR"
  ClearErrors
  File "${PAYLOAD_DIR}\prism-browser.exe"
  File "${PAYLOAD_DIR}\prism-maintenance.exe"
  File "${PAYLOAD_DIR}\LICENSE"
  File "${PAYLOAD_DIR}\THIRD_PARTY_NOTICES.md"
  File "${PAYLOAD_DIR}\GO-THIRD-PARTY-NOTICES.txt"
  File "${PAYLOAD_DIR}\NSIS-LICENSE.txt"
  File "${PAYLOAD_DIR}\INSTALLATION.md"
  File "${PAYLOAD_DIR}\release.json"
  ${If} ${Errors}
    !insertmacro Fail 26 "程序文件写入失败，安装未完成。已有版本和用户数据仍在，请释放空间或检查权限后重试。"
  ${EndIf}
  ClearErrors
  WriteUninstaller "$PLUGINSDIR\uninstall.exe"
  ${If} ${Errors}
    !insertmacro Fail 26 "安装入口暂存失败，安装未完成。旧版本、快捷方式与用户数据未修改。"
  ${EndIf}
  ExecWait '"$PLUGINSDIR\prism-maintenance.exe" --publish-program' $0
  ${If} $0 != 0
    !insertmacro Fail 26 "程序或安装入口未能完整写入，安装未完成。原工作区未修改，旧入口保持或回滚；请检查磁盘权限或空间后重试。"
  ${EndIf}
  SetErrorLevel 0
SectionEnd

Function un.onInit
  !insertmacro Guard "un."
  ReadRegStr $0 HKCU "${PRODUCT_KEY}" "InstallLocation"
  ${If} $0 != $INSTDIR
    !insertmacro Fail 21 "未找到属于当前 Windows 用户的安装记录。没有删除任何程序或数据。"
  ${EndIf}
  StrCpy $RemoveData 0
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/REMOVE-DATA=" $1
  ${IfNot} ${Errors}
    ${If} $1 != "CONFIRMED"
      !insertmacro Fail 2 "删除数据必须明确确认；未删除任何数据。"
    ${EndIf}
    StrCpy $RemoveData 1
  ${EndIf}
FunctionEnd
Function un.DataPage
  !insertmacro MUI_HEADER_TEXT "卸载与用户数据" "默认只删除程序，保留本机档案"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 48u "默认保留：环境、固定 seed、代理引用和浏览数据。重装后继续读取同一工作区。$\r$\n数据位置：%LOCALAPPDATA%\PrismBrowser（不是程序目录）。"
  Pop $0
  ${NSD_CreateCheckbox} 0 55u 100% 30u "删除此 Windows 用户的全部棱镜数据（不可撤销）"
  Pop $RemoveCheckbox
  ${NSD_Uncheck} $RemoveCheckbox
  ${NSD_CreateLabel} 0 93u 100% 28u "仅在确认不再需要任何档案时勾选。不会卸载 WebView2，也不会删除其他软件的数据。"
  Pop $0
  nsDialogs::Show
FunctionEnd
Function un.DataPageLeave
  ${NSD_GetState} $RemoveCheckbox $RemoveData
  ${If} $RemoveData == ${BST_CHECKED}
    MessageBox MB_YESNO|MB_ICONEXCLAMATION|MB_DEFBUTTON2 "确认永久删除当前 Windows 用户的全部棱镜档案和浏览数据？此操作不可撤销。" IDYES +2
    Abort
  ${EndIf}
FunctionEnd
Section "Uninstall"
  ; Program and integration cleanup precedes any explicitly requested data removal.
  ExecWait '"$PLUGINSDIR\prism-maintenance.exe" --clean-program' $0
  ${If} $0 != 0
    !insertmacro Fail 27 "程序或安装入口仍被占用或无法删除，卸载未完成。用户数据尚未删除；请正常关闭工作台并检查权限后重试。"
  ${EndIf}
  ${If} $RemoveData == 1
    ; Child obtains the same exclusive lock itself. If an app races to reopen, removal fails closed.
    System::Call 'kernel32::CloseHandle(p $MaintenanceLock)'
    StrCpy $MaintenanceLock 0
    ExecWait '"$PLUGINSDIR\prism-maintenance.exe" --remove-data' $0
    ${If} $0 != 0
      !insertmacro Fail 23 "用户数据未能全部删除，卸载未完成。剩余数据已保留，请关闭工作台、检查权限和链接后重试。"
    ${EndIf}
  ${EndIf}
  ExecWait '"$PLUGINSDIR\prism-maintenance.exe" --finish-uninstall' $0
  ${If} $0 != 0
    !insertmacro Fail 27 "卸载收尾未完成，已保留可重试的卸载入口。请检查权限或占用后重试。"
  ${EndIf}
  SetErrorLevel 0
SectionEnd
