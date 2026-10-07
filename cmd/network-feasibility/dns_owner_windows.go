//go:build windows

package main

import (
	"encoding/binary"
	"unsafe"

	"golang.org/x/sys/windows"
)

func dnsServicePID() uint32 {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0
	}
	defer windows.CloseServiceHandle(manager)
	name, _ := windows.UTF16PtrFromString("Dnscache")
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0
	}
	defer windows.CloseServiceHandle(service)
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed) != nil || status.CurrentState != windows.SERVICE_RUNNING {
		return 0
	}
	return status.ProcessId
}

// Attribute only the exact synthetic query's UDP socket before replying.
// Machine-wide socket rows and process IDs never enter the report.
func dnsSender(port int) string {
	servicePID := dnsServicePID()
	if servicePID == 0 {
		return "unconfirmed"
	}
	query := windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedUdpTable")
	var size uint32
	_, _, _ = query.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 1, 0)
	for attempt := 0; attempt < 3; attempt++ {
		if size < 4 || size > 1<<20 {
			return "unconfirmed"
		}
		buffer := make([]byte, size)
		status, _, _ := query.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 1, 0)
		if status == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if status != 0 {
			return "unconfirmed"
		}
		count := int(binary.LittleEndian.Uint32(buffer[:4]))
		if count > (len(buffer)-4)/12 {
			return "unconfirmed"
		}
		owner := uint32(0)
		for i := 0; i < count; i++ {
			row := buffer[4+i*12 : 4+(i+1)*12]
			if int(binary.BigEndian.Uint16(row[4:6])) != port {
				continue
			}
			pid := binary.LittleEndian.Uint32(row[8:12])
			if owner != 0 && owner != pid {
				return "ambiguous"
			}
			owner = pid
		}
		if owner == servicePID {
			return "dns-client-service"
		}
		if owner != 0 {
			return "other-process"
		}
		return "unconfirmed"
	}
	return "unconfirmed"
}
