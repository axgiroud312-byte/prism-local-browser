//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type browserBridgeObservation struct {
	Stage               string               `json:"stage"`
	Loaded              bool                 `json:"loaded"`
	Storage             json.RawMessage      `json:"storage,omitempty"`
	NormalExit          bool                 `json:"normalExit"`
	Processes           []processObservation `json:"processes,omitempty"`
	BridgeRequests      *int64               `json:"bridgeRequests,omitempty"`
	BridgeProcessKilled bool                 `json:"bridgeProcessKilled,omitempty"`
	OriginRequests      int64                `json:"originRequests"`
	ReplacementAccepts  int64                `json:"replacementAccepts"`
	ReplacementRequests int64                `json:"replacementRequests"`
	AfterBridgeClose    *bool                `json:"afterBridgeClose,omitempty"`
	ControlStillAlive   bool                 `json:"controlStillAlive,omitempty"`
	Error               string               `json:"error,omitempty"`
	NewPackageOnReopen  bool                 `json:"newPackageOnReopen,omitempty"`
	DataGrantCleanup    string               `json:"dataGrantCleanup,omitempty"`
}

type countingListener struct {
	net.Listener
	accepts atomic.Int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e == nil {
		l.accepts.Add(1)
	}
	return c, e
}

// The socket is created with the package identity. Accept, HTTP processing and
// upstream dialing deliberately run after impersonation has ended.
func packageListener(sid *windows.SID) (net.Listener, error) {
	primary, err := nativeLowBox(sid)
	if err != nil {
		return nil, err
	}
	defer primary.Close()
	var token windows.Token
	if err = windows.DuplicateTokenEx(primary, windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &token); err != nil {
		return nil, err
	}
	defer token.Close()
	type answer struct {
		listener net.Listener
		err      error
	}
	done := make(chan answer, 1)
	go func() {
		runtime.LockOSThread()
		if e := windows.SetThreadToken(nil, token); e != nil {
			runtime.UnlockOSThread()
			done <- answer{err: e}
			return
		}
		l, e := net.Listen("tcp4", "127.0.0.1:0")
		if revoke := windows.RevertToSelf(); revoke != nil {
			if l != nil {
				l.Close()
			}
			done <- answer{err: revoke}
			return // retire locked thread
		}
		runtime.UnlockOSThread()
		done <- answer{l, e}
	}()
	r := <-done
	return r.listener, r.err
}

func (p *labProcess) gracefulExit(ctx context.Context) bool {
	if p.call(ctx, "Browser.close", map[string]any{}, "", nil) != nil {
		return false
	}
	code, err := p.wait(ctx)
	if err != nil || code != 0 {
		return false
	}
	for ctx.Err() == nil {
		var accounting struct {
			User, Kernel, PeriodUser, PeriodKernel int64
			Faults, Total, Active, Terminated      uint32
		}
		if windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil) != nil {
			return false
		}
		if accounting.Active == 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func (p *labProcess) page(ctx context.Context, address string) (string, error) {
	var target struct {
		ID string `json:"targetId"`
	}
	var session struct {
		ID string `json:"sessionId"`
	}
	if err := p.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank", "background": true}, "", &target); err != nil {
		return "", err
	}
	if err := p.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.ID, "flatten": true}, "", &session); err != nil {
		return "", err
	}
	var navigation struct {
		Error string `json:"errorText"`
	}
	if err := p.call(ctx, "Page.navigate", map[string]any{"url": address}, session.ID, &navigation); err != nil {
		return "", err
	}
	if navigation.Error != "" {
		return "", fmt.Errorf("page-navigation: %s", navigation.Error)
	}
	for ctx.Err() == nil {
		raw, err := p.evaluate(ctx, session.ID, `location.hostname === 'prism-feasibility.example' && document.body?.textContent === 'PRISM_SYNTHETIC_ORIGIN'`)
		if err != nil {
			return "", err
		}
		if string(raw) == "true" {
			return session.ID, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return "", ctx.Err()
}
func (p *labProcess) evaluate(ctx context.Context, session, expression string) (json.RawMessage, error) {
	var result struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if err := p.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, session, &result); err != nil {
		return nil, err
	}
	if len(result.Exception) > 0 {
		return nil, fmt.Errorf("synthetic-page-script-failed")
	}
	return result.Result.Value, nil
}

