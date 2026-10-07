//go:build windows

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type bfeProbeRequest struct {
	Marker    string   `json:"marker"`
	Addresses []string `json:"addresses"`
}
type bfeProbeSample struct {
	Mode         string              `json:"mode"`
	StartedAt    string              `json:"startedAt"`
	FinishedAt   string              `json:"finishedAt"`
	AppContainer uint32              `json:"appContainer"`
	Capabilities uint32              `json:"capabilities"`
	Integrity    uint32              `json:"integrity"`
	TokenError   bool                `json:"tokenError"`
	Sockets      []socketObservation `json:"sockets"`
	DNSStatus    uint32              `json:"dnsStatus"`
	DNSQueries   int                 `json:"dnsQueries"`
	DNSSender    string              `json:"dnsSender"`
	Error        string              `json:"error,omitempty"`
}
type bfeProbePhase struct {
	Phase   string           `json:"phase"`
	Before  bfeServiceState  `json:"before"`
	After   bfeServiceState  `json:"after"`
	Samples []bfeProbeSample `json:"samples"`
}
type bfeProbeReady struct {
	Format            string `json:"format"`
	Nonce             string `json:"nonce"`
	PID               uint32 `json:"pid"`
	Created           uint64 `json:"created"`
	ExecutableSHA256  string `json:"executableSha256"`
	BaselineConfirmed bool   `json:"baselineConfirmed"`
	At                string `json:"at"`
}

func bfeProbeChild(nonce string) error {
	if _, err := uuid.Parse(nonce); err != nil {
		return err
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 16384)
	for scanner.Scan() {
		var request bfeProbeRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return err
		}
		if !strings.HasPrefix(request.Marker, nonce+".") || !strings.HasSuffix(request.Marker, ".invalid") || len(request.Addresses) != 4 {
			return errors.New("invalid synthetic request")
		}
		for _, address := range request.Addresses {
			host, _, err := net.SplitHostPort(address)
			if err != nil || !net.ParseIP(host).IsLoopback() {
				return errors.New("only loopback observers permitted")
			}
		}
		r := bfeProbeSample{StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Sockets: make([]socketObservation, 4)}
		var e1, e2, e3 error
		r.AppContainer, e1 = tokenValue(windows.GetCurrentProcessToken(), 29)
		r.Capabilities, e2 = tokenValue(windows.GetCurrentProcessToken(), 30)
		r.Integrity, e3 = tokenIntegrity(windows.GetCurrentProcessToken())
		r.TokenError = e1 != nil || e2 != nil || e3 != nil
		var work sync.WaitGroup
		for index, protocol := range []string{"tcp4", "tcp6", "udp4", "udp6"} {
			work.Add(1)
			go func() {
				defer work.Done()
				r.Sockets[index] = socketObservation{Protocol: protocol, Echo: socketEcho(protocol, request.Addresses[index], request.Marker)}
			}()
		}
		work.Add(1)
		go func() { defer work.Done(); r.DNSStatus = queryDNS(request.Marker, "127.0.0.1:53") }()
		work.Wait()
		r.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func bfeTCP4Observer() (*socketObserver, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	o := &socketObserver{protocol: "tcp4", address: listener.Addr().String(), close: listener.Close, packets: map[string]int{}, done: make(chan struct{})}
	go func() {
		defer close(o.done)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			packet := make([]byte, 256)
			n, err := c.Read(packet)
			if err == nil {
				o.record(string(packet[:n]))
				_, _ = c.Write(packet[:n])
			}
			c.Close()
		}
	}()
	return o, nil
}

func bfeChildSample(p *labProcess, request bfeProbeRequest) (bfeProbeSample, error) {
	result := make(chan struct {
		value bfeProbeSample
		err   error
	}, 1)
	go func() {
		var sample bfeProbeSample
		err := json.NewEncoder(p.input).Encode(request)
		if err == nil {
			var data []byte
			data, err = p.reader.ReadBytes('\n')
			if err == nil {
				err = json.Unmarshal(data, &sample)
			}
		}
		result <- struct {
			value bfeProbeSample
			err   error
		}{sample, err}
	}()
	select {
	case result := <-result:
		return result.value, result.err
	case <-time.After(12 * time.Second):
		p.input.Close()
		p.output.Close()
		return bfeProbeSample{}, errors.New("synthetic child probe deadline")
	}
}

