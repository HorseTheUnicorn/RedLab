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
	"path/filepath"
	"sort"
	"strings"

	"github.com/redlab/redlab/internal/evidence"
	"github.com/redlab/redlab/internal/report"
	"github.com/redlab/redlab/internal/scenario"
)

const (
	maxBundleEntryBytes = 64 << 20
	maxBundleTotalBytes = 256 << 20
	maxBundleFileBytes  = 512 << 20
	maxBundleEntries    = 64
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
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".submission-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := file.Name()
	defer os.Remove(temporaryName)
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	writer := zip.NewWriter(file)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
		if _, err := entry.Write(files[name]); err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return err
	}
	return os.Chmod(filename, 0600)
}

func Verify(filename string) (evidence.Manifest, error) {
	// #nosec G304 -- this CLI boundary intentionally opens the organizer-selected bundle path.
	file, err := os.Open(filename)
	if err != nil {
		return evidence.Manifest{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return evidence.Manifest{}, err
	}
	if !info.Mode().IsRegular() {
		return evidence.Manifest{}, errors.New("bundle must be a regular file")
	}
	return VerifyReader(file, info.Size())
}

// VerifyReader verifies a bundle already opened through a caller-controlled
// filesystem boundary such as os.Root.
func VerifyReader(readerAt io.ReaderAt, size int64) (evidence.Manifest, error) {
	if size < 0 || size > maxBundleFileBytes {
		return evidence.Manifest{}, errors.New("bundle file exceeds the 512 MiB limit")
	}
	reader, err := zip.NewReader(readerAt, size)
	if err != nil {
		return evidence.Manifest{}, err
	}
	files := map[string][]byte{}
	seen := map[string]bool{}
	var total int64
	for index, entry := range reader.File {
		if index >= maxBundleEntries {
			return evidence.Manifest{}, fmt.Errorf("bundle contains more than %d entries", maxBundleEntries)
		}
		if entry.FileInfo().IsDir() {
			return evidence.Manifest{}, fmt.Errorf("directory bundle entry %q is not allowed", entry.Name)
		}
		name, err := safeName(entry.Name)
		if err != nil {
			return evidence.Manifest{}, err
		}
		if seen[name] {
			return evidence.Manifest{}, fmt.Errorf("duplicate bundle entry %q", name)
		}
		seen[name] = true
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return evidence.Manifest{}, fmt.Errorf("symlink bundle entry %q is not allowed", name)
		}
		if entry.UncompressedSize64 > maxBundleEntryBytes {
			return evidence.Manifest{}, fmt.Errorf("bundle entry %q exceeds the 64 MiB limit", name)
		}
		stream, err := entry.Open()
		if err != nil {
			return evidence.Manifest{}, err
		}
		data, err := io.ReadAll(io.LimitReader(stream, maxBundleEntryBytes+1))
		closeErr := stream.Close()
		if err != nil {
			return evidence.Manifest{}, err
		}
		if closeErr != nil {
			return evidence.Manifest{}, closeErr
		}
		if len(data) > maxBundleEntryBytes {
			return evidence.Manifest{}, fmt.Errorf("bundle entry %q exceeds the 64 MiB limit", name)
		}
		total += int64(len(data))
		if total > maxBundleTotalBytes {
			return evidence.Manifest{}, errors.New("bundle exceeds total size limit")
		}
		files[name] = data
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
	allowed := map[string]bool{"manifest.json": true, "manifest.sig": true}
	for name, expected := range manifest.Files {
		canonical, err := safeName(name)
		if err != nil || canonical != name || name == "manifest.json" || name == "manifest.sig" {
			return evidence.Manifest{}, fmt.Errorf("manifest contains invalid file name %q", name)
		}
		if len(expected) != sha256.Size*2 {
			return evidence.Manifest{}, fmt.Errorf("manifest contains invalid hash for %s", name)
		}
		if _, err := hex.DecodeString(expected); err != nil {
			return evidence.Manifest{}, fmt.Errorf("manifest contains invalid hash for %s", name)
		}
		data, ok := files[name]
		if !ok {
			return evidence.Manifest{}, fmt.Errorf("bundle is missing %s", name)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expected {
			return evidence.Manifest{}, fmt.Errorf("bundle hash mismatch for %s", name)
		}
		allowed[name] = true
	}
	for name := range files {
		if !allowed[name] {
			return evidence.Manifest{}, fmt.Errorf("bundle contains unsigned entry %q", name)
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
		data, err := io.ReadAll(io.LimitReader(stream, maxBundleEntryBytes+1))
		closeErr := stream.Close()
		if err != nil {
			return report.Model{}, err
		}
		if closeErr != nil {
			return report.Model{}, closeErr
		}
		if len(data) > maxBundleEntryBytes {
			return report.Model{}, errors.New("report.json exceeds the 64 MiB limit")
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
func safeName(name string) (string, error) {
	if name == "" || len(name) > 4096 || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") || strings.Contains(name, ":") {
		return "", fmt.Errorf("unsafe bundle entry %q", name)
	}
	clean := path.Clean(name)
	if clean != name || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("unsafe bundle entry %q", name)
	}
	return clean, nil
}
