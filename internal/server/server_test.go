package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/redlab/redlab/internal/auth"
	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/store"
)

func TestTeamSessionAPI(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{APIVersion: "redlab/v1", Kind: "Scenario", Metadata: scenario.DocumentMeta{ID: "test", Title: "Test", Version: "1.0.0"}, Spec: scenario.ScenarioSpec{RHEL: scenario.RHELSpec{Major: 8, Hostname: "test.example", SELinux: "enforcing"}, Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}, Shell: "/bin/bash"}}}, Scoring: scenario.ScoringSpec{AutomatedMaximum: 0}}}, Files: map[string][]byte{}}
	db, err := store.Open(t.TempDir() + "/event.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	eventFile := filepath.Join(t.TempDir(), "event.yaml")
	app := New(eventFile, scenario.Event{APIVersion: "redlab/v1", Kind: "Event", Metadata: scenario.DocumentMeta{ID: "event", Title: "Event"}}, map[string]scenario.Package{"test": pkg}, db)
	login := httptest.NewRecorder()
	app.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/team/login", bytes.NewBufferString(`{"teamID":"TEAM-1"}`)))
	if login.Code != 200 {
		t.Fatalf("login status %d: %s", login.Code, login.Body.String())
	}
	var token map[string]string
	if err := json.Unmarshal(login.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/teams/TEAM-1/session", nil)
	request.Header.Set("Authorization", "Bearer "+token["accessToken"])
	created := httptest.NewRecorder()
	app.Handler().ServeHTTP(created, request)
	if created.Code != 201 {
		t.Fatalf("create status %d: %s", created.Code, created.Body.String())
	}
	var session map[string]string
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	command := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+session["sessionID"]+"/command", bytes.NewBufferString(`{"command":"id"}`))
	command.Header.Set("Authorization", "Bearer "+token["accessToken"])
	output := httptest.NewRecorder()
	app.Handler().ServeHTTP(output, command)
	if output.Code != 200 || !bytes.Contains(output.Body.Bytes(), []byte("trainee")) {
		t.Fatalf("command response: %d %s", output.Code, output.Body.String())
	}
	submit := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+session["sessionID"]+"/submit", nil)
	submit.Header.Set("Authorization", "Bearer "+token["accessToken"])
	submitted := httptest.NewRecorder()
	app.Handler().ServeHTTP(submitted, submit)
	if submitted.Code != http.StatusOK {
		t.Fatalf("submit status %d: %s", submitted.Code, submitted.Body.String())
	}
	recovered := New(eventFile, scenario.Event{APIVersion: "redlab/v1", Kind: "Event", Metadata: scenario.DocumentMeta{ID: "event", Title: "Event"}}, map[string]scenario.Package{"test": pkg}, db)
	if err := recovered.Recover(); err != nil {
		t.Fatal(err)
	}
	if len(recovered.Sessions) != 1 {
		t.Fatalf("recovered sessions = %d", len(recovered.Sessions))
	}
	if !recovered.Sessions[session["sessionID"]].IsSubmitted() {
		t.Fatal("submitted state was not recovered")
	}
}

func TestOrganizerLifecycleAndDashboard(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{APIVersion: "redlab/v1", Kind: "Scenario", Metadata: scenario.DocumentMeta{ID: "test", Title: "Test", Version: "1.0.0"}, Spec: scenario.ScenarioSpec{RHEL: scenario.RHELSpec{Major: 8, Hostname: "test.example", SELinux: "enforcing"}, Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}, Shell: "/bin/bash"}}}, Scoring: scenario.ScoringSpec{AutomatedMaximum: 0, JudgeMaximum: 10}}}, Files: map[string][]byte{}}
	db, err := store.Open(t.TempDir() + "/event.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := New("event.yaml", scenario.Event{APIVersion: "redlab/v1", Kind: "Event", Metadata: scenario.DocumentMeta{ID: "event", Title: "Event"}, Spec: scenario.EventSpec{Sessions: scenario.SessionsSpec{AllowRestart: true, MaxRestarts: 1}}}, map[string]scenario.Package{"test": pkg}, db)
	app.Credentials.Organizer, err = auth.NewRecord("organizer-secret")
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	app.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/organizer/login", bytes.NewBufferString(`{"password":"organizer-secret"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("organizer login status %d: %s", login.Code, login.Body.String())
	}
	var organizer map[string]string
	if err := json.Unmarshal(login.Body.Bytes(), &organizer); err != nil {
		t.Fatal(err)
	}
	root := httptest.NewRecorder()
	app.Handler().ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusOK || !bytes.Contains(root.Body.Bytes(), []byte("RedLab")) {
		t.Fatalf("dashboard response: %d", root.Code)
	}
	status := httptest.NewRecorder()
	app.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/preflight", nil))
	if status.Code != http.StatusOK {
		t.Fatalf("preflight status %d: %s", status.Code, status.Body.String())
	}
	closeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/organizer/event/close", nil)
	closeRequest.Header.Set("Authorization", "Bearer "+organizer["accessToken"])
	closed := httptest.NewRecorder()
	app.Handler().ServeHTTP(closed, closeRequest)
	if closed.Code != http.StatusOK || !bytes.Contains(closed.Body.Bytes(), []byte("closed")) {
		t.Fatalf("close response: %d %s", closed.Code, closed.Body.String())
	}
}

