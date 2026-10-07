//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processObservation struct {
	Role                string   `json:"role"`
	AppContainer        uint32   `json:"appContainer"`
	SamePackage         bool     `json:"samePackage"`
	CapabilityCount     uint32   `json:"capabilityCount"`
	NetworkCapabilities []string `json:"networkCapabilities"`
	RestrictedSIDCount  uint32   `json:"restrictedSidCount"`
	Integrity           uint32   `json:"integrity"`
	Win32kDisabled      bool     `json:"win32kDisabled"`
	Error               string   `json:"error,omitempty"`
}

func tokenBytes(token windows.Token, kind uint32) ([]byte, error) {
	var size uint32
	_ = windows.GetTokenInformation(token, kind, nil, 0, &size)
	if size == 0 || size > 65536 {
		return nil, fmt.Errorf("token-information-size")
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, kind, &buffer[0], size, &size); err != nil {
		return nil, err
	}
	return buffer, nil
}

func ownedProcessSnapshot(p *labProcess, packageSID *windows.SID) []processObservation {
	var list struct {
		Assigned, Count uint32
		PIDs            [256]uintptr
	}
	if err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), nil); err != nil || list.Count > 256 {
		return []processObservation{{Error: "job-process-list-unavailable"}}
	}
	result := []processObservation{}
	for _, pid := range list.PIDs[:list.Count] {
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, uint32(pid))
		if err != nil {
			result = append(result, processObservation{Error: "process-query-unavailable"})
			continue
		}
		result = append(result, inspectOwnedProcess(p, process, packageSID))
		windows.CloseHandle(process)
	}
	return result
}

func inspectOwnedProcess(p *labProcess, process windows.Handle, packageSID *windows.SID) (r processObservation) {
	dll := windows.NewLazySystemDLL("kernel32.dll")
	var member uint32
	ok, _, _ := dll.NewProc("IsProcessInJob").Call(uintptr(process), uintptr(p.job), uintptr(unsafe.Pointer(&member)))
	if ok == 0 || member == 0 {
		r.Error = "exact-job-membership-unconfirmed"
		return
	}
	r.Role = "browser"
	var size uint32
	_ = windows.NtQueryInformationProcess(process, 60, nil, 0, &size)
	if size > 0 && size <= 65536 {
		buffer := make([]byte, size)
		if windows.NtQueryInformationProcess(process, 60, unsafe.Pointer(&buffer[0]), size, &size) == nil {
			value := (*windows.NTUnicodeString)(unsafe.Pointer(&buffer[0]))
			command := windows.UTF16ToString(unsafe.Slice(value.Buffer, int(value.Length)/2))
			for _, arg := range strings.Fields(command) {
				if strings.HasPrefix(arg, "--type=") {
					r.Role = strings.TrimPrefix(arg, "--type=")
				}
				if strings.HasPrefix(arg, "--utility-sub-type=") {
					r.Role = strings.TrimPrefix(arg, "--utility-sub-type=")
				}
			}
			runtime.KeepAlive(buffer)
		} else {
			r.Role = "unavailable"
		}
	} else {
		r.Role = "unavailable"
	}
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		r.Error = "process-token-unavailable"
		return
	}
	defer token.Close()
	var err error
	r.AppContainer, err = tokenValue(token, 29)
	if err != nil {
		r.Error = "appcontainer-query"
		return
	}
	r.Integrity, err = tokenIntegrity(token)
	if err != nil {
		r.Error = "integrity-query"
		return
	}
	r.RestrictedSIDCount, err = tokenValue(token, windows.TokenRestrictedSids)
	if err != nil {
		r.Error = "restricted-sids-query"
		return
	}
	caps, err := tokenBytes(token, 30)
	if err != nil {
		r.Error = "capabilities-query"
		return
	}
	groups := (*windows.Tokengroups)(unsafe.Pointer(&caps[0]))
	r.CapabilityCount = groups.GroupCount
	r.NetworkCapabilities = []string{}
	for _, group := range groups.AllGroups() {
		switch group.Sid.String() {
		case "S-1-15-3-1":
			r.NetworkCapabilities = append(r.NetworkCapabilities, "internetClient")
		case "S-1-15-3-2":
			r.NetworkCapabilities = append(r.NetworkCapabilities, "internetClientServer")
		case "S-1-15-3-3":
			r.NetworkCapabilities = append(r.NetworkCapabilities, "privateNetworkClientServer")
		}
	}
	runtime.KeepAlive(caps)
	if r.AppContainer != 0 {
		box, e := tokenBytes(token, 31)
		if e != nil {
			r.Error = "package-query"
			return
		}
		sid := *(**windows.SID)(unsafe.Pointer(&box[0]))
		r.SamePackage = sid != nil && sid.Equals(packageSID)
		runtime.KeepAlive(box)
	}
	var policy uint32
	if ok, _, _ = dll.NewProc("GetProcessMitigationPolicy").Call(uintptr(process), 4, uintptr(unsafe.Pointer(&policy)), unsafe.Sizeof(policy)); ok != 0 {
		r.Win32kDisabled = policy&1 != 0
	} else {
		r.Error = "mitigation-query"
	}
	return
}
