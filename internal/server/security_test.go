package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redlab/redlab/internal/auth"
	"github.com/redlab/redlab/internal/scenario"
)

func configureTestCredentials(t testing.TB, app *Server, teams map[string]string) {
	t.Helper()
	organizer, err := auth.NewRecord("organizer-secret")
	if err != nil {
		t.Fatal(err)
	}
	link, err := auth.NewRecord("link-secret")
	if err != nil {
		t.Fatal(err)
	}
	records := make(map[string]auth.Record, len(teams))
	for teamID, secret := range teams {
		record, err := auth.NewRecord(secret)
		if err != nil {
			t.Fatal(err)
		}
		records[teamID] = record
	}
	app.credentialMu.Lock()
	app.Credentials = auth.File{Organizer: organizer, Link: link, Teams: records}
	app.credentialErr = nil
	app.credentialMu.Unlock()
}

func TestCredentialStoreFailsClosed(t *testing.T) {
	root := t.TempDir()
	app := New(filepath.Join(root, "event.yaml"), scenario.Event{Metadata: scenario.DocumentMeta{ID: "event"}}, map[string]scenario.Package{"test": {}}, nil)
	for _, endpoint := range []string{"/api/v1/auth/team/login", "/api/v1/auth/organizer/login"} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(`{"teamID":"TEAM-1","joinCode":"guess","password":"guess"}`)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s failed open with status %d: %s", endpoint, response.Code, response.Body.String())
		}
	}
	ready := httptest.NewRecorder()
	app.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/api/v1/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready endpoint ignored missing credentials: %d", ready.Code)
	}
}

func TestSecurityHeadersAndPublicEventRedaction(t *testing.T) {
	event := scenario.Event{APIVersion: "redlab/v1", Kind: "Event", Metadata: scenario.DocumentMeta{ID: "event", Title: "Event"}, Spec: scenario.EventSpec{Teams: scenario.TeamsSpec{Source: `C:\\private\\teams.csv`}, Server: scenario.ServerSpec{Listen: "0.0.0.0:8443", Database: `C:\\private\\event.db`, TLS: scenario.TLSSpec{Mode: "provided", Certificate: `C:\\private\\server.crt`, Key: `C:\\private\\server.key`}}}}
	app := New("event.yaml", event, map[string]scenario.Package{"test": {}}, nil)
	configureTestCredentials(t, app, map[string]string{"TEAM-1": "team-secret"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/event", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("event response: %d %s", response.Code, response.Body.String())
	}
	for _, secret := range []string{"teams.csv", "event.db", "server.crt", "server.key", "0.0.0.0"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("public event response exposed %q: %s", secret, response.Body.String())
		}
	}
	for _, header := range []string{"Content-Security-Policy", "Cross-Origin-Opener-Policy", "Referrer-Policy", "X-Content-Type-Options", "X-Frame-Options"} {
		if response.Header().Get(header) == "" {
			t.Fatalf("missing security header %s", header)
		}
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("sensitive response is cacheable: %q", response.Header().Get("Cache-Control"))
	}
}

func TestLegacyCredentialUpgradesAfterSuccessfulLogin(t *testing.T) {
	root := t.TempDir()
	salt := "00112233445566778899aabbccddeeff"
	legacy := func(secret string) auth.Record {
		sum := sha256.Sum256([]byte(salt + "\x00" + secret))
		return auth.Record{Salt: salt, Hash: hex.EncodeToString(sum[:])}
	}
	credentials := auth.File{Organizer: legacy("organizer-secret"), Link: legacy("link-secret"), Teams: map[string]auth.Record{"TEAM-1": legacy("team-secret")}}
	credentialsPath := filepath.Join(root, "data", "credentials.json")
	if err := auth.Save(credentialsPath, credentials); err != nil {
		t.Fatal(err)
	}
	app := New(filepath.Join(root, "event.yaml"), scenario.Event{Metadata: scenario.DocumentMeta{ID: "event"}}, map[string]scenario.Package{"test": {}}, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/team/login", bytes.NewBufferString(`{"teamID":"TEAM-1","joinCode":"team-secret"}`))
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy login failed: %d %s", response.Code, response.Body.String())
	}
	updated, err := auth.Load(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Teams["TEAM-1"].Algorithm != "bcrypt" || auth.NeedsUpgrade(updated.Teams["TEAM-1"]) {
		t.Fatalf("legacy credential was not upgraded: %#v", updated.Teams["TEAM-1"])
	}
}
