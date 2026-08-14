package scenario

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/redlab/redlab/internal/version"
)

const (
	maxDocumentBytes = 2 << 20
	maxPackageBytes  = 64 << 20
	maxPackageFiles  = 10000
)

type Diagnostic struct {
	Filename string
	Path     string
	Line     int
	Column   int
	Message  string
	Hint     string
}

func (d Diagnostic) Error() string {
	location := d.Filename
	if d.Line > 0 {
		location = fmt.Sprintf("%s:%d:%d", location, d.Line, d.Column)
	}
	if d.Path != "" {
		location += " (" + d.Path + ")"
	}
	if d.Hint != "" {
		return fmt.Sprintf("%s: %s; %s", location, d.Message, d.Hint)
	}
	return location + ": " + d.Message
}

type Package struct {
	Root     string
	Files    map[string][]byte
	Scenario Scenario
	Digest   string
}

func LoadScenario(path string) (Package, []Diagnostic) {
	if path == "" {
		return Package{}, []Diagnostic{{Filename: path, Message: "scenario path is required"}}
	}
	info, err := os.Stat(path)
	if err != nil {
		return Package{}, []Diagnostic{{Filename: path, Message: err.Error()}}
	}
	if info.IsDir() {
		return loadDirectory(path)
	}
	if strings.EqualFold(filepath.Ext(path), ".rlab") {
		return loadArchive(path)
	}
	return Package{}, []Diagnostic{{Filename: path, Message: "scenario must be a directory or .rlab package", Hint: "use `scenario pack` to create a package"}}
}

func LoadEvent(filename string) (Event, []Diagnostic) {
	data, err := readLimited(filename, maxDocumentBytes)
	if err != nil {
		return Event{}, []Diagnostic{{Filename: filename, Message: err.Error()}}
	}
	var event Event
	if err := decodeStrict(data, filename, &event); err != nil {
		return Event{}, diagnosticsFromYAML(err, filename)
	}
	diagnostics := ValidateEvent(event, filename)
	return event, diagnostics
}

func (p Package) ReadFile(name string) ([]byte, bool) {
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	data, ok := p.Files[name]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

func loadDirectory(root string) (Package, []Diagnostic) {
	files := make(map[string][]byte)
	var total int64
	err := filepath.Walk(root, func(filename string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed in scenario packages: %s", filename)
		}
		if info.IsDir() {
			return nil
		}
		if len(files) >= maxPackageFiles {
			return errors.New("scenario contains too many files")
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		key, err := safePackagePath(rel)
		if err != nil {
			return err
		}
		data, err := readLimited(filename, maxPackageBytes)
		if err != nil {
			return err
		}
		total += int64(len(data))
		if total > maxPackageBytes {
			return errors.New("scenario package exceeds the total size limit")
		}
		files[key] = data
		return nil
	})
	if err != nil {
		return Package{}, []Diagnostic{{Filename: root, Message: err.Error()}}
	}
	data, ok := files["scenario.yaml"]
	if !ok {
		return Package{}, []Diagnostic{{Filename: filepath.Join(root, "scenario.yaml"), Message: "scenario.yaml is required"}}
	}
	pkg, diagnostics := decodeScenario(data, filepath.Join(root, "scenario.yaml"))
	pkg.Root, pkg.Files = root, files
	pkg.Digest = digestFiles(files)
	return pkg, diagnostics
}

func loadArchive(filename string) (Package, []Diagnostic) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return Package{}, []Diagnostic{{Filename: filename, Message: err.Error()}}
	}
	defer reader.Close()
	files := make(map[string][]byte)
	var total int64
	for _, entry := range reader.File {
		key, err := safePackagePath(entry.Name)
		if err != nil {
			return Package{}, []Diagnostic{{Filename: filename, Message: err.Error()}}
		}
		if _, exists := files[key]; exists {
			return Package{}, []Diagnostic{{Filename: filename, Message: "archive contains duplicate entry: " + key}}
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return Package{}, []Diagnostic{{Filename: filename, Message: "symlinks are not allowed in scenario archives"}}
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if len(files) >= maxPackageFiles {
			return Package{}, []Diagnostic{{Filename: filename, Message: "scenario contains too many files"}}
		}
		if entry.UncompressedSize64 > maxPackageBytes || total+int64(entry.UncompressedSize64) > maxPackageBytes {
			return Package{}, []Diagnostic{{Filename: filename, Message: "scenario archive exceeds the size limit"}}
		}
		stream, err := entry.Open()
		if err != nil {
			return Package{}, []Diagnostic{{Filename: filename, Message: err.Error()}}
		}
		data, err := io.ReadAll(io.LimitReader(stream, maxPackageBytes+1))
		closeErr := stream.Close()
		if err != nil {
			return Package{}, []Diagnostic{{Filename: filename, Message: err.Error()}}
		}
		if closeErr != nil {
			return Package{}, []Diagnostic{{Filename: filename, Message: closeErr.Error()}}
		}
		if len(data) > maxPackageBytes {
			return Package{}, []Diagnostic{{Filename: filename, Message: "scenario archive entry exceeds the size limit"}}
		}
		total += int64(len(data))
		files[key] = data
	}
	data, ok := files["scenario.yaml"]
	if !ok {
		return Package{}, []Diagnostic{{Filename: filename, Message: "archive must contain scenario.yaml"}}
	}
	pkg, diagnostics := decodeScenario(data, filename+"!scenario.yaml")
	pkg.Root, pkg.Files = filename, files
	pkg.Digest = digestFiles(files)
	return pkg, diagnostics
}

