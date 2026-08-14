package main

import (
	"strings"
	"testing"
)

func TestLocalServeAlwaysForcesLoopback(t *testing.T) {
	for _, input := range []string{"0.0.0.0:8443", "192.168.1.20:9443", "example.com:10443", "[::]:8443"} {
		got, err := loopbackAddress(input)
		if err != nil {
			t.Fatalf("loopbackAddress(%q): %v", input, err)
		}
		if got != "127.0.0.1:"+input[strings.LastIndex(input, ":")+1:] {
			t.Fatalf("loopbackAddress(%q) = %q", input, got)
		}
	}
	if _, err := loopbackAddress("not-an-address"); err == nil {
		t.Fatal("malformed listen address was accepted")
	}
}

func TestJoinURLRejectsPlaintextRemoteCredentials(t *testing.T) {
	for _, value := range []string{"http://192.168.1.20:8443", "http://example.com:8443", "ftp://localhost/file", "https://user:pass@example.com", "https://example.com/path"} {
		if _, err := normalizeServerURL(value); err == nil {
			t.Fatalf("unsafe server URL was accepted: %s", value)
		}
	}
	for _, value := range []string{"http://127.0.0.1:8443", "http://localhost:8443", "http://[::1]:8443", "https://event.example:8443"} {
		if _, err := normalizeServerURL(value); err != nil {
			t.Fatalf("safe server URL %s was rejected: %v", value, err)
		}
	}
}

func TestPinnedClientRejectsMalformedFingerprint(t *testing.T) {
	if _, _, err := joinHTTPClient("https://example.com", "not-a-sha256-fingerprint"); err == nil {
		t.Fatal("malformed fingerprint was accepted")
	}
}
