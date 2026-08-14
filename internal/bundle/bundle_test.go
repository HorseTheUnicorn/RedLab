package bundle

import (
	"archive/zip"
	"io"
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

func TestVerifyRejectsNonCanonicalEntry(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "noncanonical.rlab.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	if _, err := writer.Create("nested/../manifest.json"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filename); err == nil {
		t.Fatal("non-canonical bundle entry was accepted")
	}
}

func TestVerifyRejectsUnsignedExtraEntry(t *testing.T) {
	_, private, err := evidence.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Report: report.Model{EventID: "event", ScenarioID: "scenario", TeamID: "team", SessionID: "session", ScenarioDigest: "digest", Score: scoring.Result{}}, Scenario: scenario.Scenario{Spec: scenario.ScenarioSpec{}}, Manifest: evidence.Manifest{EventID: "event", ScenarioID: "scenario", TeamID: "team", SessionID: "session", ScenarioDigest: "digest"}, PrivateKey: private}
	root := t.TempDir()
	original := filepath.Join(root, "original.rlab.zip")
	if err := Write(original, input); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(root, "tampered.rlab.zip")
	copyZipWithExtra(t, original, tampered, "unsigned.txt", []byte("not covered by manifest"))
	if _, err := Verify(tampered); err == nil {
		t.Fatal("unsigned extra bundle entry was accepted")
	}
}

func copyZipWithExtra(t testing.TB, source, destination, name string, data []byte) {
	t.Helper()
	reader, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	file, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range reader.File {
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		output, err := writer.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(output, input); err != nil {
			t.Fatal(err)
		}
		if err := input.Close(); err != nil {
			t.Fatal(err)
		}
	}
	extra, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extra.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