func TestAPIRestartRefreshHintsNotesAndJudging(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{APIVersion: "redlab/v1", Kind: "Scenario", Metadata: scenario.DocumentMeta{ID: "test", Title: "Test", Version: "1.0.0"}, Spec: scenario.ScenarioSpec{RHEL: scenario.RHELSpec{Major: 8, Hostname: "test.example", SELinux: "enforcing"}, Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}, Shell: "/bin/bash"}}}, Hints: []scenario.HintSpec{{ID: "inspect", Text: "inspect logs"}}, Scoring: scenario.ScoringSpec{AutomatedMaximum: 0, JudgeMaximum: 10}}}, Files: map[string][]byte{}}
	db, err := store.Open(t.TempDir() + "/event.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := New("event.yaml", scenario.Event{APIVersion: "redlab/v1", Kind: "Event", Metadata: scenario.DocumentMeta{ID: "event", Title: "Event"}, Spec: scenario.EventSpec{Scoring: scenario.EventScoring{HintsEnabled: true}, Sessions: scenario.SessionsSpec{AllowRestart: true, MaxRestarts: 1}}}, map[string]scenario.Package{"test": pkg}, db)
	app.Credentials.Organizer, err = auth.NewRecord("organizer-secret")
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	app.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/team/login", bytes.NewBufferString(`{"teamID":"TEAM-1"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("team login: %d %s", login.Code, login.Body.String())
	}
	var team map[string]string
	if err := json.Unmarshal(login.Body.Bytes(), &team); err != nil {
		t.Fatal(err)
	}
	access := team["accessToken"]
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+access)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, req)
		return response
	}
	created := post("/api/v1/teams/TEAM-1/session", "")
	if created.Code != http.StatusCreated {
		t.Fatalf("create session: %d %s", created.Code, created.Body.String())
	}
	var session map[string]string
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	id := session["sessionID"]
	if response := post("/api/v1/sessions/"+id+"/notes", `{"text":"reviewed journal"}`); response.Code != http.StatusOK {
		t.Fatalf("note: %d %s", response.Code, response.Body.String())
	}
	if response := post("/api/v1/sessions/"+id+"/hints/inspect", ""); response.Code != http.StatusOK {
		t.Fatalf("hint: %d %s", response.Code, response.Body.String())
	}
	if response := post("/api/v1/sessions/"+id+"/restart", ""); response.Code != http.StatusOK {
		t.Fatalf("restart: %d %s", response.Code, response.Body.String())
	}
	refresh := httptest.NewRecorder()
	app.Handler().ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+team["refreshToken"]+`"}`)))
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", refresh.Code, refresh.Body.String())
	}
	rotated := httptest.NewRecorder()
	app.Handler().ServeHTTP(rotated, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+team["refreshToken"]+`"}`)))
	if rotated.Code != http.StatusUnauthorized {
		t.Fatalf("refresh rotation: %d %s", rotated.Code, rotated.Body.String())
	}
	orgLogin := httptest.NewRecorder()
	app.Handler().ServeHTTP(orgLogin, httptest.NewRequest(http.MethodPost, "/api/v1/auth/organizer/login", bytes.NewBufferString(`{"password":"organizer-secret"}`)))
	var organizer map[string]string
	if err := json.Unmarshal(orgLogin.Body.Bytes(), &organizer); err != nil {
		t.Fatal(err)
	}
	judging := httptest.NewRequest(http.MethodPost, "/api/v1/submissions/"+id+"/judging", bytes.NewBufferString(`{"score":7,"notes":"good evidence"}`))
	judging.Header.Set("Authorization", "Bearer "+organizer["accessToken"])
	judged := httptest.NewRecorder()
	app.Handler().ServeHTTP(judged, judging)
	if judged.Code != http.StatusOK || !bytes.Contains(judged.Body.Bytes(), []byte(`"judgeScore":7`)) {
		t.Fatalf("judging: %d %s", judged.Code, judged.Body.String())
	}
}

