package runtime

import (
	"bytes"
	"testing"
	"time"

	"github.com/redlab/redlab/internal/scenario"
)

func TestReplayReproducesState(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{Metadata: scenario.DocumentMeta{ID: "replay", Title: "Replay"}, Spec: scenario.ScenarioSpec{
		RHEL:    scenario.RHELSpec{Major: 8, Hostname: "replay.example", SELinux: "enforcing"},
		Actors:  scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}, Shell: "/bin/bash"}}},
		Scoring: scenario.ScoringSpec{AutomatedMaximum: 0},
	}}}
	seed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	original, err := NewSession("original", "TEAM", pkg, seed)
	if err != nil {
		t.Fatal(err)
	}
	original.RunLine("id")
	original.RunLine("echo hello | sudo tee /tmp/hello")
	replayed, err := Replay("original", "TEAM", pkg, original.Chain.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	left, _ := original.State.SnapshotJSON()
	right, _ := replayed.State.SnapshotJSON()
	if !bytes.Equal(left, right) {
		t.Fatalf("replayed state differs\noriginal=%s\nreplayed=%s", left, right)
	}
}

func TestVirtualGlobExpansion(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{Metadata: scenario.DocumentMeta{ID: "glob", Title: "Glob"}, Spec: scenario.ScenarioSpec{
		RHEL:    scenario.RHELSpec{Major: 8, Hostname: "glob.example", SELinux: "enforcing"},
		Actors:  scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}, Shell: "/bin/bash"}}},
		Scoring: scenario.ScoringSpec{AutomatedMaximum: 0},
	}}}
	session, err := NewSession("glob", "TEAM", pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result := session.RunLine("printf 'x\\n' | sudo tee /tmp/one.conf"); result.ExitCode != 0 {
		t.Fatalf("create file: %+v", result)
	}
	if result := session.RunLine("ls /tmp/*.conf"); result.ExitCode != 0 || !bytes.Contains([]byte(result.Stdout), []byte("one.conf")) {
		t.Fatalf("glob result: %+v", result)
	}
}