func prepareBFEProbe(path string) (resultErr error) {
	dir, unpin, err := bfeEvidenceDirectory(path, true)
	if err != nil {
		return err
	}
	defer unpin()
	cleanup := "not-created"
	defer func() {
		message := ""
		if resultErr != nil {
			message = resultErr.Error()
		}
		resultErr = errors.Join(resultErr, bfeWrite(dir, "probe-finished.json", map[string]any{"cleanup": cleanup, "error": message, "at": time.Now().UTC().Format(time.RFC3339Nano)}))
	}()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	selfSHA, err := fileHash(self)
	if err != nil {
		return err
	}
	nonce := strings.TrimPrefix(filepath.Base(dir), "bfe-stop-")
	root := filepath.Join(dir, "synthetic")
	if err = os.Mkdir(root, 0700); err != nil {
		return err
	}
	cleanup = "pending"
	allExited := true
	defer func() {
		if !allExited {
			cleanup = "retained-unconfirmed-job"
			return
		}
		for attempt := 0; attempt < 20; attempt++ {
			err := os.RemoveAll(root)
			if err == nil {
				if cleanup != "container-profile-retained" {
					cleanup = "complete"
				}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		cleanup = "synthetic-directory-retained"
	}()
	container, err := createContainer("prism-feasibility-" + nonce)
	if err != nil {
		return err
	}
	defer func() {
		if !allExited {
			return
		}
		if err := container.close(); err != nil {
			cleanup = "container-profile-retained"
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err = bfeWrite(dir, "probe-container.json", map[string]string{"name": "prism-feasibility-" + nonce, "sid": container.sid.String()}); err != nil {
		return err
	}
	image := filepath.Join(root, "bfe-synthetic-probe.exe")
	if err = copyFile(self, image); err != nil {
		return err
	}
	if err = allowSyntheticRoot(root, container.sid); err != nil {
		return err
	}
	tcp4, err := bfeTCP4Observer()
	if err != nil {
		return err
	}
	defer func() { tcp4.close(); <-tcp4.done }()
	other, err := startSocketObservers()
	if err != nil {
		return err
	}
	defer func() {
		for _, o := range other {
			o.close()
			<-o.done
		}
	}()
	observers := append([]*socketObserver{tcp4}, other...)
	dns, err := newDNSObserver()
	if err != nil {
		return err
	}
	defer dns.close()
	children := []*labProcess{}
	defer func() {
		for _, child := range children {
			if err := child.stop(); err != nil {
				allExited = false
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()
	for _, sid := range []*windows.SID{nil, container.sid} {
		p, err := launch(image, []string{"--bfe-probe-child", nonce}, sid, false, nil)
		if p != nil {
			children = append(children, p)
		}
		if err != nil {
			return err
		}
	}
	identity := ownedProcessSnapshot(children[1], container.sid)
	if len(identity) != 1 || identity[0].Error != "" || identity[0].AppContainer != 1 || !identity[0].SamePackage || identity[0].CapabilityCount != 0 {
		return errors.New("synthetic AppContainer identity unconfirmed")
	}
	identity[0].Role = "synthetic-network-helper"
	if err = bfeWrite(dir, "probe-identity.json", identity); err != nil {
		return err
	}
	phase := func(name string) (bfeProbePhase, error) {
		r := bfeProbePhase{Phase: name, Before: readBFEState(), Samples: make([]bfeProbeSample, 2)}
		var work sync.WaitGroup
		for index, child := range children {
			work.Add(1)
			go func() {
				defer work.Done()
				mode := []string{"ordinary", "appcontainer-zero-capability"}[index]
				request := bfeProbeRequest{Marker: nonce + "." + name + "." + mode + ".invalid"}
				for _, observer := range observers {
					request.Addresses = append(request.Addresses, observer.address)
				}
				sample, err := bfeChildSample(child, request)
				sample.Mode = mode
				if err != nil {
					sample.Error = err.Error()
				}
				for i := range sample.Sockets {
					sample.Sockets[i].Received = observers[i].count(request.Marker)
				}
				sample.DNSQueries, sample.DNSSender = dns.count(request.Marker), dns.owner(request.Marker)
				r.Samples[index] = sample
			}()
		}
		work.Wait()
		r.After = readBFEState()
		return r, bfeWrite(dir, "probe-"+name+".json", r)
	}
	baseline, err := phase("baseline")
	if err != nil {
		return err
	}
	if baseline.Before.State != windows.SERVICE_RUNNING || baseline.After.State != windows.SERVICE_RUNNING {
		return errors.New("BFE baseline not RUNNING")
	}
	for index, sample := range baseline.Samples {
		if sample.Error != "" || sample.TokenError || sample.AppContainer != uint32(index) || len(sample.Sockets) != 4 {
			return errors.New("baseline identity or helper failure")
		}
		for _, socket := range sample.Sockets {
			if index == 0 && (!socket.Echo || socket.Received == 0) || index == 1 && (socket.Echo || socket.Received != 0) {
				return errors.New("baseline socket control not established")
			}
		}
		if index == 0 && (sample.DNSStatus != 0 || sample.DNSQueries == 0 || sample.DNSSender != "dns-client-service") || index == 1 && (sample.DNSStatus != 5 || sample.DNSQueries != 0 || sample.Capabilities != 0) {
			return errors.New("baseline delegated DNS control not established")
		}
	}
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return err
	}
	ready := bfeProbeReady{Format: bfeExperimentFormat, Nonce: nonce, PID: uint32(os.Getpid()), Created: uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), ExecutableSHA256: selfSHA, BaselineConfirmed: true, At: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = bfeWrite(dir, "probe-ready.json", ready); err != nil {
		return err
	}
	fmt.Println("BFE synthetic probes ready; no service control has been requested.")
	deadline := time.Now().Add(5 * time.Minute)
	probed := false
	for time.Now().Before(deadline) {
		var cancellation struct {
			Nonce string `json:"nonce"`
		}
		if bfeRead(dir, "probe-cancel-request.json", &cancellation) == nil && cancellation.Nonce == nonce {
			if _, err := os.Stat(filepath.Join(dir, "controller-claim.json")); os.IsNotExist(err) {
				return nil // Only cancel preparation, never an in-flight service experiment.
			}
		}
		var trigger struct {
			Nonce string `json:"nonce"`
		}
		if !probed && bfeRead(dir, "probe-stopped-request.json", &trigger) == nil && trigger.Nonce == nonce {
			probed = true
			if _, err = phase("stopped"); err != nil {
				return err
			}
		}
		var done struct {
			Nonce    string `json:"nonce"`
			Restored bool   `json:"restored"`
		}
		if bfeRead(dir, "controller-finished.json", &done) == nil && done.Nonce == nonce {
			if !done.Restored {
				return errors.New("controller did not confirm BFE RUNNING; inspect recovery evidence")
			}
			_, err = phase("after")
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return context.DeadlineExceeded
}
