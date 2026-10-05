//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Separate, unprivileged synthetic bridge owner for a genuine process-kill
// experiment. It can only route to the parent's loopback synthetic origin.
func bridgeOwner(sidText, originText string) error {
	sid, err := windows.StringToSid(sidText)
	if err != nil || !strings.HasPrefix(sidText, "S-1-15-2-") {
		return fmt.Errorf("invalid-package")
	}
	origin, err := url.Parse(originText)
	if err != nil || origin.Scheme != "http" || origin.Hostname() != "127.0.0.1" || origin.User != nil {
		return fmt.Errorf("invalid-synthetic-origin")
	}
	listener, err := packageListener(sid)
	if err != nil {
		return err
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext}
	defer transport.CloseIdleConnections()
	reverse := httputil.NewSingleHostReverseProxy(origin)
	reverse.Transport = transport
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme != "http" || r.URL.Host != "prism-feasibility.example" {
			http.Error(w, "synthetic origin only", 403)
			return
		}
		reverse.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 3 * time.Second}
	done := make(chan struct{})
	go func() { _ = server.Serve(listener); close(done) }()
	if err = json.NewEncoder(os.Stdout).Encode(map[string]string{"address": listener.Addr().String()}); err != nil {
		server.Close()
		<-done
		return err
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	server.Close()
	<-done
	return nil
}

func startBridgeOwner(helperPath string, sid *windows.SID, origin *url.URL) (p *labProcess, address string, resultErr error) {
	p, err := launch(helperPath, []string{"--bridge-owner", sid.String(), origin.String()}, nil, false, nil)
	if err != nil {
		return p, "", err
	}
	type answer struct {
		Address string `json:"address"`
		err     error
	}
	ready := make(chan answer, 1)
	go func() { var a answer; a.err = json.NewDecoder(io.LimitReader(p.output, 4096)).Decode(&a); ready <- a }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case a := <-ready:
		if a.err != nil {
			return p, "", a.err
		}
		host, _, e := net.SplitHostPort(a.Address)
		if e != nil || host != "127.0.0.1" {
			return p, "", fmt.Errorf("invalid-bridge-address")
		}
		return p, a.Address, nil
	case <-ctx.Done():
		p.output.Close()
		<-ready
		return p, "", ctx.Err()
	}
}
