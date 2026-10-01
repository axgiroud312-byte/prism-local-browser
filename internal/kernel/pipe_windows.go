//go:build windows

package kernel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Chromium on Windows adopts precisely these two inherited handles. No TCP
// debugger or named/public control endpoint is created, including during probe.
type pipeReply struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type pipeProcess struct {
	process     windows.Handle
	job         windows.Handle
	read, write *os.File
	pid         uint32
	createdAt   string
	sequence    int
	responses   chan pipeReply
	stopped     chan struct{}
	closeOnce   sync.Once
	commandGate chan struct{}
	writeLost   atomic.Bool
	readEnded   chan struct{}
}

func startPipe(executable string, args []string) (_ *pipeProcess, resultErr error) {
	return startPipeWithSession(executable, args, "")
}

func startPipeWithSession(executable string, args []string, sessionID string) (_ *pipeProcess, resultErr error) {
	return startPipeWithBinding(executable, args, sessionID, nil)
}

func startPipeWithBinding(executable string, args []string, sessionID string, bindJob func(windows.Handle) error) (_ *pipeProcess, resultErr error) {
	resourcesTransferred := false
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
		if resultErr != nil && !resourcesTransferred {
			parentWrite.Close()
			parentRead.Close()
		}
	}()
	handles := []windows.Handle{windows.Handle(childRead.Fd()), windows.Handle(childWrite.Fd())}
	for _, handle := range handles {
		if err = windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return nil, err
		}
	}
	job, err := createManagedJob(sessionID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil && !resourcesTransferred {
			windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	if bindJob != nil {
		if err = bindJob(job); err != nil {
			return nil, err
		}
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	// PROC_THREAD_ATTRIBUTE_JOB_LIST = ProcThreadAttributeValue(13, false,true,false).
	// Atomic assignment at CreateProcess prevents a child escaping before assignment.
	if err = attributes.Update(0x0002000D, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return nil, err
	}
	args = append([]string{executable}, args...)
	args = append(args, "--remote-debugging-pipe", fmt.Sprintf("--remote-debugging-io-pipes=%d,%d", uint32(handles[0]), uint32(handles[1])))
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return nil, err
	}
	image, _ := windows.UTF16PtrFromString(executable)
	si := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attributes.List()}
	var info windows.ProcessInformation
	if err = windows.CreateProcess(image, command, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW, nil, nil, &si.StartupInfo, &info); err != nil {
		return nil, err
	}
	windows.CloseHandle(info.Thread)
	p := &pipeProcess{process: info.Process, job: job, read: parentRead, write: parentWrite, pid: info.ProcessId, responses: make(chan pipeReply, 16), stopped: make(chan struct{}), commandGate: make(chan struct{}, 1), readEnded: make(chan struct{})}
	// Once CreateProcess succeeds, transfer EVERY handle even if identity
	// observation fails. The managed owner must confirm the whole Job exit;
	// closing a Job handle is not synchronous resource-exit confirmation.
	resourcesTransferred = true
	go p.readLoop()
	var created, exit, kernelTime, userTime windows.Filetime
	if err = windows.GetProcessTimes(info.Process, &created, &exit, &kernelTime, &userTime); err != nil {
		return p, problem("PROCESS_IDENTITY_UNAVAILABLE", "creation-time-unavailable", "进程已创建，但实际创建时间无法读取；仅清理本次Job，未释放仍占用的目录。")
	}
	p.createdAt = time.Unix(0, created.Nanoseconds()).UTC().Format(time.RFC3339Nano)
	return p, nil
}
func (p *pipeProcess) close() {
	p.closeOnce.Do(func() {
		close(p.stopped)
		windows.TerminateJobObject(p.job, 1)
		p.write.Close()
		p.read.Close()
		windows.CloseHandle(p.job)
		windows.WaitForSingleObject(p.process, 5000)
		windows.CloseHandle(p.process)
	})
}
func (p *pipeProcess) readLoop() {
	defer close(p.readEnded)
	defer close(p.responses)
	reader := bufio.NewReader(p.read)
	for {
		packet := []byte{}
		for {
			fragment, err := reader.ReadSlice(0)
			packet = append(packet, fragment...)
			if len(packet) > 2<<20 {
				return
			}
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil {
				return
			}
			break
		}
		var reply pipeReply
		if json.Unmarshal(packet[:len(packet)-1], &reply) != nil {
			return
		}
		if reply.ID == 0 {
			continue
		}
		select {
		case p.responses <- reply:
		case <-p.stopped:
			return
		}
	}
}
func (p *pipeProcess) packet(method string, params any, session string) (int, []byte, error) {
	p.sequence++
	id := p.sequence
	request := map[string]any{"id": id, "method": method, "params": params}
	if session != "" {
		request["sessionId"] = session
	}
	bytes, err := json.Marshal(request)
	if err != nil {
		return 0, nil, err
	}
	return id, append(bytes, 0), nil
}
func (p *pipeProcess) send(method string, params any, session string) (int, error) {
	id, packet, err := p.packet(method, params, session)
	if err != nil {
		return 0, err
	}
	_, err = p.write.Write(packet)
	return id, err
}
func (p *pipeProcess) beginCommand(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.writeLost.Load() {
		return controlWriteLost()
	}
	select {
	case p.commandGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.stopped:
		return errors.New("private pipe closed")
	}
}
func (p *pipeProcess) sendContext(ctx context.Context, method string, params any, session string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if p.writeLost.Load() {
		return 0, controlWriteLost()
	}
	id, packet, err := p.packet(method, params, session)
	if err != nil {
		return 0, err
	}
	completed := make(chan error, 1)
	go func() { _, err := p.write.Write(packet); completed <- err }()
	select {
	case err := <-completed:
		if err != nil {
			p.writeLost.Store(true)
			return 0, controlWriteLost()
		}
		return id, nil
	case <-ctx.Done():
		// A bounded command must not hang forever in a synchronous Windows pipe
		// write. A lost control channel is not re-used or published as ready.
		_ = windows.CancelIoEx(windows.Handle(p.write.Fd()), nil)
		p.writeLost.Store(true)
		_ = p.write.Close()
		return 0, errors.Join(ctx.Err(), controlWriteLost())
	}
}
func controlWriteLost() error {
	return &Problem{Code: "CONTROL_CHANNEL_LOST", Reason: "control-write-unavailable", Message: "本次私有控制通道已断开，不能重试正常关闭；仍保留会话和数据锁，没有结束其他进程。", Retryable: false}
}
func (p *pipeProcess) call(ctx context.Context, method string, params any, session string, output any) error {
	if err := p.beginCommand(ctx); err != nil {
		return err
	}
	defer func() { <-p.commandGate }()
	return p.callLocked(ctx, method, params, session, output)
}

// Caller owns commandGate for the entire typed read/write/readback sequence.
func (p *pipeProcess) callLocked(ctx context.Context, method string, params any, session string, output any) error {
	id, err := p.sendContext(ctx, method, params, session)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case reply, open := <-p.responses:
			if !open {
				return &Problem{Code: "CONTROL_CHANNEL_LOST", Reason: "control-read-ended", Message: "本次私有控制通道已断开，无法确认命令完成；仍保护原浏览数据。", Retryable: false}
			}
			if reply.ID != id {
				continue
			}
			if len(reply.Error) > 0 {
				return errors.New("diagnostic command rejected")
			}
			if output == nil {
				return nil
			}
			return json.Unmarshal(reply.Result, output)
		}
	}
}
func (p *pipeProcess) normalClose() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.beginCommand(ctx); err != nil {
		return false
	}
	defer func() { <-p.commandGate }()
	if _, err := p.sendContext(ctx, "Browser.close", map[string]any{}, ""); err != nil {
		return false
	}
	state, err := windows.WaitForSingleObject(p.process, 5000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		return false
	}
	var code uint32
	return windows.GetExitCodeProcess(p.process, &code) == nil && code == 0
}
