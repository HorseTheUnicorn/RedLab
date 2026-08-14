package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/redlab/redlab/internal/scenario"
)

func TestFortyConcurrentInteractiveSessions(t *testing.T) {
	pkg := scenario.Package{Scenario: scenario.Scenario{Metadata: scenario.DocumentMeta{ID: "load"}, Spec: scenario.ScenarioSpec{RHEL: scenario.RHELSpec{Major: 8, Hostname: "load.example", SELinux: "enforcing"}, Actors: scenario.ActorsSpec{InitialUser: "trainee", Users: []scenario.UserSpec{{Name: "trainee", UID: 1000, Groups: []string{"wheel"}}}}, Scoring: scenario.ScoringSpec{AutomatedMaximum: 1}}}}
	app := New("event.yaml", scenario.Event{Metadata: scenario.DocumentMeta{ID: "load-event"}, Spec: scenario.EventSpec{Sessions: scenario.SessionsSpec{MaxConcurrent: 40}}}, map[string]scenario.Package{"load": pkg}, nil)
	teamSecrets := make(map[string]string, 40)
	for i := 0; i < 40; i++ {
		teamSecrets[fmt.Sprintf("TEAM-%d", i+1)] = fmt.Sprintf("secret-%d", i+1)
	}
	configureTestCredentials(t, app, teamSecrets)
	started := time.Now()
	errorsCh := make(chan error, 40)
	var wait sync.WaitGroup
	for i := 0; i < 40; i++ {
		i := i
		wait.Add(1)
		go func() {
			defer wait.Done()
			remote := fmt.Sprintf("10.0.0.%d:1234", i+1)
			serve := func(method, path string, body []byte) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, bytes.NewReader(body))
				req.RemoteAddr = remote
				record := httptest.NewRecorder()
				app.Handler().ServeHTTP(record, req)
				return record
			}
			login := serve(http.MethodPost, "/api/v1/auth/team/login", []byte(fmt.Sprintf(`{"teamID":"TEAM-%d","joinCode":"secret-%d"}`, i+1, i+1)))
			if login.Code != http.StatusOK {
				errorsCh <- fmt.Errorf("team %d login: %d", i+1, login.Code)
				return
			}
			var token map[string]string
			if err := json.Unmarshal(login.Body.Bytes(), &token); err != nil {
				errorsCh <- err
				return
			}
			createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/teams/TEAM-"+fmt.Sprint(i+1)+"/session", nil)
			createRequest.RemoteAddr = remote
			createRequest.Header.Set("Authorization", "Bearer "+token["accessToken"])
			create := httptest.NewRecorder()
			app.Handler().ServeHTTP(create, createRequest)
			if create.Code != http.StatusCreated {
				errorsCh <- fmt.Errorf("team %d create: %d %s", i+1, create.Code, create.Body.String())
				return
			}
			var session map[string]string
			if err := json.Unmarshal(create.Body.Bytes(), &session); err != nil {
				errorsCh <- err
				return
			}
			for _, command := range []string{"id", "pwd", "lab check", "date +%s"} {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+session["sessionID"]+"/command", bytes.NewBufferString(`{"command":"`+command+`"}`))
				req.RemoteAddr = remote
				req.Header.Set("Authorization", "Bearer "+token["accessToken"])
				record := httptest.NewRecorder()
				app.Handler().ServeHTTP(record, req)
				if record.Code != http.StatusOK {
					errorsCh <- fmt.Errorf("team %d command %q: %d %s", i+1, command, record.Code, record.Body.String())
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatal(err)
	}
	// Keep wall-clock performance assertions out of race builds: race
	// instrumentation and intentionally expensive bcrypt checks vary heavily
	// with runner CPU capacity. The same concurrent workload still runs above,
	// so the race detector continues to inspect it in full.
	if elapsed := time.Since(started); !raceEnabled && elapsed > 5*time.Second {
		t.Fatalf("40-session interactive burst took %s; target is 5s", elapsed)
	}
	if len(app.Sessions) != 40 {
		t.Fatalf("sessions = %d, want 40", len(app.Sessions))
	}
}
