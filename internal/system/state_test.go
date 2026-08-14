package system

import (
	"os"
	"path/filepath"
	"strings"
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

func TestNewStateIncludesRHELLikeBaseFilesystem(t *testing.T) {
	state, err := NewState(testPackage(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/boot", "/dev", "/etc", "/home", "/proc", "/run", "/sys", "/usr/bin", "/var/log", "/home/trainee"} {
		entry, statErr := state.Stat(name)
		if statErr != nil || !entry.Directory {
			t.Errorf("expected base directory %s: entry=%+v err=%v", name, entry, statErr)
		}
	}
	for _, name := range []string{"/sbin/init", "/usr/bin/ls", "/usr/bin/systemctl", "/usr/sbin/useradd"} {
		entry, statErr := state.Stat(name)
		if statErr != nil || entry.Directory || entry.Mode&0111 == 0 {
			t.Errorf("expected virtual executable %s: entry=%+v err=%v", name, entry, statErr)
		}
	}
	osRelease, err := state.ReadFile("/etc/os-release", "trainee")
	if err != nil || !strings.Contains(osRelease, "VERSION_ID=\"8.10\"") {
		t.Fatalf("unexpected os-release: %q, %v", osRelease, err)
	}
	passwd, err := state.ReadFile("/etc/passwd", "trainee")
	if err != nil || !strings.Contains(passwd, "trainee:x:1000:1000:trainee:/home/trainee:/bin/bash") {
		t.Fatalf("unexpected passwd: %q, %v", passwd, err)
	}
	if state.CWD != "/home/trainee" || state.Env["HOME"] != "/home/trainee" || state.Env["PWD"] != state.CWD {
		t.Fatalf("login environment is not realistic: cwd=%q env=%v", state.CWD, state.Env)
	}
}

func TestScenarioPackageCanOverrideBaseFile(t *testing.T) {
	pkg := testPackage()
	pkg.Files = map[string][]byte{"files/etc/hosts": []byte("192.0.2.10 custom.example.test\n")}
	state, err := NewState(pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	content, err := state.ReadFile("/etc/hosts", "root")
	if err != nil || content != "192.0.2.10 custom.example.test\n" {
		t.Fatalf("scenario base-file override = %q, %v", content, err)
	}
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
