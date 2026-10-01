package kernel

import (
	"strings"
	"testing"
)

func TestManagedProxyArgumentsNeverIncludeUpstreamSecretsOrDirectFallback(t *testing.T) {
	args, err := managedNetworkArguments("http://127.0.0.1:18443")
	if err != nil || !strings.Contains(strings.Join(args, " "), "--proxy-bypass-list=<-loopback>") {
		t.Fatal("private listener or implicit bypass handling missing")
	}
	for _, value := range args {
		if strings.Contains(value, "--no-proxy-server") || strings.Contains(value, "DIRECT") || strings.Contains(value, "--no-sandbox") || strings.Contains(value, "--ignore-certificate-errors") {
			t.Fatal("proxy process has a silent direct or security bypass")
		}
	}
	for _, endpoint := range []string{"https://127.0.0.1:443", "http://synthetic-user:SYNTHETIC_PASSWORD@127.0.0.1:8080", "http://localhost:8080", "http://127.0.0.1:0", "http://127.0.0.1:8080/path", "http://127.0.0.1:8080?token=secret", "http://127.0.0.1:8080#token", "http://127.0.0.1:08080"} {
		if _, err := managedNetworkArguments(endpoint); err == nil {
			t.Fatal("untrusted/noncanonical endpoint entered browser arguments")
		}
	}
}
