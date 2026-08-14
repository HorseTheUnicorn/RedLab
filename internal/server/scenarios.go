package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/redlab/redlab/internal/scenario"
	"gopkg.in/yaml.v3"
)

const maxScenarioUpload = 64 << 20

type scenarioAdminItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Files   int    `json:"files"`
	Source  string `json:"source,omitempty"`
}

type scenarioTemplateRequest struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type scenarioSourceRequest struct {
	YAML string `json:"yaml"`
}

type scenarioFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (s *Server) organizerScenarios(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		writeJSON(w, http.StatusOK, s.scenarioItems())
		return
	}
	switch parts[0] {
	case "template":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		s.createScenarioTemplate(w, r)
		return
	case "import":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		s.importScenario(w, r)
		return
	}
	id, err := url.PathUnescape(parts[0])
	if err != nil || !validScenarioID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid scenario id"))
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.removeScenario(w, id)
		return
	}
	if len(parts) < 2 {
		writeError(w, http.StatusNotFound, errors.New("scenario resource not found"))
		return
	}
	switch parts[1] {
	case "export":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		s.exportScenario(w, id)
	case "source":
		s.editScenarioSource(w, r, id)
	case "files":
		s.editScenarioFiles(w, r, id)
	default:
		writeError(w, http.StatusNotFound, errors.New("scenario resource not found"))
	}
}

func (s *Server) scenarioItems() []scenarioAdminItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.Packages))
	for id := range s.Packages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := make([]scenarioAdminItem, 0, len(ids))
	for _, id := range ids {
		pkg := s.Packages[id]
		items = append(items, scenarioAdminItem{ID: id, Title: pkg.Scenario.Metadata.Title, Version: pkg.Scenario.Metadata.Version, Digest: pkg.Digest, Files: len(pkg.Files), Source: s.ScenarioSources[id]})
	}
	return items
}

func (s *Server) createScenarioTemplate(w http.ResponseWriter, r *http.Request) {
	if err := s.requireScenarioMutationWindow(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var req scenarioTemplateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid template request"))
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Title = strings.TrimSpace(req.Title)
	if !validScenarioID(req.ID) {
		writeError(w, http.StatusBadRequest, errors.New("scenario id must contain 1-64 lowercase letters, numbers, and hyphens"))
		return
	}
	if req.Title == "" {
		req.Title = req.ID
	}
	s.mu.RLock()
	_, exists := s.Packages[req.ID]
	s.mu.RUnlock()
	if exists {
		writeError(w, http.StatusConflict, errors.New("scenario already exists"))
		return
	}
	data := scenario.TemplateYAML(req.ID, req.Title)
	pkg, diagnostics := scenario.LoadScenarioYAML(data, "dashboard:scenario.yaml", map[string][]byte{})
	if len(diagnostics) > 0 {
		writeError(w, http.StatusBadRequest, diagnosticsError(diagnostics))
		return
	}
	if err := s.saveManagedScenario(pkg); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.itemForScenario(pkg))
}

func (s *Server) importScenario(w http.ResponseWriter, r *http.Request) {
	if err := s.requireScenarioMutationWindow(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxScenarioUpload+(1<<20))
	// #nosec G120 -- MaxBytesReader caps the entire body and ParseMultipartForm caps in-memory buffering.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("scenario upload must be multipart form data"))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("scenario upload field `file` is required"))
		return
	}
	defer file.Close()
	root := filepath.Join(filepath.Dir(s.EventFile), "data")
	if err := os.MkdirAll(root, 0700); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	temp, err := os.CreateTemp(root, "scenario-import-*.rlab")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	written, err := io.Copy(temp, io.LimitReader(file, maxScenarioUpload+1))
	if err != nil {
		_ = temp.Close()
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if written > maxScenarioUpload {
		_ = temp.Close()
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("scenario upload exceeds the 64 MiB limit"))
		return
	}
	if err := temp.Close(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	pkg, diagnostics := scenario.LoadScenario(tempName)
	if len(diagnostics) > 0 {
		writeError(w, http.StatusBadRequest, diagnosticsError(diagnostics))
		return
	}
	if !validScenarioID(pkg.Scenario.Metadata.ID) {
		writeError(w, http.StatusBadRequest, errors.New("scenario metadata.id is not a safe package id"))
		return
	}
	s.mu.RLock()
	_, exists := s.Packages[pkg.Scenario.Metadata.ID]
	s.mu.RUnlock()
	if exists {
		writeError(w, http.StatusConflict, errors.New("scenario already exists"))
		return
	}
	if err := s.saveManagedScenario(pkg); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.itemForScenario(pkg))
}

