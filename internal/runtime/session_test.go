package runtime

import (
	"bytes"
	"strings"
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

func TestScenarioPasswordsAreRedactedFromEvidenceAndTranscript(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{Metadata: scenario.DocumentMeta{ID: "redaction", Title: "Redaction"}, Spec: scenario.ScenarioSpec{
		RHEL:   scenario.RHELSpec{Major: 8, Hostname: "redaction.example", SELinux: "enforcing"},
		Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Password: "super-secret-password", Shell: "/bin/bash"}}},
	}}}
	session, err := NewSession("redaction", "TEAM", pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	result := session.RunLine("printf super-secret-password")
	if !strings.Contains(result.Stdout, "super-secret-password") {
		t.Fatalf("interactive command result unexpectedly changed: %#v", result)
	}
	if strings.Contains(session.TranscriptText(), "super-secret-password") {
		t.Fatal("scenario password leaked into transcript")
	}
	events := session.Report("event").Timeline
	if len(events) != 1 || strings.Contains(events[0].Command, "super-secret-password") {
		t.Fatalf("scenario password leaked into evidence: %#v", events)
	}
	replayed, err := Replay("redaction", "TEAM", pkg, session.PersistenceEvents())
	if err != nil {
		t.Fatal(err)
	}
	if got := replayed.State.CurrentTime(); !got.Equal(session.State.CurrentTime()) {
		t.Fatalf("secret-bearing command did not replay: got %s want %s", got, session.State.CurrentTime())
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
