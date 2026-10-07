//go:build windows

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type labProcess struct {
	process, job  windows.Handle
	input, output *os.File
	pid           uint32
	seq           int
	reader        *bufio.Reader
}

func launch(exe string, args []string, sid *windows.SID, cdp bool, stderr *os.File) (p *labProcess, resultErr error) {
	return launchWithIntegrity(exe, args, sid, cdp, stderr, false)
}

func launchWithIntegrity(exe string, args []string, sid *windows.SID, cdp bool, stderr *os.File, medium bool) (p *labProcess, resultErr error) {
	return launchWithToken(exe, args, sid, cdp, stderr, medium, 0)
}

func launchWithToken(exe string, args []string, sid *windows.SID, cdp bool, stderr *os.File, medium bool, primary windows.Token) (p *labProcess, resultErr error) {
	return launchWithFlags(exe, args, sid, cdp, stderr, medium, primary, 0)
}

func launchWithFlags(exe string, args []string, sid *windows.SID, cdp bool, stderr *os.File, medium bool, primary windows.Token, extraFlags uint32) (p *labProcess, resultErr error) {
	transferred := false
	childRead, parentWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer childRead.Close()
	parentRead, childWrite, err := os.Pipe()
	if err != nil {
		parentWrite.Close()
		return nil, err
	}
	defer childWrite.Close()
	defer func() {
		if resultErr != nil && !transferred {
			parentRead.Close()
			parentWrite.Close()
		}
	}()
	handles := []windows.Handle{windows.Handle(childRead.Fd()), windows.Handle(childWrite.Fd())}
	if stderr != nil {
		handles = append(handles, windows.Handle(stderr.Fd()))
	}
	if cdp && stderr != nil {
		nul, e := os.OpenFile("NUL", os.O_RDWR, 0)
		if e != nil {
			return nil, e
		}
		defer nul.Close()
		handles = append(handles, windows.Handle(nul.Fd()))
	}
	for _, h := range handles {
		if err = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return nil, err
		}
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil && !transferred {
			windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	count := uint32(2)
	if sid != nil {
		count++
	}
	attrs, err := windows.NewProcThreadAttributeList(count)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	if err = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	if err = attrs.Update(0x0002000D, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return nil, err
	}
	caps := securityCapabilities{SID: sid}
	if sid != nil {
		if err = attrs.Update(0x00020009, unsafe.Pointer(&caps), unsafe.Sizeof(caps)); err != nil {
			return nil, err
		}
	}
	if cdp {
		args = append(args, "--remote-debugging-pipe", fmt.Sprintf("--remote-debugging-io-pipes=%d,%d", handles[0], handles[1]))
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{exe}, args...)))
	if err != nil {
		return nil, err
	}
	image, _ := windows.UTF16PtrFromString(exe)
	si := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESTDHANDLES, StdInput: handles[0], StdOutput: handles[1], StdErr: handles[1]}, ProcThreadAttributeList: attrs.List()}
	if stderr != nil {
		si.StdErr = handles[2]
	}
	if cdp {
		// CDP owns these handles; do not also give the CRT ownership as stdin
		// and stdout. Match the production pipe launcher exactly.
		si.Flags = 0
		si.StdInput, si.StdOutput, si.StdErr = 0, 0, 0
		if stderr != nil {
			si.Flags = windows.STARTF_USESTDHANDLES
			si.StdInput = handles[3]
			si.StdOutput = handles[3]
			si.StdErr = handles[2]
		}
	}
	var info windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW)
	flags |= extraFlags
	if medium {
		flags |= windows.CREATE_SUSPENDED
	}
	if primary != 0 {
		err = windows.CreateProcessAsUser(primary, image, command, nil, nil, true, flags, nil, nil, &si.StartupInfo, &info)
	} else {
		err = windows.CreateProcess(image, command, nil, nil, true, flags, nil, nil, &si.StartupInfo, &info)
	}
	runtime.KeepAlive(caps)
	runtime.KeepAlive(handles)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(info.Thread)
	p = &labProcess{process: info.Process, job: job, input: parentWrite, output: parentRead, pid: info.ProcessId, reader: bufio.NewReader(parentRead)}
	transferred = true
	if medium {
		var token windows.Token
		if err = windows.OpenProcessToken(info.Process, windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_QUERY, &token); err != nil {
			return p, fmt.Errorf("open token for integrity: %w", err)
		}
		defer token.Close()
		labelSID, e := windows.StringToSid("S-1-16-8192")
		if e != nil {
			return p, e
		}
		label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: labelSID, Attributes: windows.SE_GROUP_INTEGRITY}}
		if err = windows.SetTokenInformation(token, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), label.Size()); err != nil {
			return p, fmt.Errorf("set medium integrity: %w", err)
		}
		if _, err = windows.ResumeThread(info.Thread); err != nil {
			return p, err
		}
	}
	return p, nil
}

func (p *labProcess) stop() error {
	defer windows.CloseHandle(p.process)
	defer windows.CloseHandle(p.job)
	defer p.input.Close()
	defer p.output.Close()
	_ = windows.TerminateJobObject(p.job, 1)
	deadline := time.Now().Add(10 * time.Second)
	for {
		var accounting struct {
			User, Kernel, PeriodUser, PeriodKernel     int64
			Faults, Total, ActiveProcesses, Terminated uint32
		}
		if err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return err
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("owned job exit unconfirmed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
func (p *labProcess) wait(ctx context.Context) (uint32, error) {
	for {
		status, err := windows.WaitForSingleObject(p.process, 50)
		if err != nil {
			return 0, err
		}
		if status == windows.WAIT_OBJECT_0 {
			var code uint32
			err = windows.GetExitCodeProcess(p.process, &code)
			return code, err
		}
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
	}
}
func (p *labProcess) call(ctx context.Context, method string, params any, session string, dst any) error {
	p.seq++
	id := p.seq
	request := map[string]any{"id": id, "method": method, "params": params}
	if session != "" {
		request["sessionId"] = session
	}
	raw, _ := json.Marshal(request)
	type answer struct {
		raw []byte
		err error
	}
	done := make(chan answer, 1)
	go func() {
		if _, err := p.input.Write(append(raw, 0)); err != nil {
			done <- answer{err: err}
			return
		}
		for {
			packet, err := p.reader.ReadBytes(0)
			if err != nil {
				done <- answer{err: err}
				return
			}
			if len(packet) > 2<<20 {
				done <- answer{err: fmt.Errorf("oversized CDP response")}
				return
			}
			var v struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(packet[:len(packet)-1], &v) != nil {
				done <- answer{err: fmt.Errorf("invalid CDP packet")}
				return
			}
			if v.ID != id {
				continue
			}
			if len(v.Error) > 0 {
				done <- answer{err: fmt.Errorf("CDP returned error")}
				return
			}
			done <- answer{raw: v.Result}
			return
		}
	}()
	select {
	case a := <-done:
		if a.err != nil {
			return a.err
		}
		if dst != nil {
			return json.Unmarshal(a.raw, dst)
		}
		return nil
	case <-ctx.Done():
		p.input.Close()
		p.output.Close()
		<-done
		return ctx.Err()
	}
}
func (p *labProcess) helperReport(ctx context.Context) ([]byte, error) {
	done := make(chan []byte, 1)
	go func() { data, _ := io.ReadAll(io.LimitReader(p.output, 1<<20)); done <- data }()
	if _, err := p.wait(ctx); err != nil {
		p.output.Close()
		<-done
		return nil, err
	}
	select {
	case data := <-done:
		return data, nil
	case <-ctx.Done():
		p.output.Close()
		<-done
		return nil, ctx.Err()
	}
}
