package bundle

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/redlab/redlab/internal/evidence"
	"github.com/redlab/redlab/internal/report"
	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/scoring"
)

func TestWriteAndVerify(t *testing.T) {
	_, private, err := evidence.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Report: report.Model{EventID: "event", ScenarioID: "scenario", TeamID: "team", SessionID: "session", ScenarioDigest: "digest", Score: scoring.Result{}}, Scenario: scenario.Scenario{Spec: scenario.ScenarioSpec{}}, Manifest: evidence.Manifest{EventID: "event", ScenarioID: "scenario", TeamID: "team", SessionID: "session", ScenarioDigest: "digest"}, PrivateKey: private}
	filename := filepath.Join(t.TempDir(), "submission.rlab.zip")
	if err := Write(filename, input); err != nil {
		t.Fatal(err)
	}
	manifest, err := Verify(filename)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SessionID != "session" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestVerifyRejectsDuplicateEntries(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "duplicate.rlab.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for i := 0; i < 2; i++ {
		entry, err := writer.Create("report.md")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("duplicate")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filename); err == nil {
		t.Fatal("duplicate bundle entry was accepted")
	}
}
