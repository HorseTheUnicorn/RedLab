package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/redlab/redlab/internal/auth"
	"github.com/redlab/redlab/internal/report"
	"github.com/redlab/redlab/internal/runtime"
	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/store"
)

type Server struct {
	Event           scenario.Event
	EventFile       string
	Packages        map[string]scenario.Package
	Store           *store.Store
	Sessions        map[string]*runtime.Session
	Restarts        map[string]int
	Activity        map[string]time.Time
	Tokens          map[string]string
	TokenExpiry     map[string]time.Time
	RefreshTokens   map[string]string
	SigningKey      []byte
	securityErr     error
	Credentials     auth.File
	credentialErr   error
	manualClose     bool
	ScenarioSources map[string]string
	rateMu          sync.Mutex
	rate            map[string]rateWindow
	mu              sync.RWMutex
	HTTP            *http.Server
	upgrader        websocket.Upgrader
}

type loginRequest struct {
	TeamID    string `json:"teamID"`
	JoinCode  string `json:"joinCode"`
	Password  string `json:"password"`
	LinkToken string `json:"linkToken"`
}
type commandRequest struct {
	Command string `json:"command"`
}
type judgeRequest struct {
	Score int    `json:"score"`
	Notes string `json:"notes"`
}
type terminalMessage struct {
	Type     string `json:"type"`
	Sequence uint64 `json:"sequence,omitempty"`
	Ack      uint64 `json:"ack,omitempty"`
	Cursor   uint64 `json:"cursor,omitempty"`
	Width    uint16 `json:"width,omitempty"`
	Height   uint16 `json:"height,omitempty"`
	Data     string `json:"data,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
}
type rateWindow struct {
	Started time.Time
	Count   int
}

func New(eventFile string, event scenario.Event, packages map[string]scenario.Package, db *store.Store) *Server {
	credentials, credentialErr := auth.Load(filepath.Join(filepath.Dir(eventFile), "data", "credentials.json"))
	if os.IsNotExist(credentialErr) {
		credentialErr = nil
	} else if credentialErr == nil {
		credentialErr = auth.EnsureValid(credentials)
	}
	var signingKey []byte
	var securityErr error
	if db != nil {
		signingKey, securityErr = db.SigningKey()
	} else {
		signingKey = make([]byte, 32)
		_, securityErr = rand.Read(signingKey)
	}
	sources := map[string]string{}
	for _, reference := range event.Spec.Scenarios {
		for id, pkg := range packages {
			if samePath(pkg.Root, reference.Package, filepath.Dir(eventFile)) {
				sources[id] = reference.Package
			}
		}
	}
	s := &Server{Event: event, EventFile: eventFile, Packages: packages, Store: db, Sessions: map[string]*runtime.Session{}, Restarts: map[string]int{}, Activity: map[string]time.Time{}, Tokens: map[string]string{}, TokenExpiry: map[string]time.Time{}, RefreshTokens: map[string]string{}, SigningKey: signingKey, securityErr: securityErr, Credentials: credentials, credentialErr: credentialErr, rate: map[string]rateWindow{}, ScenarioSources: sources, upgrader: websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096}}
	s.HTTP = &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	return s
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/healthz", s.health)
	mux.HandleFunc("/api/v1/readyz", s.ready)
	mux.HandleFunc("/api/v1/preflight", s.preflight)
	mux.HandleFunc("/api/v1/auth/team/login", s.teamLogin)
	mux.HandleFunc("/api/v1/auth/link", s.linkLogin)
	mux.HandleFunc("/api/v1/auth/organizer/login", s.organizerLogin)
	mux.HandleFunc("/api/v1/auth/refresh", s.refresh)
	mux.HandleFunc("/api/v1/event", s.event)
	mux.HandleFunc("/api/v1/event/", s.event)
	mux.HandleFunc("/api/v1/event/scenarios", s.scenarios)
	mux.HandleFunc("/api/v1/organizer/", s.organizer)
	mux.HandleFunc("/api/v1/teams/", s.teams)
	mux.HandleFunc("/api/v1/sessions/", s.sessions)
	mux.HandleFunc("/api/v1/submissions/", s.submissions)
	mux.HandleFunc("/", s.dashboard)
	return s.limitMiddleware(mux)
}
func (s *Server) ListenAndServe(address string) error {
	s.HTTP.Addr = address
	return s.HTTP.ListenAndServe()
}
func (s *Server) Shutdown() error {
	ctx, cancel := contextWithTimeout()
	defer cancel()
	return s.HTTP.Shutdown(ctx)
}

func (s *Server) Recover() error {
	if s.Store == nil {
		return nil
	}
	records, err := s.Store.ListSessions()
	if err != nil {
		return err
	}
	meta, err := s.Store.ListSessionMeta()
	if err != nil {
		return err
	}
	closed, err := s.Store.ManualClosed()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.manualClose = closed
	s.mu.Unlock()
	for _, record := range records {
		var pkg scenario.Package
		found := false
		for _, candidate := range s.Packages {
			if candidate.Scenario.Metadata.ID == record.ScenarioID {
				pkg = candidate
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("session %s references unavailable scenario %s", record.ID, record.ScenarioID)
		}
		events, err := s.Store.LoadEvents(record.ID)
		if err != nil {
			return err
		}
		var session *runtime.Session
		if len(events) == 0 {
			session, err = runtime.NewSession(record.ID, record.TeamID, pkg, record.CreatedAt)
		} else {
			session, err = runtime.Replay(record.ID, record.TeamID, pkg, events)
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.Sessions[record.ID] = session
		if saved, ok := meta[record.ID]; ok {
			s.Restarts[record.ID] = saved.Restarts
			if saved.Judged {
				_ = session.SetJudge(saved.JudgeScore, saved.JudgeNotes)
			}
			if saved.Submitted {
				submittedAt := record.SubmittedAt
				if submittedAt.IsZero() {
					submittedAt = record.CreatedAt
				}
				session.RestoreSubmission(submittedAt)
			}
			if saved.LastActivity.IsZero() {
				s.Activity[record.ID] = time.Now()
			} else {
				s.Activity[record.ID] = saved.LastActivity
			}
		} else {
			s.Activity[record.ID] = time.Now()
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	if s.Store != nil {
		if err := s.Store.Health(); err != nil {
			writeError(w, 503, err)
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if len(s.Packages) == 0 || s.eventStatus() == "scheduled" {
		writeError(w, 503, errors.New("no scenario packages loaded"))
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}
func (s *Server) preflight(w http.ResponseWriter, _ *http.Request) {
	checks := map[string]string{}
	status := http.StatusOK
	if s.Store == nil {
		checks["database"] = "missing"
		status = http.StatusServiceUnavailable
	} else if err := s.Store.Health(); err != nil {
		checks["database"] = err.Error()
		status = http.StatusServiceUnavailable
	} else {
		checks["database"] = "ok"
	}
	if len(s.Packages) == 0 {
		checks["scenarios"] = "missing"
		status = http.StatusServiceUnavailable
	} else {
		checks["scenarios"] = fmt.Sprintf("%d loaded", len(s.Packages))
	}
	if s.Event.Metadata.ID == "" {
		checks["event"] = "missing id"
		status = http.StatusServiceUnavailable
	} else {
		checks["event"] = "ok"
	}
	checks["schedule"] = s.eventStatus()
	writeJSON(w, status, map[string]any{"status": map[int]string{http.StatusOK: "ok", http.StatusServiceUnavailable: "failed"}[status], "checks": checks})
}
func (s *Server) teamLogin(w http.ResponseWriter, r *http.Request) {
	if s.credentialErr != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credential store is unavailable"))
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || req.TeamID == "" {
		writeError(w, 400, errors.New("teamID is required"))
		return
	}
	access, refresh, err := s.issueTeamTokens(req.TeamID, req.JoinCode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, map[string]string{"accessToken": access, "refreshToken": refresh, "expiresIn": "1800", "teamID": req.TeamID})
}

func (s *Server) linkLogin(w http.ResponseWriter, r *http.Request) {
	if s.credentialErr != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credential store is unavailable"))
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || req.TeamID == "" || req.JoinCode == "" || req.LinkToken == "" {
		writeError(w, http.StatusUnauthorized, errors.New("teamID, joinCode, and linkToken are required"))
		return
	}
	s.mu.RLock()
	link := s.Credentials.Link
	s.mu.RUnlock()
	if !auth.Verify(link, req.LinkToken) {
		writeError(w, http.StatusUnauthorized, errors.New("invalid event link token"))
		return
	}
	access, refresh, err := s.issueTeamTokens(req.TeamID, req.JoinCode)
	if err != nil {
		if strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "unknown team") {
			writeError(w, http.StatusUnauthorized, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"accessToken": access, "refreshToken": refresh, "expiresIn": "1800", "teamID": req.TeamID})
}

func (s *Server) issueTeamTokens(teamID, joinCode string) (string, string, error) {
	s.mu.RLock()
	teams := s.Credentials.Teams
	s.mu.RUnlock()
	if record, configured := teams[teamID]; configured && !auth.Verify(record, joinCode) {
		return "", "", errors.New("invalid team credentials")
	}
	if len(teams) > 0 {
		if _, configured := teams[teamID]; !configured {
			return "", "", errors.New("unknown team")
		}
	}
	return s.issueTokens(teamID)
}
func (s *Server) organizerLogin(w http.ResponseWriter, r *http.Request) {
	if s.credentialErr != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credential store is unavailable"))
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || req.Password == "" {
		writeError(w, 401, errors.New("password is required"))
		return
	}
	if s.Credentials.Organizer.Hash != "" && !auth.Verify(s.Credentials.Organizer, req.Password) {
		writeError(w, http.StatusUnauthorized, errors.New("invalid organizer credentials"))
		return
	}
	access, refresh, err := s.issueTokens("organizer")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, map[string]string{"accessToken": access, "refreshToken": refresh, "expiresIn": "1800"})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("refreshToken is required"))
		return
	}
	hash := hashToken(req.RefreshToken)
	var owner string
	var ok bool
	if s.Store != nil {
		var err error
		owner, ok, err = s.Store.RotateRefreshToken(hash, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		s.mu.Lock()
		owner, ok = s.RefreshTokens[hash]
		if ok {
			delete(s.RefreshTokens, hash)
		}
		s.mu.Unlock()
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("invalid or already-rotated refresh token"))
		return
	}
	access, refresh, err := s.issueTokens(owner)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"accessToken": access, "refreshToken": refresh, "expiresIn": "1800"})
}
func (s *Server) event(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/status") {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"eventID": s.Event.Metadata.ID, "status": s.eventStatus(), "now": time.Now().UTC()})
		return
	}
	if r.Method != "GET" {
		writeError(w, 405, errors.New("method not allowed"))
		return
	}
	writeJSON(w, 200, s.Event)
}
func (s *Server) scenarios(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, errors.New("method not allowed"))
		return
	}
	type item struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Version string `json:"version"`
		Digest  string `json:"digest"`
	}
	var out []item
	for _, pkg := range s.Packages {
		out = append(out, item{pkg.Scenario.Metadata.ID, pkg.Scenario.Metadata.Title, pkg.Scenario.Metadata.Version, pkg.Digest})
	}
	writeJSON(w, 200, out)
}

func (s *Server) organizer(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r, "organizer") || !s.isOrganizer(r) {
		writeError(w, http.StatusUnauthorized, errors.New("organizer authentication required"))
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) >= 4 && parts[3] == "scenarios" {
		s.organizerScenarios(w, r, parts[4:])
		return
	}
	if len(parts) == 4 && parts[3] == "link-token" {
		if r.Method == http.MethodGet {
			s.mu.RLock()
			configured := s.Credentials.Link.Hash != ""
			s.mu.RUnlock()
			writeJSON(w, http.StatusOK, map[string]bool{"configured": configured})
			return
		}
		if r.Method == http.MethodPost {
			s.rotateLinkToken(w)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if len(parts) >= 5 && ((parts[3] == "event" && (parts[4] == "close" || parts[4] == "reopen")) || (parts[3] == "submissions" && parts[4] == "close")) {
		if r.Method != http.MethodPost || (parts[4] != "close" && parts[4] != "reopen") {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		s.mu.Lock()
		s.manualClose = parts[4] == "close"
		s.mu.Unlock()
		if s.Store != nil {
			if err := s.Store.SetManualClosed(parts[4] == "close"); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": s.eventStatus()})
		return
	}
	if len(parts) == 4 && parts[3] == "sessions" && r.Method == http.MethodGet {
		s.mu.RLock()
		sessions := make([]*runtime.Session, 0, len(s.Sessions))
		for _, session := range s.Sessions {
			sessions = append(sessions, session)
		}
		s.mu.RUnlock()
		sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID < sessions[j].ID })
		out := make([]report.Model, 0, len(sessions))
		for _, session := range sessions {
			out = append(out, session.Report(s.Event.Metadata.ID))
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if len(parts) == 4 && parts[3] == "submissions" && r.Method == http.MethodGet {
		s.mu.RLock()
		sessions := make([]*runtime.Session, 0, len(s.Sessions))
		for _, session := range s.Sessions {
			if session.IsSubmitted() {
				sessions = append(sessions, session)
			}
		}
		s.mu.RUnlock()
		out := make([]report.Model, 0, len(sessions))
		for _, session := range sessions {
			out = append(out, session.Report(s.Event.Metadata.ID))
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if len(parts) == 6 && parts[3] == "sessions" && parts[5] == "judge" && r.Method == http.MethodPost {
		s.mu.RLock()
		session := s.Sessions[parts[4]]
		s.mu.RUnlock()
		if session == nil {
			writeError(w, http.StatusNotFound, errors.New("session not found"))
			return
		}
		var req judgeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid judge request"))
			return
		}
		if err := session.SetJudge(req.Score, req.Notes); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.persistSession(session); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, session.Report(s.Event.Metadata.ID))
		return
	}
	writeError(w, http.StatusNotFound, errors.New("not found"))
}

func (s *Server) rotateLinkToken(w http.ResponseWriter) {
	token, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	record, err := auth.NewRecord(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	credentialsPath := filepath.Join(filepath.Dir(s.EventFile), "data", "credentials.json")
	s.mu.Lock()
	credentials := s.Credentials
	credentials.Link = record
	s.mu.Unlock()
	if err := auth.Save(credentialsPath, credentials); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.mu.Lock()
	s.Credentials = credentials
	s.credentialErr = nil
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "rotatedAt": time.Now().UTC()})
}

func (s *Server) isOrganizer(r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return s.tokenOwner(token) == "organizer"
}

func (s *Server) eventStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.manualClose {
		return "closed"
	}
	now := time.Now().UTC()
	if !s.Event.Spec.Schedule.OpensAt.IsZero() && now.Before(s.Event.Spec.Schedule.OpensAt) {
		return "scheduled"
	}
	if !s.Event.Spec.Schedule.SubmissionsCloseAt.IsZero() && !now.Before(s.Event.Spec.Schedule.SubmissionsCloseAt) {
		return "closed"
	}
	return "open"
}

func (s *Server) packageForTeam(teamID string) (scenario.Package, bool) {
	ids := make([]string, 0, len(s.Packages))
	for id := range s.Packages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return scenario.Package{}, false
	}
	sum := sha256.Sum256([]byte(s.Event.Metadata.ID + "\x00" + teamID))
	value := uint64(0)
	for _, b := range sum[:8] {
		value = value<<8 | uint64(b)
	}
	return s.Packages[ids[value%uint64(len(ids))]], true
}
func (s *Server) teams(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "teams" {
		writeError(w, 404, errors.New("not found"))
		return
	}
	teamID := parts[3]
	if !s.authorize(r, teamID) {
		writeError(w, 401, errors.New("authentication required"))
		return
	}
	if r.Method == "GET" && len(parts) == 5 && parts[4] == "session" {
		if session := s.sessionForTeam(teamID); session != nil {
			writeJSON(w, 200, session.Report(s.Event.Metadata.ID))
			return
		}
		writeError(w, 404, errors.New("session not found"))
		return
	}
	if r.Method == "POST" && len(parts) == 5 && parts[4] == "session" {
		s.createSession(w, teamID)
		return
	}
	writeError(w, 404, errors.New("not found"))
}
func (s *Server) createSession(w http.ResponseWriter, teamID string) {
	if s.eventStatus() != "open" {
		writeError(w, http.StatusForbidden, errors.New("event is not accepting new sessions"))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.sessionForTeamLocked(teamID); existing != nil {
		writeJSON(w, 200, map[string]string{"sessionID": existing.ID})
		return
	}
	if s.Event.Spec.Sessions.MaxConcurrent > 0 && len(s.Sessions) >= s.Event.Spec.Sessions.MaxConcurrent {
		writeError(w, http.StatusServiceUnavailable, errors.New("maximum concurrent sessions reached"))
		return
	}
	pkg, ok := s.packageForTeam(teamID)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("no scenario available"))
		return
	}
	id := "session-" + newToken(teamID)[:16]
	seed := deterministicSeed(s.Event.Metadata.ID, teamID, pkg.Scenario.Metadata.ID)
	session, err := runtime.NewSession(id, teamID, pkg, seed)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	s.Sessions[id] = session
	s.Activity[id] = time.Now()
	if s.Store != nil {
		state, _ := session.State.SnapshotJSON()
		if err := s.Store.SaveSession(id, teamID, pkg.Scenario.Metadata.ID, state, seed); err != nil {
			writeError(w, 500, err)
			return
		}
		if err := s.Store.SaveSessionMeta(store.SessionMeta{SessionID: id, LastActivity: s.Activity[id]}); err != nil {
			writeError(w, 500, err)
			return
		}
	}
	writeJSON(w, 201, map[string]string{"sessionID": id, "scenarioID": pkg.Scenario.Metadata.ID})
}
func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "sessions" {
		writeError(w, 404, errors.New("not found"))
		return
	}
	id := parts[3]
	s.mu.RLock()
	session := s.Sessions[id]
	s.mu.RUnlock()
	if session == nil {
		writeError(w, 404, errors.New("session not found"))
		return
	}
	if !s.authorize(r, session.TeamID) {
		writeError(w, 401, errors.New("authentication required"))
		return
	}
	if !s.touchActivity(id) {
		writeError(w, http.StatusGone, errors.New("session expired due to inactivity"))
		return
	}
	if s.eventStatus() == "closed" && (r.Method == http.MethodPost || (r.Method == http.MethodGet && len(parts) >= 5 && parts[4] == "terminal")) {
		writeError(w, http.StatusForbidden, errors.New("event is closed"))
		return
	}
	if r.Method == http.MethodGet && len(parts) == 4 {
		writeJSON(w, http.StatusOK, session.Report(s.Event.Metadata.ID))
		return
	}
	if len(parts) >= 5 && parts[4] == "terminal" {
		s.terminal(w, r, session)
		return
	}
	if len(parts) >= 5 && parts[4] == "objectives" {
		writeJSON(w, 200, session.Report(s.Event.Metadata.ID).Score)
		return
	}
	if r.Method == http.MethodPost && len(parts) == 6 && parts[4] == "hints" {
		if !s.Event.Spec.Scoring.HintsEnabled {
			writeError(w, http.StatusForbidden, errors.New("hints are disabled for this event"))
			return
		}
		result := session.RunLine("lab hint " + strconv.Quote(parts[5]))
		if err := s.persistSession(session); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if r.Method == http.MethodPost && len(parts) == 5 && parts[4] == "notes" {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
			writeError(w, http.StatusBadRequest, errors.New("note text is required"))
			return
		}
		if len(req.Text) > 8192 {
			writeError(w, http.StatusRequestEntityTooLarge, errors.New("note exceeds 8192 bytes"))
			return
		}
		result := session.RunLine("lab note " + strconv.Quote(req.Text))
		if err := s.persistSession(session); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if r.Method != "POST" || len(parts) < 5 {
		writeError(w, 405, errors.New("method not allowed"))
		return
	}
	switch parts[4] {
	case "command":
		var req commandRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || req.Command == "" {
			writeError(w, 400, errors.New("command is required"))
			return
		}
		result := session.RunLine(req.Command)
		if s.Store != nil {
			if err := s.persistSession(session); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, 200, result)
	case "check":
		result := session.RunLine("lab check")
		if err := s.persistSession(session); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, 200, result)
	case "submit":
		if s.eventStatus() == "closed" {
			writeError(w, http.StatusForbidden, errors.New("submissions are closed"))
			return
		}
		if session.IsSubmitted() {
			writeError(w, http.StatusConflict, errors.New("session has already been submitted"))
			return
		}
		result := session.RunLine("lab submit")
		if result.ExitCode == 0 {
			if err := s.persistSession(session); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if err := s.persistSubmission(session); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if s.Store != nil {
				_ = s.Store.MarkSubmitted(session.ID, time.Now().UTC())
			}
		}
		writeJSON(w, 200, result)
	case "restart":
		if session.IsSubmitted() {
			writeError(w, http.StatusConflict, errors.New("submitted sessions cannot be restarted"))
			return
		}
		if !s.Event.Spec.Sessions.AllowRestart {
			writeError(w, http.StatusForbidden, errors.New("session restart is disabled"))
			return
		}
		s.mu.Lock()
		count := s.Restarts[id]
		if s.Event.Spec.Sessions.MaxRestarts > 0 && count >= s.Event.Spec.Sessions.MaxRestarts {
			s.mu.Unlock()
			writeError(w, http.StatusForbidden, errors.New("restart limit reached"))
			return
		}
		s.Restarts[id] = count + 1
		s.mu.Unlock()
		if err := session.Restart(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if s.Store != nil {
			_ = s.Store.ClearEvidence(id)
			if err := s.persistSession(session); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessionID": id, "restartsUsed": count + 1})
	default:
		writeError(w, 404, errors.New("not found"))
	}
}

func (s *Server) persistSession(session *runtime.Session) error {
	if s.Store == nil {
		return nil
	}
	state, err := session.State.SnapshotJSON()
	if err != nil {
		return err
	}
	if err := s.Store.SaveSession(session.ID, session.TeamID, session.Scenario.Metadata.ID, state, session.Started); err != nil {
		return err
	}
	model := session.Report(s.Event.Metadata.ID)
	for _, event := range model.Timeline {
		if err := s.Store.SaveEvidence(event); err != nil {
			return err
		}
	}
	s.mu.RLock()
	restarts := s.Restarts[session.ID]
	activity := s.Activity[session.ID]
	s.mu.RUnlock()
	if activity.IsZero() {
		activity = time.Now()
	}
	if err := s.Store.SaveSessionMeta(store.SessionMeta{SessionID: session.ID, Restarts: restarts, JudgeScore: model.JudgeScore, JudgeNotes: model.JudgeNotes, Judged: model.Judged, Submitted: session.IsSubmitted(), LastActivity: activity}); err != nil {
		return err
	}
	return nil
}

func (s *Server) touchActivity(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if previous, ok := s.Activity[id]; ok && s.Event.Spec.Sessions.IdleTimeout.Duration() > 0 && now.Sub(previous) > s.Event.Spec.Sessions.IdleTimeout.Duration() {
		return false
	}
	s.Activity[id] = now
	return true
}

func (s *Server) persistSubmission(session *runtime.Session) error {
	root := filepath.Join(filepath.Dir(s.EventFile), "data", "submissions")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	bundleFile := filepath.Join(root, session.ID+".rlab.zip")
	if err := session.ExportBundle(bundleFile, s.Event.Metadata.ID); err != nil {
		return err
	}
	if s.Store != nil {
		model := session.Report(s.Event.Metadata.ID)
		data, err := report.JSON(model)
		if err != nil {
			return err
		}
		if err := s.Store.SaveSubmission(session.ID, session.ID, data, report.Markdown(model, session.Scenario.Spec), session.State.CurrentTime()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) submissions(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeError(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	submissionID := parts[3]
	s.mu.RLock()
	session := s.Sessions[submissionID]
	s.mu.RUnlock()
	if session == nil {
		writeError(w, http.StatusNotFound, errors.New("submission not found"))
		return
	}
	if !s.authorize(r, session.TeamID) {
		writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
		return
	}
	if r.Method == http.MethodGet && len(parts) == 4 {
		writeJSON(w, http.StatusOK, session.Report(s.Event.Metadata.ID))
		return
	}
	if r.Method == http.MethodPost && len(parts) == 5 && parts[4] == "export" {
		root := filepath.Join(filepath.Dir(s.EventFile), "data", "submissions")
		if err := os.MkdirAll(root, 0700); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := session.ExportBundle(filepath.Join(root, submissionID+".rlab.zip"), s.Event.Metadata.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"bundle": submissionID + ".rlab.zip"})
		return
	}
	if r.Method == http.MethodPost && len(parts) == 5 && parts[4] == "judging" {
		var req judgeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid judging request"))
			return
		}
		if !s.isOrganizer(r) {
			writeError(w, http.StatusUnauthorized, errors.New("organizer authentication required"))
			return
		}
		if err := session.SetJudge(req.Score, req.Notes); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.persistSession(session); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, session.Report(s.Event.Metadata.ID))
		return
	}
	writeError(w, http.StatusNotFound, errors.New("not found"))
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/dashboard" {
		writeError(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}
func (s *Server) terminal(w http.ResponseWriter, r *http.Request, session *runtime.Session) {
	if r.Method != "GET" {
		writeError(w, 405, errors.New("method not allowed"))
		return
	}
	connection, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(60 * time.Minute))
	connection.SetReadLimit(64 << 10)
	sequence := uint64(len(session.Report(s.Event.Metadata.ID).Timeline))
	for {
		var message terminalMessage
		if err := connection.ReadJSON(&message); err != nil {
			return
		}
		switch message.Type {
		case "input":
			if len(message.Data) > 16<<10 {
				_ = connection.WriteJSON(terminalMessage{Type: "error", Sequence: sequence, Cursor: sequence, Stderr: "input exceeds 16 KiB\n", ExitCode: 2})
				continue
			}
			result := session.RunLine(message.Data)
			events := session.Report(s.Event.Metadata.ID).Timeline
			if len(events) > 0 {
				sequence = events[len(events)-1].Sequence
			} else {
				sequence++
			}
			if err := connection.WriteJSON(terminalMessage{Type: "output", Sequence: sequence, Ack: message.Sequence, Cursor: sequence, Stdout: limitOutput(result.Stdout), Stderr: limitOutput(result.Stderr), ExitCode: result.ExitCode}); err != nil {
				return
			}
		case "resume":
			cursor := message.Cursor
			for _, event := range session.Report(s.Event.Metadata.ID).Timeline {
				if event.Sequence <= cursor {
					continue
				}
				if err := connection.WriteJSON(terminalMessage{Type: "replay", Sequence: event.Sequence, Cursor: event.Sequence, Data: event.Command, ExitCode: event.ExitCode}); err != nil {
					return
				}
			}
			if err := connection.WriteJSON(terminalMessage{Type: "ack", Sequence: cursor, Ack: cursor, Cursor: sequence}); err != nil {
				return
			}
		case "ack":
			if message.Sequence > sequence {
				_ = connection.WriteJSON(terminalMessage{Type: "error", Sequence: sequence, Cursor: sequence, Stderr: "acknowledgment cursor is ahead of the terminal\n", ExitCode: 2})
				continue
			}
			if err := connection.WriteJSON(terminalMessage{Type: "ack", Sequence: message.Sequence, Ack: message.Sequence, Cursor: sequence}); err != nil {
				return
			}
		default:
			_ = connection.WriteJSON(terminalMessage{Type: "error", Sequence: sequence, Cursor: sequence, Stderr: "unsupported message type\n", ExitCode: 2})
		}
	}
}

func limitOutput(value string) string {
	const limit = 64 << 10
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[output truncated by RedLab]\n"
}
func (s *Server) authorize(r *http.Request, team string) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return false
	}
	owner := s.tokenOwner(token)
	return owner != "" && (owner == team || owner == "organizer")
}

func (s *Server) issueTokens(owner string) (access, refresh string, err error) {
	if s.securityErr != nil || len(s.SigningKey) < 32 {
		if s.securityErr != nil {
			return "", "", s.securityErr
		}
		return "", "", errors.New("token signing key is unavailable")
	}
	now := time.Now().UTC()
	expires := now.Add(30 * time.Minute)
	access = s.signAccess(owner, expires)
	refresh, err = opaqueToken()
	if err != nil {
		return "", "", err
	}
	if s.Store != nil {
		if err := s.Store.SaveRefreshToken(hashToken(refresh), owner, now, now.Add(30*24*time.Hour)); err != nil {
			return "", "", err
		}
	}
	s.mu.Lock()
	s.Tokens[access] = owner
	s.TokenExpiry[access] = expires
	if s.Store == nil {
		s.RefreshTokens[hashToken(refresh)] = owner
	}
	s.mu.Unlock()
	return access, refresh, nil
}

func (s *Server) tokenOwner(token string) string {
	if token == "" {
		return ""
	}
	if owner, ok := s.verifyAccess(token); ok {
		return owner
	}
	if strings.Count(token, ".") == 2 {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	owner, ok := s.Tokens[token]
	if !ok {
		return ""
	}
	if expiry := s.TokenExpiry[token]; !expiry.IsZero() && time.Now().After(expiry) {
		delete(s.Tokens, token)
		delete(s.TokenExpiry, token)
		return ""
	}
	return owner
}

type accessClaims struct {
	Owner  string `json:"owner"`
	Expiry int64  `json:"exp"`
}

func (s *Server) signAccess(owner string, expires time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"RLAT"}`))
	claims, _ := json.Marshal(accessClaims{Owner: owner, Expiry: expires.Unix()})
	payload := base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, s.SigningKey)
	_, _ = mac.Write([]byte(header + "." + payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + payload + "." + signature
}

func (s *Server) verifyAccess(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(s.SigningKey) < 32 {
		return "", false
	}
	mac := hmac.New(sha256.New, s.SigningKey)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expected, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expected, mac.Sum(nil)) {
		return "", false
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims accessClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil || claims.Owner == "" || claims.Expiry <= time.Now().UTC().Unix() {
		return "", false
	}
	return claims.Owner, true
}
func (s *Server) sessionForTeam(team string) *runtime.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionForTeamLocked(team)
}
func (s *Server) sessionForTeamLocked(team string) *runtime.Session {
	for _, session := range s.Sessions {
		if session.TeamID == team {
			return session
		}
	}
	return nil
}
func splitPath(value string) []string { return strings.Split(strings.Trim(value, "/"), "/") }
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func (s *Server) limitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowRequest(r) {
			writeError(w, http.StatusTooManyRequests, errors.New("request rate limit exceeded"))
			return
		}
		limit := int64(2 << 20)
		if r.URL.Path == "/api/v1/organizer/scenarios/import" {
			limit = maxScenarioUpload + (1 << 20)
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowRequest(r *http.Request) bool {
	key := r.RemoteAddr
	if index := strings.LastIndexByte(key, ':'); index > 0 {
		key = key[:index]
	}
	now := time.Now()
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	window := s.rate[key]
	if window.Started.IsZero() || now.Sub(window.Started) >= time.Minute {
		window = rateWindow{Started: now}
	}
	window.Count++
	s.rate[key] = window
	return window.Count <= 120
}

func newToken(seed string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, time.Now().UnixNano())))
	return hex.EncodeToString(sum[:])
}
func opaqueToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func deterministicSeed(event, team, scenarioID string) time.Time {
	sum := sha256.Sum256([]byte(event + "\x00" + team + "\x00" + scenarioID))
	value := int64(0)
	for _, b := range sum[:8] {
		value = value<<8 | int64(b)
	}
	if value < 0 {
		value = -value
	}
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(value%86400) * time.Second)
}
func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

