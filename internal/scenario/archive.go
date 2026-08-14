package scenario

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	if len(p.Files) > maxPackageFiles {
		return fmt.Errorf("scenario contains more than %d files", maxPackageFiles)
	}
	writer := zip.NewWriter(w)
	keys := make([]string, 0, len(p.Files))
	normalized := make(map[string][]byte, len(p.Files))
	var total int64
	for key, data := range p.Files {
		safe, err := safePackagePath(key)
		if err != nil {
			_ = writer.Close()
			return err
		}
		if _, exists := normalized[safe]; exists {
			_ = writer.Close()
			return fmt.Errorf("duplicate canonical package path %q", safe)
		}
		if int64(len(data)) > maxPackageBytes || total+int64(len(data)) > maxPackageBytes {
			_ = writer.Close()
			return errors.New("scenario package exceeds the total size limit")
		}
		total += int64(len(data))
		normalized[safe] = data
		keys = append(keys, safe)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry, err := writer.Create(key)
		if err != nil {
			_ = writer.Close()
			return err
		}
		if _, err := entry.Write(normalized[key]); err != nil {
			_ = writer.Close()
			return err
		}
	}
	return writer.Close()
}

// WriteArchiveFile writes a package to a path without exposing the package's
// source directory or allowing callers to construct unsafe archive entries.
func (p Package) WriteArchiveFile(filename string) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".scenario-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := file.Name()
	defer os.Remove(temporaryName)
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if err := p.WriteArchive(file); err != nil {
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
	if _, err := os.Lstat(root); err == nil {
		return fmt.Errorf("refusing to extract into existing destination: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(root)
		}
	}()
	scoped, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer scoped.Close()
	keys := make([]string, 0, len(pkg.Files))
	for key := range pkg.Files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		data := pkg.Files[key]
		safe, err := safePackagePath(key)
		if err != nil {
			return err
		}
		relative := filepath.FromSlash(safe)
		if directory := filepath.Dir(relative); directory != "." {
			if err := scoped.MkdirAll(directory, 0700); err != nil {
				return err
			}
		}
		output, err := scoped.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		if _, err := output.Write(data); err != nil {
			_ = output.Close()
			return err
		}
		if err := output.Close(); err != nil {
			return err
		}
	}
	success = true
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
