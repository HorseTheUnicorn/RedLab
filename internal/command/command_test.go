package command

import (
	"strings"
	"testing"
	"time"

	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/system"
)

func TestLevelABCommandsUseOnlyVirtualState(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{Spec: scenario.ScenarioSpec{
		RHEL:       scenario.RHELSpec{Hostname: "app.example.test", SELinux: "enforcing"},
		Actors:     scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}, Shell: "/bin/bash"}}},
		Filesystem: scenario.FilesystemSpec{Entries: []scenario.FileSpec{{Path: "/etc/app.conf", Owner: "root", Group: "root", Mode: "0644", Append: "b\na\na\n"}}},
		Packages:   scenario.PackagesSpec{Installed: []scenario.PackageSpec{{Name: "app", Version: "1.0"}}},
		Network:    scenario.NetworkSpec{DNS: scenario.DNSSpec{Records: map[string]string{"app.example.test": "10.0.0.2"}}},
	}}, Files: map[string][]byte{}}
	state, err := system.NewState(pkg, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	RegisterCore(registry)
	env := &Env{State: state, User: "root", CWD: "/", Variables: map[string]string{}}

	if result := registry.Run("cp", env, []string{"/etc/app.conf", "/tmp/app.conf"}, ""); result.ExitCode != 0 {
		t.Fatalf("cp failed: %+v", result)
	}
	if result := registry.Run("mv", env, []string{"/tmp/app.conf", "/tmp/moved.conf"}, ""); result.ExitCode != 0 {
		t.Fatalf("mv failed: %+v", result)
	}
	if result := registry.Run("find", env, []string{"/tmp", "-type", "f"}, ""); !strings.Contains(result.Stdout, "/tmp/moved.conf") {
		t.Fatalf("find output = %q", result.Stdout)
	}
	if result := registry.Run("rpm", env, []string{"-q", "app"}, ""); !strings.Contains(result.Stdout, "app-1.0") {
		t.Fatalf("rpm output = %q", result.Stdout)
	}
	if result := registry.Run("host", env, []string{"app.example.test"}, ""); !strings.Contains(result.Stdout, "10.0.0.2") {
		t.Fatalf("host output = %q", result.Stdout)
	}
	if result := registry.Run("date", env, []string{"+%s"}, ""); result.Stdout != "1767323045\n" {
		t.Fatalf("date output = %q", result.Stdout)
	}
	if result := registry.Run("usermod", env, []string{"-aG", "ops", "trainee"}, ""); result.ExitCode != 0 || !state.UserInGroup("trainee", "ops") {
		t.Fatalf("usermod result = %+v", result)
	}
}

func TestShellHelpAndCompatibilityInfoAreDeterministic(t *testing.T) {
	registry := NewRegistry()
	RegisterCore(registry)
	state := &system.State{CurrentUser: "root", CWD: "/", Env: map[string]string{}}
	env := &Env{State: state, User: "root", CWD: "/", Variables: state.Env}

	if result := registry.Run("bash", env, []string{"--version"}, ""); result.ExitCode != 0 || !strings.Contains(result.Stdout, "bounded shell") {
		t.Fatalf("bash info = %+v", result)
	}
	help := registry.Run("help", env, nil, "")
	if help.ExitCode != 0 || !strings.Contains(help.Stdout, "systemctl") || !strings.Contains(help.Stdout, "Recognized") {
		t.Fatalf("help = %+v", help)
	}
	man := registry.Run("man", env, []string{"curl"}, "")
	if man.ExitCode != 0 || !strings.Contains(man.Stdout, "networking") {
		t.Fatalf("man = %+v", man)
	}
	if result := registry.Run("alias", env, nil, ""); result.ExitCode != 0 || !strings.Contains(result.Stdout, "no aliases") {
		t.Fatalf("alias = %+v", result)
	}
}