func TestSignedAccessAndPersistentRefresh(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(root + "/event.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pkg := scenario.Package{Scenario: scenario.Scenario{Metadata: scenario.DocumentMeta{ID: "test"}, Spec: scenario.ScenarioSpec{RHEL: scenario.RHELSpec{Major: 8, Hostname: "test.example"}, Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000}}}}}}
	event := scenario.Event{Metadata: scenario.DocumentMeta{ID: "event"}}
	app := New("event.yaml", event, map[string]scenario.Package{"test": pkg}, db)
	login := httptest.NewRecorder()
	app.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/team/login", bytes.NewBufferString(`{"teamID":"TEAM-1"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var first map[string]string
	if err := json.Unmarshal(login.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if strings.Count(first["accessToken"], ".") != 2 {
		t.Fatalf("access token is not signed: %q", first["accessToken"])
	}
	recovered := New("event.yaml", event, map[string]scenario.Package{"test": pkg}, db)
	refresh := httptest.NewRecorder()
	recovered.Handler().ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+first["refreshToken"]+`"}`)))
	if refresh.Code != http.StatusOK {
		t.Fatalf("persistent refresh: %d %s", refresh.Code, refresh.Body.String())
	}
	rotated := httptest.NewRecorder()
	recovered.Handler().ServeHTTP(rotated, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+first["refreshToken"]+`"}`)))
	if rotated.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token was reusable: %d", rotated.Code)
	}
}

func TestTeamRoleIsolation(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/event.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pkg := scenario.Package{Scenario: scenario.Scenario{Metadata: scenario.DocumentMeta{ID: "test"}, Spec: scenario.ScenarioSpec{RHEL: scenario.RHELSpec{Major: 8, Hostname: "test.example"}, Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000}}}}}}
	app := New("event.yaml", scenario.Event{Metadata: scenario.DocumentMeta{ID: "event"}}, map[string]scenario.Package{"test": pkg}, db)
	login := func(team string) string {
		record := httptest.NewRecorder()
		app.Handler().ServeHTTP(record, httptest.NewRequest(http.MethodPost, "/api/v1/auth/team/login", bytes.NewBufferString(`{"teamID":"`+team+`"}`)))
		var response map[string]string
		if err := json.Unmarshal(record.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response["accessToken"]
	}
	create := func(team, token string) string {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+team+"/session", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		record := httptest.NewRecorder()
		app.Handler().ServeHTTP(record, req)
		if record.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", team, record.Code, record.Body.String())
		}
		var response map[string]string
		if err := json.Unmarshal(record.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response["sessionID"]
	}
	teamOne := login("TEAM-1")
	teamTwo := login("TEAM-2")
	teamTwoSession := create("TEAM-2", teamTwo)
	_ = create("TEAM-1", teamOne)
	cross := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+teamTwoSession, nil)
	cross.Header.Set("Authorization", "Bearer "+teamOne)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, cross)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("cross-team session access: %d %s", response.Code, response.Body.String())
	}
	organizer := httptest.NewRequest(http.MethodGet, "/api/v1/organizer/sessions", nil)
	organizer.Header.Set("Authorization", "Bearer "+teamOne)
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, organizer)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("team reached organizer endpoint: %d", response.Code)
	}
}

func TestTerminalProtocolSupportsResumeCursor(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{Metadata: scenario.DocumentMeta{ID: "test"}, Spec: scenario.ScenarioSpec{RHEL: scenario.RHELSpec{Major: 8, Hostname: "test.example"}, Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000}}}}}}
	app := New("event.yaml", scenario.Event{Metadata: scenario.DocumentMeta{ID: "event"}}, map[string]scenario.Package{"test": pkg}, nil)
	httpServer := httptest.NewServer(app.Handler())
	defer httpServer.Close()
	client := httpServer.Client()
	login, err := client.Post(httpServer.URL+"/api/v1/auth/team/login", "application/json", bytes.NewBufferString(`{"teamID":"TEAM-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer login.Body.Close()
	var credentials map[string]string
	if err := json.NewDecoder(login.Body).Decode(&credentials); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/teams/TEAM-1/session", nil)
	request.Header.Set("Authorization", "Bearer "+credentials["accessToken"])
	created, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer created.Body.Close()
	var session map[string]string
	if err := json.NewDecoder(created.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/sessions/" + session["sessionID"] + "/terminal"
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Authorization": []string{"Bearer " + credentials["accessToken"]}})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteJSON(map[string]any{"type": "input", "sequence": 1, "data": "id"}); err != nil {
		t.Fatal(err)
	}
	var output terminalMessage
	if err := connection.ReadJSON(&output); err != nil {
		t.Fatal(err)
	}
	if output.Type != "output" || output.Sequence != 1 || output.Ack != 1 {
		t.Fatalf("output envelope = %+v", output)
	}
	_ = connection.Close()

	connection, _, err = websocket.DefaultDialer.Dial(wsURL, http.Header{"Authorization": []string{"Bearer " + credentials["accessToken"]}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.WriteJSON(map[string]any{"type": "resume", "cursor": 0}); err != nil {
		t.Fatal(err)
	}
	var replay terminalMessage
	if err := connection.ReadJSON(&replay); err != nil {
		t.Fatal(err)
	}
	if replay.Type != "replay" || replay.Sequence != 1 {
		t.Fatalf("replay envelope = %+v", replay)
	}
	var ack terminalMessage
	if err := connection.ReadJSON(&ack); err != nil {
		t.Fatal(err)
	}
	if ack.Type != "ack" || ack.Cursor != 1 {
		t.Fatalf("ack envelope = %+v", ack)
	}
}
