package scenario

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadScenarioYAML validates a scenario document while preserving the supplied
// package files. It is used by the organizer editor and keeps scenario editing
// subject to the same strict loader as files loaded from disk.
func LoadScenarioYAML(data []byte, filename string, files map[string][]byte) (Package, []Diagnostic) {
	pkg, diagnostics := decodeScenario(data, filename)
	if len(diagnostics) > 0 {
		return Package{}, diagnostics
	}
	copyFiles := cloneFiles(files)
	copyFiles["scenario.yaml"] = append([]byte(nil), data...)
	pkg.Files = copyFiles
	pkg.Digest = digestFiles(copyFiles)
	return pkg, nil
}

// Clone returns an independent package copy suitable for an editor or import
// transaction. Scenario packages are treated as immutable by running sessions.
func (p Package) Clone() Package {
	clone := p
	clone.Files = cloneFiles(p.Files)
	return clone
}

// ScenarioYAML returns the package's source document.
func (p Package) ScenarioYAML() []byte {
	return append([]byte(nil), p.Files["scenario.yaml"]...)
}

// RecomputeDigest returns the content digest after an editor changes files.
func (p Package) RecomputeDigest() string { return digestFiles(p.Files) }

// WriteArchive writes a deterministic .rlab package archive.
func (p Package) WriteArchive(w io.Writer) error {
	if len(p.Files) == 0 {
		return fmt.Errorf("scenario package has no files")
	}
	writer := zip.NewWriter(w)
	keys := make([]string, 0, len(p.Files))
	for key := range p.Files {
		safe, err := safePackagePath(key)
		if err != nil {
			_ = writer.Close()
			return err
		}
		keys = append(keys, safe)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry, err := writer.Create(key)
		if err != nil {
			_ = writer.Close()
			return err
		}
		if _, err := entry.Write(p.Files[key]); err != nil {
			_ = writer.Close()
			return err
		}
	}
	return writer.Close()
}

// WriteArchiveFile writes a package to a path without exposing the package's
// source directory or allowing callers to construct unsafe archive entries.
func (p Package) WriteArchiveFile(filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	if err := p.WriteArchive(file); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// ExtractPackage safely materializes a validated .rlab archive for local
// authoring. It refuses traversal, symlinks, and files outside destination.
func ExtractPackage(filename, destination string) error {
	pkg, diagnostics := LoadScenario(filename)
	if len(diagnostics) > 0 {
		return diagnostics[0]
	}
	root, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	for key, data := range pkg.Files {
		safe, err := safePackagePath(key)
		if err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(safe))
		absolute, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		prefix := root + string(filepath.Separator)
		if absolute != root && !strings.HasPrefix(absolute, prefix) {
			return fmt.Errorf("package file escapes destination: %s", key)
		}
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(absolute, data, 0600); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePackagePath exposes the archive path boundary to package managers.
func ValidatePackagePath(name string) (string, error) { return safePackagePath(name) }

func cloneFiles(files map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(files)+1)
	for key, data := range files {
		clone[key] = append([]byte(nil), data...)
	}
	return clone
}
