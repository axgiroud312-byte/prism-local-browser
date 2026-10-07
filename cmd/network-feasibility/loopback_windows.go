//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"runtime"
	"time"

	"golang.org/x/sys/windows"
)

type loopbackObservation struct {
	Mode       string `json:"mode"`
	Connected  bool   `json:"connected"`
	Accepted   bool   `json:"accepted"`
	Error      string `json:"error,omitempty"`
	JobsExited bool   `json:"jobsExited"`
}

// Unlike a port whitelist, same-package loopback is checked against the
// identities of both sockets. This is an experiment of Go's actual Listen /
// Accept path with impersonation at socket creation, not a policy assertion.
func samePackageLoopback(helperPath string, sid *windows.SID) (result []loopbackObservation, allExited bool) {
	allExited = true
	holder, err := launch(helperPath, []string{"--token-holder"}, sid, false, nil)
	if err != nil {
		return []loopbackObservation{{Mode: "token-holder", Error: err.Error()}}, true
	}
	defer func() {
		if holder.stop() != nil {
			allExited = false
		}
	}()
	var primary, impersonation windows.Token
	if err = windows.OpenProcessToken(holder.process, windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY, &primary); err == nil {
		err = windows.DuplicateTokenEx(primary, windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE|windows.TOKEN_DUPLICATE, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &impersonation)
		primary.Close()
	}
	if err != nil {
		return []loopbackObservation{{Mode: "duplicate-container-token", Error: err.Error()}}, true
	}
	defer impersonation.Close()
	type listenerResult struct {
		listener net.Listener
		err      error
	}
	ready := make(chan listenerResult, 1)
	accepted := make(chan bool, 1)
	finished := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(finished)
		if e := windows.SetThreadToken(nil, impersonation); e != nil {
			ready <- listenerResult{err: e}
			return
		}
		defer windows.RevertToSelf()
		listener, e := net.Listen("tcp4", "127.0.0.1:0")
		ready <- listenerResult{listener, e}
		if e != nil {
			return
		}
		defer listener.Close()
		_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(8 * time.Second))
		c, e := listener.Accept()
		accepted <- e == nil
		if e == nil {
			_ = c.SetDeadline(time.Now().Add(3 * time.Second))
			packet := make([]byte, 5)
			if _, e = io.ReadFull(c, packet); e == nil {
				_, _ = c.Write(packet)
			}
			c.Close()
		}
	}()
	l := <-ready
	if l.err != nil {
		<-finished
		return []loopbackObservation{{Mode: "impersonated-listener", Error: l.err.Error()}}, true
	}
	address := l.listener.Addr().String()
	probe := func(mode string) loopbackObservation {
		r := loopbackObservation{Mode: mode, JobsExited: true}
		p, e := launch(helperPath, []string{"--tcp-only", address}, sid, false, nil)
		if e != nil {
			r.Error = e.Error()
			return r
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		raw, e := p.helperReport(ctx)
		cancel()
		if e != nil {
			r.Error = e.Error()
		} else {
			var h helperObservation
			if json.Unmarshal(raw, &h) != nil {
				r.Error = "invalid-helper-response"
			} else {
				r.Connected = h.TCPConnected
				r.Error = h.TCPError
			}
		}
		if p.stop() != nil {
			r.JobsExited = false
			allExited = false
		}
		return r
	}
	one := probe("same-package-listener")
	l.listener.Close()
	<-finished
	select {
	case one.Accepted = <-accepted:
	default:
	}
	result = append(result, one)
	// Rebind the exact released port as the ordinary host. The old client still
	// uses its original package and must not transmit to this replacement.
	replacement, err := net.Listen("tcp4", address)
	if err != nil {
		return append(result, loopbackObservation{Mode: "ordinary-port-replacement", Error: fmt.Sprint(err)}), allExited
	}
	defer replacement.Close()
	replacedAccepted := make(chan bool, 1)
	go func() {
		_ = replacement.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
		c, e := replacement.Accept()
		replacedAccepted <- e == nil
		if e == nil {
			c.Close()
		}
	}()
	two := probe("ordinary-port-replacement")
	replacement.Close()
	two.Accepted = <-replacedAccepted
	result = append(result, two)
	return
}