func (s *Server) editScenarioSource(w http.ResponseWriter, r *http.Request, id string) {
	pkg, ok := s.packageSnapshot(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("scenario not found"))
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "digest": pkg.Digest, "yaml": string(pkg.ScenarioYAML())})
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if err := s.requireScenarioMutationWindow(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var req scenarioSourceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil || strings.TrimSpace(req.YAML) == "" {
		writeError(w, http.StatusBadRequest, errors.New("yaml is required"))
		return
	}
	updated, diagnostics := scenario.LoadScenarioYAML([]byte(req.YAML), "dashboard:scenario.yaml", pkg.Files)
	if len(diagnostics) > 0 {
		writeError(w, http.StatusBadRequest, diagnosticsError(diagnostics))
		return
	}
	if updated.Scenario.Metadata.ID != id {
		writeError(w, http.StatusBadRequest, errors.New("metadata.id cannot change during an edit; create or import a new scenario instead"))
		return
	}
	if err := s.saveManagedScenario(updated); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.itemForScenario(updated))
}

func (s *Server) editScenarioFiles(w http.ResponseWriter, r *http.Request, id string) {
	pkg, ok := s.packageSnapshot(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("scenario not found"))
		return
	}
	if r.Method == http.MethodGet {
		type item struct {
			Path    string `json:"path"`
			Size    int    `json:"size"`
			Content string `json:"content,omitempty"`
			Text    bool   `json:"text"`
		}
		keys := make([]string, 0, len(pkg.Files))
		for key := range pkg.Files {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]item, 0, len(keys))
		for _, key := range keys {
			data := pkg.Files[key]
			entry := item{Path: key, Size: len(data), Text: true}
			if len(data) <= 256<<10 {
				entry.Content = string(data)
			} else {
				entry.Text = false
			}
			out = append(out, entry)
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if err := s.requireScenarioMutationWindow(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	if r.Method == http.MethodPut {
		var req scenarioFileRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			writeError(w, http.StatusBadRequest, errors.New("path and content are required"))
			return
		}
		path, err := scenario.ValidatePackagePath(req.Path)
		if err != nil || path == "scenario.yaml" {
			writeError(w, http.StatusBadRequest, errors.New("file path is invalid or reserved"))
			return
		}
		pkg.Files[path] = []byte(req.Content)
		pkg.Digest = pkg.RecomputeDigest()
		if err := s.saveManagedScenario(pkg); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, s.itemForScenario(pkg))
		return
	}
	if r.Method == http.MethodDelete {
		path, err := scenario.ValidatePackagePath(r.URL.Query().Get("path"))
		if err != nil || path == "scenario.yaml" {
			writeError(w, http.StatusBadRequest, errors.New("file path is invalid or reserved"))
			return
		}
		if _, exists := pkg.Files[path]; !exists {
			writeError(w, http.StatusNotFound, errors.New("file not found"))
			return
		}
		delete(pkg.Files, path)
		pkg.Digest = pkg.RecomputeDigest()
		if err := s.saveManagedScenario(pkg); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, s.itemForScenario(pkg))
		return
	}
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func (s *Server) exportScenario(w http.ResponseWriter, id string) {
	pkg, ok := s.packageSnapshot(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("scenario not found"))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".rlab"))
	if err := pkg.WriteArchive(w); err != nil {
		return
	}
}

func (s *Server) removeScenario(w http.ResponseWriter, id string) {
	if err := s.requireScenarioMutationWindow(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	s.mu.Lock()
	oldPkg, ok := s.Packages[id]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, errors.New("scenario not found"))
		return
	}
	oldSource := s.ScenarioSources[id]
	oldEvent := s.Event
	delete(s.Packages, id)
	delete(s.ScenarioSources, id)
	refs := make([]scenario.EventScenario, 0, len(s.Event.Spec.Scenarios))
	for _, ref := range s.Event.Spec.Scenarios {
		if oldSource != "" && samePath(oldSource, ref.Package, filepath.Dir(s.EventFile)) {
			continue
		}
		refs = append(refs, ref)
	}
	s.Event.Spec.Scenarios = refs
	event := s.Event
	s.mu.Unlock()
	if err := s.persistEvent(event); err != nil {
		s.mu.Lock()
		s.Packages[id] = oldPkg
		s.ScenarioSources[id] = oldSource
		s.Event = oldEvent
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "id": id})
}