//go:embed dashboard.html
var dashboardHTML string

const legacyDashboardHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>RedLab</title><style>
body{font:16px system-ui,sans-serif;max-width:960px;margin:2rem auto;padding:0 1rem;background:#101820;color:#e8f0f2}main{display:grid;gap:1rem;grid-template-columns:repeat(auto-fit,minmax(260px,1fr))}.card{background:#182831;border:1px solid #2e4b56;border-radius:10px;padding:1rem}code{color:#9fe7c1}li{margin:.4rem 0}.muted{color:#a4bcc4}
</style></head><body><h1>RedLab</h1><p class="muted">Deterministic RHEL 8 incident-response lab</p><main>
<section class="card"><h2>Event</h2><p id="event">Loading…</p><p id="preflight" class="muted"></p></section>
<section class="card"><h2>Scenarios</h2><ul id="scenarios"><li>Loading…</li></ul></section>
<section class="card"><h2>Participant quick start</h2><p>Authenticate with a team join code, then create your session through <code>/api/v1/teams/&lt;team&gt;/session</code>.</p><p class="muted">The terminal uses WebSocket <code>/api/v1/sessions/&lt;id&gt;/terminal</code>.</p></section>
<section class="card"><h2>Organizer</h2><p>Organizer tokens can review <code>/api/v1/organizer/sessions</code>, close or reopen the event, export bundles, and record rubric scores.</p></section>
</main><script>
async function load(){
 const [event,status,scenarios,preflight]=await Promise.all([fetch('/api/v1/event'),fetch('/api/v1/event/status'),fetch('/api/v1/event/scenarios'),fetch('/api/v1/preflight')]);
 const e=await event.json(), s=await status.json(), sc=await scenarios.json(), p=await preflight.json();
 document.querySelector('#event').textContent=(e.metadata?.title||e.metadata?.id||'Event')+' — '+s.status;
 document.querySelector('#preflight').textContent='Preflight: '+(p.status||'unavailable');
 document.querySelector('#scenarios').innerHTML=sc.map(x=>'<li><strong>'+x.title+'</strong><br><span class="muted">'+x.id+' · '+x.version+'</span></li>').join('')||'<li>No scenarios</li>';
}
load().catch(err=>document.querySelector('#event').textContent='Dashboard unavailable: '+err);
</script></body></html>`