func decodeScenario(data []byte, filename string) (Package, []Diagnostic) {
	if len(data) > maxDocumentBytes {
		return Package{}, []Diagnostic{{Filename: filename, Message: fmt.Sprintf("scenario document exceeds %d byte limit", maxDocumentBytes)}}
	}
	var scenario Scenario
	if err := decodeStrict(data, filename, &scenario); err != nil {
		return Package{}, diagnosticsFromYAML(err, filename)
	}
	return Package{Scenario: scenario}, ValidateScenario(scenario, filename)
}

func decodeStrict(data []byte, filename string, destination any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("document contains more than one YAML document")
		}
		return err
	}
	return nil
}

func diagnosticsFromYAML(err error, filename string) []Diagnostic {
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		out := make([]Diagnostic, 0, len(typeErr.Errors))
		for _, message := range typeErr.Errors {
			out = append(out, Diagnostic{Filename: filename, Message: message, Hint: "check the field name and value type"})
		}
		return out
	}
	return []Diagnostic{{Filename: filename, Message: err.Error()}}
}

func ValidateScenario(s Scenario, filename string) []Diagnostic {
	var diagnostics []Diagnostic
	if s.APIVersion != version.Schema {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "apiVersion", Message: "unsupported apiVersion", Hint: "use redlab/v1"})
	}
	if s.Kind != "Scenario" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "kind", Message: "kind must be Scenario"})
	}
	if s.Metadata.ID == "" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "metadata.id", Message: "scenario id is required"})
	}
	if s.Spec.RHEL.Major != 8 {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.rhel.major", Message: "only RHEL major version 8 is supported"})
	}
	if s.Spec.RHEL.Hostname == "" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.rhel.hostname", Message: "hostname is required"})
	}
	if s.Spec.Actors.InitialUser == "" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.actors.initialUser", Message: "initialUser is required"})
	}
	users := map[string]bool{}
	for _, user := range s.Spec.Actors.Users {
		if user.Name == "" || users[user.Name] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.actors.users", Message: "user names must be non-empty and unique"})
		}
		users[user.Name] = true
	}
	if s.Spec.Actors.InitialUser != "" && !users[s.Spec.Actors.InitialUser] {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.actors.initialUser", Message: "initial user is not declared"})
	}
	services := map[string]bool{}
	for _, service := range s.Spec.Services {
		if service.Name == "" || services[service.Name] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.services", Message: "service names must be non-empty and unique"})
		}
		services[service.Name] = true
	}
	ids := map[string]bool{}
	for _, item := range s.Spec.Objectives {
		if item.ID == "" || ids[item.ID] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.objectives", Message: "objective ids must be non-empty and unique"})
		}
		ids[item.ID] = true
		if err := validateConditionGroup(item.Checks, "spec.objectives."+item.ID+".checks"); err != nil {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.objectives." + item.ID + ".checks", Message: err.Error()})
		}
		if item.Points < 0 {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.objectives." + item.ID + ".points", Message: "points cannot be negative"})
		}
	}
	for index, command := range s.Spec.ReferenceSolution {
		if strings.TrimSpace(command) == "" {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: fmt.Sprintf("spec.referenceSolution[%d]", index), Message: "reference solution commands must be non-empty"})
		}
	}
	for solutionIndex, solution := range s.Spec.ReferenceSolutions {
		if len(solution) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: fmt.Sprintf("spec.referenceSolutions[%d]", solutionIndex), Message: "alternate reference solutions must contain commands"})
		}
		for commandIndex, command := range solution {
			if strings.TrimSpace(command) == "" {
				diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: fmt.Sprintf("spec.referenceSolutions[%d][%d]", solutionIndex, commandIndex), Message: "reference solution commands must be non-empty"})
			}
		}
	}
	for _, item := range s.Spec.Guardrails {
		if item.ID == "" || ids[item.ID] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.guardrails", Message: "guardrail ids must be non-empty and unique"})
		}
		ids[item.ID] = true
	}
	rubrics := map[string]bool{}
	for _, rubric := range s.Spec.JudgeRubrics {
		if rubric.ID == "" || rubrics[rubric.ID] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.judgeRubrics", Message: "rubric ids must be non-empty and unique"})
		}
		rubrics[rubric.ID] = true
	}
	for _, objective := range s.Spec.Objectives {
		if objective.Response != nil && objective.Response.RubricID != "" && !rubrics[objective.Response.RubricID] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.objectives." + objective.ID + ".response.rubricId", Message: "referenced rubric does not exist"})
		}
	}
	for _, rule := range s.Spec.Rules {
		if rule.ID == "" || ids[rule.ID] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.rules", Message: "rule ids must be non-empty and unique"})
		}
		ids[rule.ID] = true
		if err := validateConditionGroup(rule.When, "spec.rules."+rule.ID+".when"); err != nil {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.rules." + rule.ID + ".when", Message: err.Error()})
		}
		for _, effect := range rule.Effects {
			if effect.Type != "setPort" && effect.Type != "setHttpResponse" {
				diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.rules." + rule.ID + ".effects", Message: "unsupported effect type " + effect.Type})
			}
		}
	}
	for _, guardrail := range s.Spec.Guardrails {
		if err := validateConditionGroup(guardrail.When, "spec.guardrails."+guardrail.ID+".when"); err != nil {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.guardrails." + guardrail.ID + ".when", Message: err.Error()})
		}
	}
	for _, entry := range s.Spec.Filesystem.Entries {
		if _, err := safeVirtualPath(entry.Path); err != nil {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.filesystem.entries", Message: err.Error()})
		}
	}
	return diagnostics
}

