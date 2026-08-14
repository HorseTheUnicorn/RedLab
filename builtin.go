package redlab

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Built-in scenario content is embedded into every RedLab binary. The helper
// exposes bytes only; loading and validating YAML remains in internal/scenario.
//
//go:embed scenario-packs/core/**
var builtinScenarioFS embed.FS

// BuiltinScenarioIDs returns the stable, sorted IDs embedded in this build.
func BuiltinScenarioIDs() ([]string, error) {
	entries, err := fs.ReadDir(builtinScenarioFS, "scenario-packs/core")
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil, fmt.Errorf("no built-in scenarios are embedded in this build")
	}
	return ids, nil
}

// BuiltinScenarioFiles returns a defensive copy of a first-party scenario
// package. IDs may be written as broken-httpd, core/broken-httpd, or
// builtin:broken-httpd.
func BuiltinScenarioFiles(id string) (map[string][]byte, error) {
	id = strings.TrimPrefix(id, "builtin:")
	id = strings.TrimPrefix(id, "core/")
	id = path.Clean(id)
	if id == "." || id == ".." || strings.HasPrefix(id, "../") || strings.Contains(id, "/../") || strings.Contains(id, ":") {
		return nil, fmt.Errorf("invalid built-in scenario id %q", id)
	}
	root := path.Join("scenario-packs", "core", id)
	if _, err := fs.Stat(builtinScenarioFS, root); err != nil {
		return nil, fmt.Errorf("built-in scenario %q not found", id)
	}
	files := map[string][]byte{}
	err := fs.WalkDir(builtinScenarioFS, root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(name, root+"/")
		if relative == "" || relative == name {
			return fmt.Errorf("built-in scenario path escaped package root")
		}
		data, err := fs.ReadFile(builtinScenarioFS, name)
		if err != nil {
			return err
		}
		files[relative] = append([]byte(nil), data...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("built-in scenario %q is empty", id)
	}
	return files, nil
}
