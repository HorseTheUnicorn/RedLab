package system

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/redlab/redlab/internal/scenario"
)

func testPackage() scenario.Package {
	return scenario.Package{Scenario: scenario.Scenario{Spec: scenario.ScenarioSpec{
		RHEL: scenario.RHELSpec{Major: 8, Hostname: "test.example", SELinux: "enforcing"},
		Actors: scenario.ActorsSpec{
			InitialUser: "trainee",
			Users:       []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}, Shell: "/bin/bash"}},
		},
		Filesystem: scenario.FilesystemSpec{Entries: []scenario.FileSpec{{Path: "/etc/test.conf", Owner: "root", Group: "root", Mode: "0600", Append: "initial\n"}}},
	}}}
}

func TestVirtualStateResetAndHostIsolation(t *testing.T) {
	state, err := NewState(testPackage(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	hostFilename := filepath.Join(t.TempDir(), "must-not-exist")
	if err := state.WriteFile(hostFilename, "host escape", "root", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hostFilename); !os.IsNotExist(err) {
		t.Fatalf("virtual write touched host path: %v", err)
	}
	if err := state.WriteFile("/tmp/changed", "changed", "root", false); err != nil {
		t.Fatal(err)
	}
	if err := state.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Stat("/tmp/changed"); err == nil {
		t.Fatal("reset retained a virtual mutation")
	}
}

func TestPermissionsAreVirtual(t *testing.T) {
	state, err := NewState(testPackage(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReadFile("/etc/test.conf", "trainee"); err == nil {
		t.Fatal("trainee unexpectedly read root-only file")
	}
	if _, err := state.ReadFile("/etc/test.conf", "root"); err != nil {
		t.Fatal(err)
	}
}
