//go:build windows

package kernel

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
	"golang.org/x/sys/windows"
)

// Instance-local operations keep lifecycle failure tests separate from actual
// process creation. Production always constructs these from its private pipe;
// they are never launch options or an RPC/network-boundary override.
type managedProcessControl struct {
	snapshot     func() RuntimeSnapshot
	exitReady    func() (bool, error)
	requestClose func(context.Context) error
	terminate    func() error
	closeWrite   func()
	releasePipe  func()
}

type managedCleanupAttempt struct {
	done     chan struct{}
	err      error
	finished bool // protected by ManagedProcess.mu
}

func pipeManagedControl(p *pipeProcess) managedProcessControl {
	return managedProcessControl{
		snapshot: p.snapshot,
		exitReady: func() (bool, error) {
			state, err := windows.WaitForSingleObject(p.process, 0)
			if err != nil || state != windows.WAIT_OBJECT_0 {
				return false, err
			}
			count, err := p.activeProcesses()
			return err == nil && count == 0, err
		},
		requestClose: func(ctx context.Context) error {
			if err := p.beginCommand(ctx); err != nil {
				return err
			}
			defer func() { <-p.commandGate }()
			_, err := p.sendContext(ctx, "Browser.close", map[string]any{}, "")
			return err
		},
		terminate:   func() error { return windows.TerminateJobObject(p.job, 1) },
		closeWrite:  p.closeWrite,
		releasePipe: p.close,
	}
}

func newManagedProcess(p *pipeProcess, network ManagedNetwork, release func(), control managedProcessControl) *ManagedProcess {
	var releaseOnce sync.Once
	return &ManagedProcess{pipe: p, network: network, release: func() { releaseOnce.Do(release) }, control: control,
		done: make(chan struct{}), exited: make(chan struct{}), stopGate: make(chan struct{}, 1)}
}

// ManagesNetwork lets the workspace avoid adding a second channel owner around
// a real ManagedProcess. No process returned means ownership never transferred.
func (p *ManagedProcess) ManagesNetwork() bool { return p.network != nil }

func (p *ManagedProcess) confirmJobExit() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.jobExited {
		ready, err := p.control.exitReady()
		if err != nil || !ready {
			return false
		}
		p.lastSnapshot = p.control.snapshot()
		p.lastSnapshot.RootAlive, p.lastSnapshot.ControlReady, p.lastSnapshot.ResourcesExited = false, false, false
		p.jobExited = true
		close(p.exited)
	}
	return true
}

func (p *ManagedProcess) observeExit() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if p.confirmJobExit() {
			p.beginCleanup(false)
			return
		}
		<-ticker.C
	}
}

func (p *ManagedProcess) beginCleanup(retry bool) *managedCleanupAttempt {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.jobExited {
		return nil
	}
	if p.cleanup != nil && (!retry || !p.cleanup.finished || p.cleanup.err == nil) {
		return p.cleanup
	}
	attempt := &managedCleanupAttempt{done: make(chan struct{})}
	p.cleanup = attempt
	go func() {
		// Network close can wait for a probe callback that reads Snapshot.
		// Never hold p.mu while it drains workers, and retain every file/Job
		// handle until it succeeds. An error must not consume releaseOnce.
		var err error
		if p.network != nil {
			err = p.network.Close()
		}
		if err == nil {
			p.control.releasePipe()
			p.release()
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		attempt.finished = true
		p.networkCleanupFailed = err != nil
		if err != nil {
			attempt.err = &proxy.CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "本次进程树已退出，但代理通道清理尚未确认；目录和内核文件保护仍保留，可重试关闭。", Retryable: true}
		} else {
			p.lastSnapshot.ResourcesExited = true
			close(p.done)
		}
		close(attempt.done)
	}()
	return attempt
}

func (p *ManagedProcess) waitCleanup(ctx context.Context, retry bool) error {
	select {
	case <-p.exited:
	case <-ctx.Done():
		return ctx.Err()
	}
	attempt := p.beginCleanup(retry)
	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop never turns a timeout into permission to terminate the tree. Once the
// whole Job is known empty, retry only cleanup, even if CDP has already closed.
func (p *ManagedProcess) Stop(ctx context.Context) error {
	if p.confirmJobExit() {
		return p.waitCleanup(ctx, true)
	}
	select {
	case <-p.done:
		return nil
	case p.stopGate <- struct{}{}:
		defer func() { <-p.stopGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if p.confirmJobExit() {
		return p.waitCleanup(ctx, true)
	}
	if err := p.control.requestClose(ctx); err != nil {
		if p.confirmJobExit() {
			return p.waitCleanup(ctx, false)
		}
		return err
	}
	return p.waitCleanup(ctx, false)
}

func (p *ManagedProcess) Close() error { return p.closeOwned(true) }

// Automatic fault/startup cleanup joins the first attempt; only an explicit
// later Stop/Close retries a completed failure. Force-close never waits for the
// normal-close gate, so loss of the control channel cannot block safety work.
func (p *ManagedProcess) closeOwned(retry bool) (resultErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	defer func() {
		if resultErr != nil && p.network != nil {
			p.mu.Lock()
			if !p.lastSnapshot.ResourcesExited {
				p.networkCleanupFailed = true
			}
			p.mu.Unlock()
		}
	}()
	if !p.confirmJobExit() {
		retry = false
		p.mu.Lock()
		var err error
		if !p.jobExited {
			err = p.control.terminate()
		}
		p.mu.Unlock()
		if err != nil && !p.confirmJobExit() {
			return err
		}
		p.control.closeWrite()
	}
	if err := p.waitCleanup(ctx, retry); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return problem("PROCESS_STOP_TIMEOUT", "owned-resources-unconfirmed", "本次进程树或代理通道资源尚未确认释放，目录与档案保护仍保留。")
		}
		return err
	}
	return nil
}