func (s *Server) packageSnapshot(id string) (scenario.Package, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pkg, ok := s.Packages[id]
	if !ok {
		return scenario.Package{}, false
	}
	return pkg.Clone(), true
}

func (s *Server) itemForScenario(pkg scenario.Package) scenarioAdminItem {
	s.mu.RLock()
	source := s.ScenarioSources[pkg.Scenario.Metadata.ID]
	s.mu.RUnlock()
	return scenarioAdminItem{ID: pkg.Scenario.Metadata.ID, Title: pkg.Scenario.Metadata.Title, Version: pkg.Scenario.Metadata.Version, Digest: pkg.Digest, Files: len(pkg.Files), Source: source}
}

func (s *Server) saveManagedScenario(pkg scenario.Package) error {
	if !validScenarioID(pkg.Scenario.Metadata.ID) {
		return errors.New("scenario metadata.id is not a safe package id")
	}
	if err := s.requireScenarioMutationWindow(); err != nil {
		return err
	}
	archivePath := filepath.Join(filepath.Dir(s.EventFile), "data", "scenarios", pkg.Scenario.Metadata.ID+".rlab")
	if err := pkg.WriteArchiveFile(archivePath); err != nil {
		return err
	}
	managedSource := filepath.ToSlash(filepath.Join(".", "data", "scenarios", pkg.Scenario.Metadata.ID+".rlab"))
	pkg.Root = archivePath
	s.mu.Lock()
	id := pkg.Scenario.Metadata.ID
	oldPkg, hadPkg := s.Packages[id]
	oldSource, hadSource := s.ScenarioSources[id]
	oldEvent := s.Event
	s.Packages[pkg.Scenario.Metadata.ID] = pkg
	s.ScenarioSources[pkg.Scenario.Metadata.ID] = managedSource
	found := false
	for index, ref := range s.Event.Spec.Scenarios {
		if oldSource != "" && samePath(oldSource, ref.Package, filepath.Dir(s.EventFile)) {
			s.Event.Spec.Scenarios[index].Package = managedSource
			found = true
			break
		}
	}
	if !found {
		s.Event.Spec.Scenarios = append(s.Event.Spec.Scenarios, scenario.EventScenario{Package: managedSource, Enabled: true})
	}
	event := s.Event
	s.mu.Unlock()
	if err := s.persistEvent(event); err != nil {
		s.mu.Lock()
		if hadPkg {
			s.Packages[id] = oldPkg
		} else {
			delete(s.Packages, id)
		}
		if hadSource {
			s.ScenarioSources[id] = oldSource
		} else {
			delete(s.ScenarioSources, id)
		}
		s.Event = oldEvent
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *Server) requireScenarioMutationWindow() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Sessions) > 0 {
		return errors.New("scenario editing is locked while sessions exist; start a fresh event before changing packages")
	}
	return nil
}

func (s *Server) persistEvent(event scenario.Event) error {
	data, err := yaml.Marshal(event)
	if err != nil {
		return err
	}
	return writeFileAtomic(s.EventFile, data, 0600)
}

func diagnosticsError(diagnostics []scenario.Diagnostic) error {
	if len(diagnostics) == 0 {
		return errors.New("scenario validation failed")
	}
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Error())
	}
	return errors.New(strings.Join(parts, "; "))
}

func validScenarioID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for index, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || (index > 0 && char == '-') {
			continue
		}
		return false
	}
	return id[0] != '-' && id[len(id)-1] != '-'
}

func samePath(left, right, base string) bool {
	if left == "" || right == "" {
		return false
	}
	if !filepath.IsAbs(left) {
		left = filepath.Join(base, filepath.FromSlash(left))
	}
	if !filepath.IsAbs(right) {
		right = filepath.Join(base, filepath.FromSlash(right))
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
