package scenario

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalScenario = `apiVersion: redlab/v1
kind: Scenario
metadata: {id: test, title: Test, version: 1.0.0}
spec:
  rhel: {major: 8, minorProfile: "8.10", hostname: test.example, architecture: x86_64, selinux: enforcing, commandPacks: [coreutils]}
  briefing: {difficulty: beginner, duration: 10m, summary: Test, objectivesShownToParticipants: [Test]}
  actors:
    initialUser: trainee
    users: [{name: trainee, uid: 1000, groups: [wheel], shell: /bin/bash}]
    sudo: []
  filesystem:
    templates: []
    entries: [{path: /etc/test.conf, owner: root, group: root, mode: "0644", append: "ok\n"}]
  packages: {installed: []}
  services: []
  network: {interfaces: [], dns: {servers: [], records: {}}, firewall: {defaultZone: public, zones: {}}, simulatedHosts: []}
  faults: []
  rules: []
  objectives: []
  guardrails: []
  hints: []
  scoring: {automatedMaximum: 0, judgeMaximum: 0, completionBonus: 0, minimumPassingScore: 0}
  judgeRubrics: []
`

func TestLoadScenarioRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(filename, []byte(strings.Replace(minimalScenario, "kind: Scenario", "kind: Scenario\nunknown: true", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	_, diagnostics := LoadScenario(dir)
	if len(diagnostics) == 0 {
		t.Fatal("unknown field was accepted")
	}
}

func TestArchiveRejectsTraversal(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "bad.rlab")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("x"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, diagnostics := LoadScenario(filename)
	if len(diagnostics) == 0 {
		t.Fatal("traversal archive was accepted")
	}
}

func TestScenarioDocumentSizeLimit(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "scenario.yaml")
	data := minimalScenario + "\n#" + strings.Repeat("x", (2<<20)+1)
	if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	_, diagnostics := LoadScenario(filepath.Dir(filename))
	if len(diagnostics) == 0 {
		t.Fatal("oversized scenario document was accepted")
	}
}
