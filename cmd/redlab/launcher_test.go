package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	redlab "github.com/redlab/redlab"
	"github.com/redlab/redlab/internal/auth"
	"github.com/redlab/redlab/internal/scenario"
)

func TestLauncherShowsHelpAndExits(t *testing.T) {
	var output bytes.Buffer
	if err := runLauncher(strings.NewReader("4\n5\n"), &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Start a participant scenario", "Advanced command-line examples", "Goodbye."} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("launcher output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestLauncherStartsEmbeddedScenario(t *testing.T) {
	var output bytes.Buffer
	if err := runLauncher(strings.NewReader("1\n1\nlab briefing\nexit\n"), &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Available scenarios:", "RedLab practice session", "redlab$"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("participant output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestInitializeEventPackagesEveryBuiltinScenario(t *testing.T) {
	root := filepath.Join(t.TempDir(), "event")
	var output bytes.Buffer
	if err := initializeEvent(root, &output); err != nil {
		t.Fatal(err)
	}
	eventFile := filepath.Join(root, "event.yaml")
	event, diagnostics := scenario.LoadEvent(eventFile)
	if len(diagnostics) > 0 {
		t.Fatalf("event diagnostics: %v", diagnostics)
	}
	ids, err := redlab.BuiltinScenarioIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(event.Spec.Scenarios) != len(ids) {
		t.Fatalf("event scenarios = %d, want %d", len(event.Spec.Scenarios), len(ids))
	}
	for _, reference := range event.Spec.Scenarios {
		archive := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(reference.Package, "./")))
		if info, err := os.Stat(archive); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("scenario archive %s is unavailable: %v", archive, err)
		}
		if _, packageDiagnostics := scenario.LoadScenario(archive); len(packageDiagnostics) > 0 {
			t.Fatalf("scenario archive %s is invalid: %v", archive, packageDiagnostics)
		}
	}
	credentials, err := auth.Load(filepath.Join(root, "data", "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.EnsureValid(credentials); err != nil {
		t.Fatal(err)
	}
	if err := initializeEvent(root, &bytes.Buffer{}); err == nil {
		t.Fatal("second initialization unexpectedly overwrote the event")
	}
	for _, expected := range []string{"Organizer recovery secret:", "Event link token:", "TEAM-1 join code:"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("initializer did not print %q", expected)
		}
	}
}

func TestOpenDashboardWaitsForHealthBeforeLaunchingBrowser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	original := launchBrowser
	defer func() { launchBrowser = original }()
	opened := ""
	launchBrowser = func(address string) error {
		opened = address
		return nil
	}
	address := server.URL + "/"
	if err := openDashboardWhenReady(address); err != nil {
		t.Fatal(err)
	}
	if opened != address {
		t.Fatalf("opened %q, want %q", opened, address)
	}
}