func validateConditionGroup(group ConditionGroup, location string) error {
	known := map[string]bool{"": true, "serviceRunning": true, "firewallServiceAllowed": true, "selinuxMode": true, "selinuxModeIn": true, "serviceDisabled": true, "httpStatus": true, "fileContains": true, "fileAbsent": true, "userInGroup": true}
	if !known[group.Type] {
		return fmt.Errorf("%s: unsupported condition type %s", location, group.Type)
	}
	for i, child := range group.All {
		if err := validateConditionGroup(child, fmt.Sprintf("%s.all[%d]", location, i)); err != nil {
			return err
		}
	}
	for i, child := range group.Any {
		if err := validateConditionGroup(child, fmt.Sprintf("%s.any[%d]", location, i)); err != nil {
			return err
		}
	}
	if group.Not != nil {
		return validateConditionGroup(*group.Not, location+".not")
	}
	return nil
}

func ValidateEvent(e Event, filename string) []Diagnostic {
	var diagnostics []Diagnostic
	if e.APIVersion != version.Schema {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "apiVersion", Message: "unsupported apiVersion", Hint: "use redlab/v1"})
	}
	if e.Kind != "Event" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "kind", Message: "kind must be Event"})
	}
	if e.Metadata.ID == "" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "metadata.id", Message: "event id is required"})
	}
	seen := map[string]bool{}
	for _, item := range e.Spec.Scenarios {
		if item.Package == "" || seen[item.Package] {
			diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.scenarios", Message: "scenario packages must be non-empty and unique"})
		}
		seen[item.Package] = true
	}
	if e.Spec.Server.Listen == "" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.server.listen", Message: "server listen address is required"})
	}
	if !e.Spec.Schedule.OpensAt.IsZero() && !e.Spec.Schedule.SubmissionsCloseAt.IsZero() && !e.Spec.Schedule.SubmissionsCloseAt.After(e.Spec.Schedule.OpensAt) {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.schedule.submissionsCloseAt", Message: "submissionsCloseAt must be after opensAt"})
	}
	if e.Spec.Sessions.MaxConcurrent < 0 || e.Spec.Sessions.MaxRestarts < 0 {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.sessions", Message: "session limits cannot be negative"})
	}
	if e.Spec.Sessions.Assignment != "" && e.Spec.Sessions.Assignment != "organizer" && e.Spec.Sessions.Assignment != "round-robin" && e.Spec.Sessions.Assignment != "random-seeded" {
		diagnostics = append(diagnostics, Diagnostic{Filename: filename, Path: "spec.sessions.assignment", Message: "unsupported session assignment mode"})
	}
	return diagnostics
}

func safePackagePath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || len(name) > 4096 || strings.ContainsRune(name, '\x00') || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return "", fmt.Errorf("unsafe package path: %q", name)
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("package path escapes package root: %q", name)
	}
	return clean, nil
}

func safeVirtualPath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || !strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("virtual path must be absolute: %q", name)
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "/" || strings.HasPrefix(clean, "/../") || clean == "/.." {
		return "", fmt.Errorf("virtual path escapes root: %q", name)
	}
	return clean, nil
}

func readLimited(filename string, limit int64) ([]byte, error) {
	// #nosec G304 -- this CLI boundary intentionally opens the organizer-supplied scenario or event path.
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d byte limit", limit)
	}
	return data, nil
}

func digestFiles(files map[string][]byte) string {
	hash := sha256.New()
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		hash.Write([]byte(key))
		hash.Write([]byte{0})
		hash.Write(files[key])
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
