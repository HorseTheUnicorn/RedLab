package bundle

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/redlab/redlab/internal/evidence"
	"github.com/redlab/redlab/internal/report"
	"github.com/redlab/redlab/internal/scenario"
)

type Input struct {
	Report     report.Model
	Scenario   scenario.Scenario
	Events     []evidence.Event
	Manifest   evidence.Manifest
	PrivateKey ed25519.PrivateKey
}

func Write(filename string, input Input) error {
	if len(input.PrivateKey) != ed25519.PrivateKeySize {
		return errors.New("an Ed25519 private key is required")
	}
	reportJSON, err := report.JSON(input.Report)
	if err != nil {
		return err
	}
	reportMD := report.Markdown(input.Report, input.Scenario.Spec)
	timeline := timelineJSONL(input.Events)
	files := map[string][]byte{"report.md": []byte(reportMD), "report.json": reportJSON, "transcript.txt": []byte(input.Report.Transcript), "timeline.jsonl": []byte(timeline), "state-diff.patch": []byte(report.Patch(input.Report.StateDiff)), "scenario-manifest.json": mustJSON(map[string]any{"id": input.Scenario.Metadata.ID, "version": input.Scenario.Metadata.Version, "digest": input.Report.ScenarioDigest})}
	input.Manifest.Files = map[string]string{}
	for name, data := range files {
		sum := sha256.Sum256(data)
		input.Manifest.Files[name] = hex.EncodeToString(sum[:])
	}
	if len(input.Events) > 0 {
		input.Manifest.ChainHead = input.Events[len(input.Events)-1].Hash
	}
	signature, err := evidence.SignManifest(input.Manifest, input.PrivateKey)
	if err != nil {
		return err
	}
	files["manifest.json"] = mustJSON(input.Manifest)
	files["manifest.sig"] = mustJSON(signature)
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write(files[name]); err != nil {
			return err
		}
	}
	return writer.Close()
}

func Verify(filename string) (evidence.Manifest, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return evidence.Manifest{}, err
	}
	defer reader.Close()
	files := map[string][]byte{}
	seen := map[string]bool{}
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if err := safeName(entry.Name); err != nil {
			return evidence.Manifest{}, err
		}
		if seen[entry.Name] {
			return evidence.Manifest{}, fmt.Errorf("duplicate bundle entry %q", entry.Name)
		}
		seen[entry.Name] = true
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return evidence.Manifest{}, fmt.Errorf("symlink bundle entry %q is not allowed", entry.Name)
		}
		if entry.UncompressedSize64 > 64<<20 {
			return evidence.Manifest{}, fmt.Errorf("bundle entry %q exceeds the 64 MiB limit", entry.Name)
		}
		stream, err := entry.Open()
		if err != nil {
			return evidence.Manifest{}, err
		}
		data, err := io.ReadAll(io.LimitReader(stream, 64<<20))
		stream.Close()
		if err != nil {
			return evidence.Manifest{}, err
		}
		total += int64(len(data))
		if total > 256<<20 {
			return evidence.Manifest{}, errors.New("bundle exceeds total size limit")
		}
		files[entry.Name] = data
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return evidence.Manifest{}, errors.New("invalid manifest.json")
	}
	var signature evidence.Signature
	if err := json.Unmarshal(files["manifest.sig"], &signature); err != nil {
		return evidence.Manifest{}, errors.New("invalid manifest.sig")
	}
	if err := evidence.VerifyManifest(manifest, signature); err != nil {
		return evidence.Manifest{}, err
	}
	for name, expected := range manifest.Files {
		data, ok := files[name]
		if !ok {
			return evidence.Manifest{}, fmt.Errorf("bundle is missing %s", name)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expected {
			return evidence.Manifest{}, fmt.Errorf("bundle hash mismatch for %s", name)
		}
	}
	return manifest, nil
}

func ReadReport(filename string) (report.Model, error) {
	if _, err := Verify(filename); err != nil {
		return report.Model{}, err
	}
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return report.Model{}, err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Name != "report.json" {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			return report.Model{}, err
		}
		data, err := io.ReadAll(io.LimitReader(stream, 64<<20))
		stream.Close()
		if err != nil {
			return report.Model{}, err
		}
		var model report.Model
		if err := json.Unmarshal(data, &model); err != nil {
			return report.Model{}, err
		}
		return model, nil
	}
	return report.Model{}, errors.New("bundle is missing report.json")
}

func timelineJSONL(events []evidence.Event) string {
	var b strings.Builder
	for _, event := range events {
		data, _ := json.Marshal(event)
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}
func mustJSON(value any) []byte { data, _ := json.MarshalIndent(value, "", "  "); return data }
func safeName(name string) error {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return fmt.Errorf("unsafe bundle entry %q", name)
	}
	return nil
}
