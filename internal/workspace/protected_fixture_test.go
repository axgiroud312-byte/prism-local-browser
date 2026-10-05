//go:build windows

package workspace

import (
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

// A controlled external-to-browser upstream, restricted to one local synthetic
// site and its HTTPS IP observer. It never forwards arbitrary public requests.
func protectedSiteProxy(t *testing.T, target string) (string, proxy.CheckOptions) {
	t.Helper()
	observer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ip":"203.0.113.29"}`) }))
	t.Cleanup(observer.Close)
	pool := x509.NewCertPool()
	pool.AddCert(observer.Certificate())
	observed, _ := url.Parse(observer.URL)
	allowed, _ := url.Parse(target)
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			if r.Host != observed.Host {
				http.Error(w, "uncontrolled target", 403)
				return
			}
			remote, err := net.DialTimeout("tcp4", observed.Host, 5*time.Second)
			if err != nil {
				http.Error(w, "observer unavailable", 502)
				return
			}
			client, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				remote.Close()
				return
			}
			fmt.Fprint(buffer, "HTTP/1.1 200 Connection Established\r\n\r\n")
			buffer.Flush()
			go func() { defer remote.Close(); defer client.Close(); io.Copy(remote, buffer) }()
			io.Copy(client, remote)
			client.Close()
			remote.Close()
			return
		}
		if r.URL.Scheme != "http" || r.URL.Host != allowed.Host {
			http.Error(w, "uncontrolled target", 403)
			return
		}
		request := r.Clone(r.Context())
		request.RequestURI = ""
		response, err := transport.RoundTrip(request)
		if err != nil {
			http.Error(w, "target unavailable", 502)
			return
		}
		defer response.Body.Close()
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		io.Copy(w, response.Body)
	}))
	t.Cleanup(upstream.Close)
	return upstream.URL, proxy.CheckOptions{TargetURL: observer.URL, RootCAs: pool}
}