func storageExpression(write string) string {
	return `(async()=>{const marker=` + strconv.Quote(write) + `;if(marker){document.cookie='prism_lab='+marker+';Path=/;Max-Age=7200;SameSite=Lax';localStorage.setItem('prism_lab',marker)}const db=await new Promise((resolve,reject)=>{const r=indexedDB.open('prism_lab',1);r.onupgradeneeded=()=>r.result.createObjectStore('state');r.onerror=()=>reject(Error('open'));r.onsuccess=()=>resolve(r.result)});const value=await new Promise((resolve,reject)=>{const tx=db.transaction('state',marker?'readwrite':'readonly');const s=tx.objectStore('state');let value=null;if(marker)s.put(marker,'marker');const get=s.get('marker');get.onsuccess=()=>value=get.result??null;tx.oncomplete=()=>resolve(value);tx.onerror=()=>reject(Error('transaction'))});db.close();return{cookie:document.cookie,local:localStorage.getItem('prism_lab'),indexed:value}})()`
}
func storageMatches(raw json.RawMessage, marker string) bool {
	var values struct{ Cookie, Local, Indexed string }
	return json.Unmarshal(raw, &values) == nil && values.Cookie == "prism_lab="+marker && values.Local == marker && values.Indexed == marker
}

func browserBridgeTrial(exe, helperPath, root string, baseArgs []string, sid *windows.SID) (result []browserBridgeObservation, allExited bool) {
	allExited = true
	profile := filepath.Join(root, "bridge-persistence-profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		return []browserBridgeObservation{{Stage: "prepare", Error: err.Error()}}, true
	}
	next, err := createContainer("prism-feasibility-" + uuid.NewString())
	if err != nil {
		return []browserBridgeObservation{{Stage: "second-package", Error: err.Error()}}, true
	}
	defer func() {
		if allExited {
			if e := next.close(); e != nil {
				result = append(result, browserBridgeObservation{Stage: "second-package-cleanup", Error: e.Error()})
			}
		}
	}()
	revokeStation, err := grantExistingSyntheticWindowStation(next.sid, filepath.Join("output", "goal", "T11", "station-acl-private-"+uuid.NewString()+".txt"))
	if err != nil {
		return []browserBridgeObservation{{Stage: "second-station-grant", Error: err.Error()}}, true
	}
	defer func() {
		if e := revokeStation(); e != nil {
			result = append(result, browserBridgeObservation{Stage: "second-station-cleanup", Error: e.Error()})
		}
	}()
	originListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return []browserBridgeObservation{{Stage: "origin-listen", Error: err.Error()}}, true
	}
	var originRequests atomic.Int64
	origin := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originRequests.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("<!doctype html><body>PRISM_SYNTHETIC_ORIGIN</body>"))
	}), ReadHeaderTimeout: 3 * time.Second}
	go origin.Serve(originListener)
	defer origin.Close()
	originURL, _ := url.Parse("http://" + originListener.Addr().String())
	for _, stage := range []string{"ordinary-write", "container-read-write", "container-reopen-and-bridge-loss", "ordinary-read-back"} {
		c := browserBridgeObservation{Stage: stage}
		isolated := strings.HasPrefix(stage, "container")
		stageSID := sid
		if stage == "container-reopen-and-bridge-loss" {
			stageSID = next.sid
			c.NewPackageOnReopen = true
		}
		var revokeData func() error
		if isolated {
			revokeData, err = grantSyntheticDataTree(profile, stageSID)
			if err != nil {
				c.Error = "existing-data-grant: " + err.Error()
				if revokeData != nil {
					_ = revokeData()
				}
				result = append(result, c)
				return
			}
		}
		var listener net.Listener
		if isolated {
			listener, err = packageListener(stageSID)
		} else {
			listener, err = net.Listen("tcp4", "127.0.0.1:0")
		}
		if err != nil {
			c.Error = "bridge-listen: " + err.Error()
			result = append(result, c)
			return
		}
		address := listener.Addr().String()
		var bridgeRequests atomic.Int64
		transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext}
		reverse := httputil.NewSingleHostReverseProxy(originURL)
		reverse.Transport = transport
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Host != "prism-feasibility.example" || r.URL.Scheme != "http" {
				http.Error(w, "synthetic origin only", 403)
				return
			}
			bridgeRequests.Add(1)
			reverse.ServeHTTP(w, r)
		}), ReadHeaderTimeout: 3 * time.Second}
		served := make(chan struct{})
		go func() { _ = server.Serve(listener); close(served) }()
		closeBridge := func() { server.Close(); <-served; transport.CloseIdleConnections() }
		if stage == "container-reopen-and-bridge-loss" {
			closeBridge()
			var owner *labProcess
			owner, address, err = startBridgeOwner(helperPath, stageSID, originURL)
			if err != nil {
				c.Error = "separate-bridge-owner: " + err.Error()
				if owner != nil && owner.stop() != nil {
					allExited = false
				}
				result = append(result, c)
				return
			}
			closed := false
			closeBridge = func() {
				if closed {
					return
				}
				closed = true
				if e := windows.TerminateProcess(owner.process, 57); e == nil {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					code, e := owner.wait(ctx)
					cancel()
					c.BridgeProcessKilled = e == nil && code == 57
				}
				if owner.stop() != nil {
					allExited = false
				}
			}
		}
		args := []string{}
		for _, arg := range baseArgs {
			if !strings.HasPrefix(arg, "--user-data-dir=") && !strings.HasPrefix(arg, "--proxy-server=") {
				args = append(args, arg)
			}
		}
		args = append(args, "--user-data-dir="+profile, "--proxy-server=http://"+address)
		var packageSID *windows.SID
		if isolated {
			packageSID = stageSID
		}
		p, e := launch(exe, args, packageSID, true, nil)
		if e != nil {
			closeBridge()
			c.Error = "browser-start: " + e.Error()
			result = append(result, c)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		session, e := p.page(ctx, "http://prism-feasibility.example/")
		if e != nil {
			c.Error = e.Error()
		} else {
			c.Loaded = true
			c.Processes = ownedProcessSnapshot(p, stageSID)
			write := ""
			if stage == "ordinary-write" {
				write = "before-container"
			}
			c.Storage, e = p.evaluate(ctx, session, storageExpression(write))
			if e != nil {
				c.Error = e.Error()
			} else {
				expected := "inside-container"
				if stage == "ordinary-write" || stage == "container-read-write" {
					expected = "before-container"
				}
				if !storageMatches(c.Storage, expected) {
					c.Error = "persistent-synthetic-state-mismatch"
				}
				if c.Error == "" && stage == "container-read-write" {
					raw, e := p.evaluate(ctx, session, storageExpression("inside-container"))
					if e != nil || !storageMatches(raw, "inside-container") {
						c.Error = "container-state-write-failed"
					}
				}
			}
			if c.Error == "" && stage == "container-reopen-and-bridge-loss" {
				closeBridge()
				replacement, e := net.Listen("tcp4", address)
				if e != nil {
					c.Error = "port-replacement: " + e.Error()
				} else {
					counted := &countingListener{Listener: replacement}
					var hits atomic.Int64
					fake := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write([]byte("unexpected")) })}
					done := make(chan struct{})
					go func() { _ = fake.Serve(counted); close(done) }()
					raw, e := p.evaluate(ctx, session, `fetch('http://prism-feasibility.example/after-bridge-loss',{cache:'no-store',signal:AbortSignal.timeout(3000)}).then(()=>true).catch(()=>false)`)
					if e != nil {
						c.Error = "bridge-loss-probe: " + e.Error()
					} else {
						var value bool
						if json.Unmarshal(raw, &value) != nil {
							c.Error = "bridge-loss-invalid-value"
						} else {
							c.AfterBridgeClose = &value
						}
					}
					c.ControlStillAlive = p.call(ctx, "Browser.getVersion", map[string]any{}, "", nil) == nil
					fake.Close()
					<-done
					c.ReplacementAccepts = counted.accepts.Load()
					c.ReplacementRequests = hits.Load()
				}
			}
		}
		cancel()
		exitCtx, exitCancel := context.WithTimeout(context.Background(), 10*time.Second)
		c.NormalExit = p.gracefulExit(exitCtx)
		exitCancel()
		if p.stop() != nil {
			allExited = false
			c.Error = errors.Join(fmt.Errorf("job-exit-unconfirmed"), e).Error()
		}
		closeBridge()
		if revokeData != nil && allExited {
			if e := revokeData(); e != nil {
				c.Error = "data-grant-revoke: " + e.Error()
				c.DataGrantCleanup = "failed"
			} else {
				c.DataGrantCleanup = "revoked"
			}
		}
		if stage != "container-reopen-and-bridge-loss" {
			count := bridgeRequests.Load()
			c.BridgeRequests = &count
		}
		c.OriginRequests = originRequests.Load()
		result = append(result, c)
		if !allExited || c.Error != "" || !c.NormalExit {
			return
		}
	}
	return
}
