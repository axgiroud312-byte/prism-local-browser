param([Parameter(Mandatory = $true)][string]$EvidenceDirectory)

# Explicitly authorized T11 administrator READ-ONLY collection. Target handles
# request only query rights. The only writes are new private evidence files.
# No service control, process termination, policy import, firewall changes,
# logging enablement, driver installation, or UI automation is implemented.
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$allowed = [IO.Path]::GetFullPath((Join-Path $root 'output/goal/T11')) + [IO.Path]::DirectorySeparatorChar
$destination = [IO.Path]::GetFullPath($EvidenceDirectory)
if (-not $destination.StartsWith($allowed, [StringComparison]::OrdinalIgnoreCase)) { throw 'Evidence must be a new directory under output/goal/T11.' }
if (Test-Path -LiteralPath $destination) { throw 'Existing evidence directory refused.' }
for ($parent = [IO.DirectoryInfo]::new($destination).Parent; $null -ne $parent; $parent = $parent.Parent) {
    if ($parent.Exists -and (($parent.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)) { throw 'Reparse-point evidence ancestor refused.' }
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$elevated = [Security.Principal.WindowsPrincipal]::new($identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $elevated) { throw 'This collection requires the specifically authorized elevated read-only session.' }

Add-Type -TypeDefinition @'
using System;
using System.IO;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
public static class PrismBoundaryQuery {
    [StructLayout(LayoutKind.Sequential)] public struct Status { public uint Type, State, Accepts, Win32Exit, SpecificExit, Checkpoint, WaitHint, Pid, Flags; }
    [StructLayout(LayoutKind.Sequential)] public struct Luid { public uint Low; public int High; }
    [StructLayout(LayoutKind.Sequential)] public struct Privileges { public uint Count; public Luid Id; public uint Attributes; }
    [DllImport("advapi32.dll", CharSet=CharSet.Unicode, SetLastError=true)] public static extern IntPtr OpenSCManager(string machine,string database,uint access);
    public static IntPtr OpenLocalManager() { return OpenSCManager(null,null,1); }
    [DllImport("advapi32.dll", CharSet=CharSet.Unicode, SetLastError=true)] public static extern IntPtr OpenService(IntPtr manager,string name,uint access);
    [DllImport("advapi32.dll", SetLastError=true)] public static extern bool QueryServiceStatusEx(IntPtr service,int level,out Status info,uint size,out uint needed);
    [DllImport("advapi32.dll", EntryPoint="QueryServiceConfig2W", SetLastError=true)] public static extern bool QueryProtection(IntPtr service,int level,out uint info,uint size,out uint needed);
    [DllImport("advapi32.dll")] public static extern bool CloseServiceHandle(IntPtr handle);
    [DllImport("kernel32.dll", SetLastError=true)] public static extern IntPtr OpenProcess(uint access,bool inherit,uint pid);
    [DllImport("kernel32.dll", SetLastError=true)] public static extern bool IsProcessCritical(IntPtr process,out bool critical);
    [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr handle);
    [DllImport("kernel32.dll")] public static extern IntPtr GetCurrentProcess();
    [DllImport("advapi32.dll", SetLastError=true)] public static extern bool OpenProcessToken(IntPtr process,uint access,out IntPtr token);
    [DllImport("advapi32.dll", CharSet=CharSet.Unicode, SetLastError=true)] public static extern bool LookupPrivilegeValue(string system,string name,out Luid value);
    [DllImport("advapi32.dll", SetLastError=true)] public static extern bool AdjustTokenPrivileges(IntPtr token,bool disableAll,ref Privileges state,uint length,out Privileges previous,out uint returned);
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)] private static extern bool CreateDirectory(string path,IntPtr attributes);
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)] private static extern SafeFileHandle CreateFile(string path,uint access,uint share,IntPtr attributes,uint creation,uint flags,IntPtr template);
    [StructLayout(LayoutKind.Sequential)] private struct AttributeTag { public uint Attributes,Tag; }
    [DllImport("kernel32.dll", SetLastError=true)] private static extern bool GetFileInformationByHandleEx(SafeFileHandle file,int type,out AttributeTag info,uint size);
    public sealed class EvidencePins : IDisposable {
        private readonly List<SafeFileHandle> handles = new List<SafeFileHandle>();
        public EvidencePins(string destination) {
            try {
                var parents = new Stack<string>();
                for (var p = new DirectoryInfo(destination).Parent; p != null; p = p.Parent) parents.Push(p.FullName);
                while (parents.Count != 0) Pin(parents.Pop());
                if (!CreateDirectory(destination,IntPtr.Zero)) throw new Win32Exception(Marshal.GetLastWin32Error());
                Pin(destination);
            } catch { Dispose(); throw; }
        }
        private void Pin(string path) {
            // Read access and no write/delete sharing preserve this directory
            // object while child evidence files are created. Never follow a
            // reparse point; inspect the opened object rather than a prior path.
            var h = CreateFile(path,0x80000000,1,IntPtr.Zero,3,0x02200000,IntPtr.Zero);
            if (h.IsInvalid) { var error=Marshal.GetLastWin32Error(); h.Dispose(); throw new Win32Exception(error); }
            handles.Add(h);
            AttributeTag info;
            if (!GetFileInformationByHandleEx(h,9,out info,8)) throw new Win32Exception(Marshal.GetLastWin32Error());
            if ((info.Attributes & 0x410) != 0x10) throw new IOException("Evidence directory is not an ordinary directory.");
        }
        public void Dispose() { for (int i=handles.Count-1;i>=0;i--) handles[i].Dispose(); handles.Clear(); }
    }
}
'@

