//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type debugObservation struct {
	Code        string  `json:"code"`
	FirstChance bool    `json:"firstChance,omitempty"`
	Module      string  `json:"module,omitempty"`
	Offset      string  `json:"offset,omitempty"`
	Message     string  `json:"message,omitempty"`
	ReturnValue *uint32 `json:"returnValue,omitempty"`
}
type debugEvent struct {
	Kind, PID, TID uint32
	Padding        uint32
	Data           [160]byte
}
type debugModule struct {
	base uintptr
	name string
}

// A debugger for this exact synthetic browser only. No attach-by-name/PID to
// existing processes. Loader breakpoints are continued; genuine faults remain
// faults and are never patched or suppressed to force startup success.
func debugContainerBrowser(exe string, args []string, sid *windows.SID, log *os.File) (observations []debugObservation, exited bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := launchWithFlags(exe, args, sid, true, log, false, 0, 0x2) // DEBUG_ONLY_THIS_PROCESS
	if err != nil {
		return []debugObservation{{Code: "debug-create-failed", Message: err.Error()}}, true
	}
	dll := windows.NewLazySystemDLL("kernel32.dll")
	wait := dll.NewProc("WaitForDebugEvent")
	resume := dll.NewProc("ContinueDebugEvent")
	defer func() {
		// On a deadline, detach before termination so an undrained debug-exit
		// event cannot retain a dying process or its profile image sections.
		dll.NewProc("DebugActiveProcessStop").Call(uintptr(p.pid))
		exited = p.stop() == nil
	}()
	modules := []debugModule{}
	firstBreakpoint := true
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var event debugEvent
		ok, _, e := wait.Call(uintptr(unsafe.Pointer(&event)), 100)
		if ok == 0 {
			if e == windows.ERROR_SEM_TIMEOUT {
				continue
			}
			observations = append(observations, debugObservation{Code: "debug-wait-failed", Message: e.Error()})
			return
		}
		status := uint32(0x00010002) // DBG_CONTINUE
		switch event.Kind {
		case 2: // CREATE_THREAD_DEBUG_EVENT: debugger-owned extra handle
			h := windows.Handle(binary.LittleEndian.Uint64(event.Data[:8]))
			if h != 0 {
				windows.CloseHandle(h)
			}
		case 3, 6: // CREATE_PROCESS / LOAD_DLL
			h := windows.Handle(binary.LittleEndian.Uint64(event.Data[:8]))
			offset := 8
			if event.Kind == 3 {
				offset = 24
			}
			base := uintptr(binary.LittleEndian.Uint64(event.Data[offset : offset+8]))
			name := "unknown"
			if h != 0 {
				buffer := make([]uint16, 2048)
				n, e := windows.GetFinalPathNameByHandle(h, &buffer[0], uint32(len(buffer)), 0)
				if e == nil && int(n) < len(buffer) {
					path := windows.UTF16ToString(buffer[:n])
					parts := strings.Split(path, `\`)
					name = parts[len(parts)-1]
				}
				windows.CloseHandle(h)
			}
			modules = append(modules, debugModule{base, name})
			if event.Kind == 3 {
				ph := windows.Handle(binary.LittleEndian.Uint64(event.Data[8:16]))
				th := windows.Handle(binary.LittleEndian.Uint64(event.Data[16:24]))
				if ph != 0 && ph != p.process {
					windows.CloseHandle(ph)
				}
				if th != 0 {
					windows.CloseHandle(th)
				}
			}
		case 1: // EXCEPTION_DEBUG_EVENT
			code := binary.LittleEndian.Uint32(event.Data[:4])
			address := uintptr(binary.LittleEndian.Uint64(event.Data[16:24]))
			first := binary.LittleEndian.Uint32(event.Data[152:156]) != 0
			m := debugModule{}
			for _, candidate := range modules {
				if candidate.base <= address && candidate.base > m.base {
					m = candidate
				}
			}
			if code == 0x80000003 && firstBreakpoint && strings.EqualFold(m.name, "ntdll.dll") {
				firstBreakpoint = false
				break
			}
			observation := debugObservation{Code: fmt.Sprintf("0x%08x", code), FirstChance: first, Module: m.name, Offset: fmt.Sprintf("0x%x", address-m.base)}
			if code == 0x80000003 && strings.EqualFold(m.name, "chrome.dll") {
				thread, e := windows.OpenThread(windows.THREAD_GET_CONTEXT, false, event.TID)
				if e == nil {
					storage := make([]byte, 1248)
					offset := (-uintptr(unsafe.Pointer(&storage[0]))) & 15
					ptr := unsafe.Add(unsafe.Pointer(&storage[0]), offset)
					context := unsafe.Slice((*byte)(ptr), 1232)
					binary.LittleEndian.PutUint32(context[48:52], 0x00100003)
					ok, _, _ := dll.NewProc("GetThreadContext").Call(uintptr(thread), uintptr(ptr))
					if ok != 0 {
						value := binary.LittleEndian.Uint32(context[120:124])
						observation.ReturnValue = &value
					}
					runtime.KeepAlive(storage)
					windows.CloseHandle(thread)
				}
			}
			observations = append(observations, observation)
			status = 0x80010001 // DBG_EXCEPTION_NOT_HANDLED
		case 8: // OUTPUT_DEBUG_STRING_EVENT
			address := uintptr(binary.LittleEndian.Uint64(event.Data[:8]))
			unicode := binary.LittleEndian.Uint16(event.Data[8:10])
			length := int(binary.LittleEndian.Uint16(event.Data[10:12]))
			if unicode != 0 {
				length *= 2
			}
			if length <= 0 || length > 16384 {
				break
			}
			buffer := make([]byte, length)
			var read uintptr
			if windows.ReadProcessMemory(p.process, address, &buffer[0], uintptr(len(buffer)), &read) == nil {
				// Keep raw debug messages only in the ignored local evidence file.
				_, _ = log.Write(buffer[:read])
				if strings.Contains(string(buffer), "CreateAlternateDesktop") {
					observations = append(observations, debugObservation{Code: "chromium-check", Message: "CreateAlternateDesktop"})
				}
			}
		case 5:
			observations = append(observations, debugObservation{Code: fmt.Sprintf("exit-0x%08x", binary.LittleEndian.Uint32(event.Data[:4]))})
			resume.Call(uintptr(event.PID), uintptr(event.TID), uintptr(status))
			return
		}
		if ok, _, e = resume.Call(uintptr(event.PID), uintptr(event.TID), uintptr(status)); ok == 0 {
			observations = append(observations, debugObservation{Code: "debug-resume-failed", Message: e.Error()})
			return
		}
	}
	observations = append(observations, debugObservation{Code: "debug-deadline"})
	return
}
