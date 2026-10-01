package proxy

import (
	"strings"
	"testing"
)

func TestProxyParsingKeepsURIIPv6AndEncodedCredentialsDistinct(t *testing.T) {
	for _, test := range []struct {
		input, kind, host, username, password string
		port                                  int
	}{
		{"http://synthetic-user:p%3Aa%40ss@EXAMPLE.invalid:8080", "http", "example.invalid", "synthetic-user", "p:a@ss", 8080},
		{"https://[2001:db8::1]:8443", "https", "2001:db8::1", "", "", 8443},
		{"127.0.0.1:8080:synthetic-user:synthetic-pass", "http", "127.0.0.1", "synthetic-user", "synthetic-pass", 8080},
		{"socks5://[::1]:1080", "socks5", "::1", "", "", 1080},
	} {
		config, credentials, err := ParseLine(test.input)
		if err != nil || config.Type != test.kind || config.Host != test.host || config.Port != test.port {
			t.Fatalf("safe configuration parsing mismatch for %s", test.kind)
		}
		if test.username == "" && credentials != nil {
			t.Fatal("parser invented authentication")
		}
		if test.username != "" && (credentials == nil || credentials.Username != test.username || credentials.Password != test.password) {
			t.Fatal("encoded/legacy credentials were changed")
		}
	}
}
func TestProxyParsingReportsBadLinesWithoutEchoingSecrets(t *testing.T) {
	marker := "SYNTHETIC_PRIVATE_PASSWORD"
	for _, input := range []string{"http://user:" + marker + "@example.invalid:0", "http://user:" + marker + "@example.invalid:8080/path", "2001:db8::1:1080", "http://example.invalid", "http://user%3Aname:" + marker + "@example.invalid:8080", "https://example.invalid:8443?secret=" + marker, "http://user:%0a@localhost:80"} {
		_, _, err := ParseLine(input)
		if err == nil || strings.Contains(err.Error(), marker) {
			t.Fatal("bad proxy was accepted or its credentials entered a parser diagnostic")
		}
	}
	rows, ignored, err := ParseImport("\ufeff# comment\r\n\nhttp://localhost:8080\nbad:" + marker + "\n")
	if err != nil || ignored != 3 || len(rows) != 2 || rows[0].Line != 3 || rows[1].Line != 4 || rows[1].Error == "" {
		t.Fatal("original line numbers/invalid rows were silently lost")
	}
}