# Process-local privilege only, used for QUERY_LIMITED_INFORMATION handles.
# Restore it before the collector exits; no target process is modified.
$evidencePins = [PrismBoundaryQuery+EvidencePins]::new($destination)
$debugToken = [IntPtr]::Zero
$debugEnabled = $false
$privilegeAdjusted = $false
$previous = [PrismBoundaryQuery+Privileges]::new()

$report = [ordered]@{ format='prism-network-boundary-readonly-v1'; generatedAt=[DateTimeOffset]::UtcNow.ToString('o'); elevated=$elevated; queryDebugPrivilege=$false; privilegeRestore='not-adjusted'; readOnly=$true; services=@(); exports=@(); errors=@() }
try {
    if ([PrismBoundaryQuery]::OpenProcessToken([PrismBoundaryQuery]::GetCurrentProcess(),40,[ref]$debugToken)) {
        $luid = [PrismBoundaryQuery+Luid]::new()
        if ([PrismBoundaryQuery]::LookupPrivilegeValue($null,'SeDebugPrivilege',[ref]$luid)) {
            $privileges = [PrismBoundaryQuery+Privileges]::new()
            $privileges.Count = 1; $privileges.Id = $luid; $privileges.Attributes = 2
            [uint32]$returned = 0
            $privilegeAdjusted = [PrismBoundaryQuery]::AdjustTokenPrivileges($debugToken,$false,[ref]$privileges,16,[ref]$previous,[ref]$returned)
            $adjustError = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
            $debugEnabled = $privilegeAdjusted -and $adjustError -eq 0
            $report.queryDebugPrivilege = $debugEnabled
        }
    }
    $version = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    $report.windows = @{ editionId=$version.EditionID; displayVersion=$version.DisplayVersion; build=$version.CurrentBuildNumber; ubr=$version.UBR }
    $report.components = @(foreach ($relative in @('bfe.dll','mpssvc.dll','dnsapi.dll','drivers/mpsdrv.sys','drivers/tcpip.sys','drivers/netio.sys')) {
        $path = Join-Path ([Environment]::SystemDirectory) $relative
        if (Test-Path -LiteralPath $path) { @{ component=$relative; fileVersion=(Get-Item -LiteralPath $path).VersionInfo.FileVersion } }
    })
    $manager = [PrismBoundaryQuery]::OpenLocalManager()
    if ($manager -eq [IntPtr]::Zero) { throw "SCM query open failed: $([Runtime.InteropServices.Marshal]::GetLastWin32Error())" }
    try {
        $allServices = Get-CimInstance Win32_Service
        $report.services = @(foreach ($name in @('BFE','MpsSvc','Dnscache')) {
            $row = [ordered]@{ name=$name; statusRead=$false; state=$null; acceptsStop=$null; processId=$null; sharedServices=@(); protectionRead=$false; launchProtection=$null; criticalRead=$false; critical=$null; errors=@() }
            $service = [PrismBoundaryQuery]::OpenService($manager,$name,4)
            if ($service -eq [IntPtr]::Zero) { $row.errors += "status-open:$([Runtime.InteropServices.Marshal]::GetLastWin32Error())" }
            else {
                try {
                    $status = [PrismBoundaryQuery+Status]::new()
                    [uint32]$needed = 0
                    $row.statusRead = [PrismBoundaryQuery]::QueryServiceStatusEx($service,0,[ref]$status,36,[ref]$needed)
                    if ($row.statusRead) {
                        $row.state = $status.State; $row.acceptsStop = ($status.Accepts -band 1) -ne 0; $row.processId = $status.Pid
                        if ($status.Pid -gt 0) {
                            $row.sharedServices = @($allServices | Where-Object ProcessId -EQ $status.Pid | Select-Object -ExpandProperty Name)
                            $process = [PrismBoundaryQuery]::OpenProcess(4096,$false,$status.Pid)
                            if ($process -eq [IntPtr]::Zero) { $row.errors += "process-query-open:$([Runtime.InteropServices.Marshal]::GetLastWin32Error())" }
                            else {
                                try {
                                    [bool]$critical = $false
                                    $row.criticalRead = [PrismBoundaryQuery]::IsProcessCritical($process,[ref]$critical)
                                    if ($row.criticalRead) { $row.critical = $critical } else { $row.errors += "critical-query:$([Runtime.InteropServices.Marshal]::GetLastWin32Error())" }
                                } finally { [void][PrismBoundaryQuery]::CloseHandle($process) }
                            }
                            $after = [PrismBoundaryQuery+Status]::new()
                            $stable = [PrismBoundaryQuery]::QueryServiceStatusEx($service,0,[ref]$after,36,[ref]$needed) -and $after.Pid -eq $status.Pid -and $after.State -eq $status.State
                            if (-not $stable) { $row.criticalRead=$false; $row.critical=$null; $row.errors += 'service-changed-during-query' }
                        }
                    } else { $row.errors += "status-query:$([Runtime.InteropServices.Marshal]::GetLastWin32Error())" }
                } finally { [void][PrismBoundaryQuery]::CloseServiceHandle($service) }
            }
            $config = [PrismBoundaryQuery]::OpenService($manager,$name,1)
            if ($config -eq [IntPtr]::Zero) { $row.errors += "config-open:$([Runtime.InteropServices.Marshal]::GetLastWin32Error())" }
            else {
                try {
                    [uint32]$protection = 0; [uint32]$needed = 0
                    $row.protectionRead = [PrismBoundaryQuery]::QueryProtection($config,12,[ref]$protection,4,[ref]$needed)
                    if ($row.protectionRead) { $row.launchProtection=$protection } else { $row.errors += "protection-query:$([Runtime.InteropServices.Marshal]::GetLastWin32Error())" }
                } finally { [void][PrismBoundaryQuery]::CloseServiceHandle($config) }
            }
            $row
        })
    } finally { [void][PrismBoundaryQuery]::CloseServiceHandle($manager) }
    foreach ($service in $report.services) {
        if (-not $service.statusRead -or -not $service.protectionRead -or -not $service.criticalRead -or $service.errors.Count -gt 0) {
            $report.errors += "required-service-query-incomplete:$($service.name)"
        }
    }

    foreach ($kind in @('state','boottimepolicy','netevents')) {
        $file = Join-Path $destination ("wfp-" + $kind + '.xml')
        $log = Join-Path $destination ("wfp-" + $kind + '-command.txt')
        # The directory was atomically created and is pinned. A second copy of
        # this collector cannot enter it. Refuse any unexpected pre-existing
        # export; arbitrary hostile writers inside this directory are not part
        # of this developer collector's security contract.
        if (Test-Path -LiteralPath $file) { throw 'Existing WFP export refused.' }
        $text = & (Join-Path ([Environment]::SystemDirectory) 'netsh.exe') wfp show $kind "file=$file" 2>&1
        $code = $LASTEXITCODE
        $text | Out-File -LiteralPath $log -Encoding utf8 -NoClobber
        $entry = [ordered]@{ command="netsh wfp show $kind"; exitCode=$code; fileCreated=(Test-Path -LiteralPath $file) }
        if ($entry.fileCreated) { $entry.sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLower(); $entry.bytes=(Get-Item -LiteralPath $file).Length }
        if ($code -ne 0 -or -not $entry.fileCreated -or $entry.bytes -le 0) { $report.errors += "required-wfp-export-incomplete:$kind" }
        $report.exports += $entry
    }
} catch {
    $report.errors += $_.Exception.Message
} finally {
    try {
        if ($debugToken -ne [IntPtr]::Zero) {
            try {
                if ($privilegeAdjusted) {
                    $discard=[PrismBoundaryQuery+Privileges]::new(); [uint32]$returned=0
                    $restored=[PrismBoundaryQuery]::AdjustTokenPrivileges($debugToken,$false,[ref]$previous,16,[ref]$discard,[ref]$returned)
                    $restoreError=[Runtime.InteropServices.Marshal]::GetLastWin32Error()
                    if ($restored -and $restoreError -eq 0) { $report.privilegeRestore='confirmed' }
                    else { $report.privilegeRestore='failed'; $report.errors += "privilege-restore:$restoreError" }
                }
            } finally { [void][PrismBoundaryQuery]::CloseHandle($debugToken) }
        }
        $report | ConvertTo-Json -Depth 10 | Out-File -LiteralPath (Join-Path $destination 'summary.json') -Encoding utf8 -NoClobber
    } finally { $evidencePins.Dispose() }
}
if ($report.errors.Count -gt 0) { exit 1 }
