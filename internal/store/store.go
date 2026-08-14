package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/redlab/redlab/internal/evidence"
)

type Store struct{ DB *sql.DB }
type SessionRecord struct {
	ID          string
	TeamID      string
	ScenarioID  string
	CreatedAt   time.Time
	SubmittedAt time.Time
}
type SessionMeta struct {
	SessionID    string
	Restarts     int
	JudgeScore   int
	JudgeNotes   string
	Judged       bool
	Submitted    bool
	LastActivity time.Time
}

func Open(filename string) (*Store, error) {
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{"PRAGMA foreign_keys = ON", "PRAGMA journal_mode = WAL", "PRAGMA busy_timeout = 5000"} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	store := &Store{DB: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}
func (s *Store) migrate() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, team_id TEXT NOT NULL, scenario_id TEXT NOT NULL, created_at TEXT NOT NULL, submitted_at TEXT, state_json BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS evidence_events (session_id TEXT NOT NULL, sequence INTEGER NOT NULL, hash TEXT NOT NULL, event_json BLOB NOT NULL, PRIMARY KEY(session_id, sequence), FOREIGN KEY(session_id) REFERENCES sessions(id));
CREATE TABLE IF NOT EXISTS submissions (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, created_at TEXT NOT NULL, report_json BLOB NOT NULL, report_md TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id));
CREATE TABLE IF NOT EXISTS session_meta (session_id TEXT PRIMARY KEY, restarts INTEGER NOT NULL DEFAULT 0, judge_score INTEGER NOT NULL DEFAULT 0, judge_notes TEXT NOT NULL DEFAULT '', judged INTEGER NOT NULL DEFAULT 0, submitted INTEGER NOT NULL DEFAULT 0, last_activity TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id));
CREATE TABLE IF NOT EXISTS event_state (id INTEGER PRIMARY KEY CHECK(id=1), manual_closed INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS auth_signing (id INTEGER PRIMARY KEY CHECK(id=1), secret BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS auth_refresh_tokens (token_hash TEXT PRIMARY KEY, owner TEXT NOT NULL, issued_at TEXT NOT NULL, expires_at TEXT NOT NULL, used_at TEXT);`)
	return err
}

func (s *Store) SigningKey() ([]byte, error) {
	var secret []byte
	err := s.DB.QueryRow(`SELECT secret FROM auth_signing WHERE id=1`).Scan(&secret)
	if err == nil {
		return append([]byte(nil), secret...), nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	secret = make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`INSERT INTO auth_signing(id,secret) VALUES(1,?)`, secret); err != nil {
		var existing []byte
		if scanErr := s.DB.QueryRow(`SELECT secret FROM auth_signing WHERE id=1`).Scan(&existing); scanErr != nil {
			return nil, err
		}
		return existing, nil
	}
	return append([]byte(nil), secret...), nil
}

func (s *Store) SaveRefreshToken(tokenHash, owner string, issuedAt, expiresAt time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO auth_refresh_tokens(token_hash,owner,issued_at,expires_at) VALUES(?,?,?,?)`, tokenHash, owner, issuedAt.UTC().Format(time.RFC3339Nano), expiresAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) RotateRefreshToken(tokenHash string, now time.Time) (string, bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	var owner, expiresAt string
	var usedAt sql.NullString
	if err := tx.QueryRow(`SELECT owner,expires_at,used_at FROM auth_refresh_tokens WHERE token_hash=?`, tokenHash).Scan(&owner, &expiresAt, &usedAt); err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || usedAt.Valid || !now.Before(expires) {
		return "", false, nil
	}
	result, err := tx.Exec(`UPDATE auth_refresh_tokens SET used_at=? WHERE token_hash=? AND used_at IS NULL`, now.UTC().Format(time.RFC3339Nano), tokenHash)
	if err != nil {
		return "", false, err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return owner, true, nil
}
func (s *Store) SaveSession(id, team, scenarioID string, state []byte, created time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO sessions(id,team_id,scenario_id,created_at,state_json) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state_json=excluded.state_json,submitted_at=COALESCE(sessions.submitted_at,excluded.submitted_at)`, id, team, scenarioID, created.UTC().Format(time.RFC3339Nano), state)
	return err
}
func (s *Store) SaveEvidence(event evidence.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT OR REPLACE INTO evidence_events(session_id,sequence,hash,event_json) VALUES(?,?,?,?)`, event.SessionID, event.Sequence, event.Hash, data)
	return err
}

func (s *Store) ClearEvidence(sessionID string) error {
	_, err := s.DB.Exec(`DELETE FROM evidence_events WHERE session_id=?`, sessionID)
	return err
}

func (s *Store) MarkSubmitted(sessionID string, submitted time.Time) error {
	_, err := s.DB.Exec(`UPDATE sessions SET submitted_at=? WHERE id=?`, submitted.UTC().Format(time.RFC3339Nano), sessionID)
	return err
}

func (s *Store) SaveSessionMeta(meta SessionMeta) error {
	_, err := s.DB.Exec(`INSERT INTO session_meta(session_id,restarts,judge_score,judge_notes,judged,submitted,last_activity) VALUES(?,?,?,?,?,?,?) ON CONFLICT(session_id) DO UPDATE SET restarts=excluded.restarts,judge_score=excluded.judge_score,judge_notes=excluded.judge_notes,judged=excluded.judged,submitted=excluded.submitted,last_activity=excluded.last_activity`, meta.SessionID, meta.Restarts, meta.JudgeScore, meta.JudgeNotes, boolInt(meta.Judged), boolInt(meta.Submitted), meta.LastActivity.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListSessionMeta() (map[string]SessionMeta, error) {
	rows, err := s.DB.Query(`SELECT session_id,restarts,judge_score,judge_notes,judged,submitted,last_activity FROM session_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]SessionMeta{}
	for rows.Next() {
		var meta SessionMeta
		var judged, submitted int
		var activity string
		if err := rows.Scan(&meta.SessionID, &meta.Restarts, &meta.JudgeScore, &meta.JudgeNotes, &judged, &submitted, &activity); err != nil {
			return nil, err
		}
		meta.Judged = judged != 0
		meta.Submitted = submitted != 0
		meta.LastActivity, _ = time.Parse(time.RFC3339Nano, activity)
		result[meta.SessionID] = meta
	}
	return result, rows.Err()
}

func (s *Store) SetManualClosed(closed bool) error {
	_, err := s.DB.Exec(`INSERT INTO event_state(id,manual_closed) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET manual_closed=excluded.manual_closed`, boolInt(closed))
	return err
}

func (s *Store) ManualClosed() (bool, error) {
	var closed int
	err := s.DB.QueryRow(`SELECT manual_closed FROM event_state WHERE id=1`).Scan(&closed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return closed != 0, err
}
func (s *Store) SaveSubmission(id, sessionID string, reportJSON []byte, reportMD string, created time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO submissions(id,session_id,created_at,report_json,report_md) VALUES(?,?,?,?,?)`, id, sessionID, created.UTC().Format(time.RFC3339Nano), reportJSON, reportMD)
	return err
}
func (s *Store) LoadEvents(sessionID string) ([]evidence.Event, error) {
	rows, err := s.DB.Query(`SELECT event_json FROM evidence_events WHERE session_id=? ORDER BY sequence`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []evidence.Event
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var event evidence.Event
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) ListSessions() ([]SessionRecord, error) {
	rows, err := s.DB.Query(`SELECT id, team_id, scenario_id, created_at, submitted_at FROM sessions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SessionRecord
	for rows.Next() {
		var record SessionRecord
		var created string
		var submitted sql.NullString
		if err := rows.Scan(&record.ID, &record.TeamID, &record.ScenarioID, &created, &submitted); err != nil {
			return nil, err
		}
		record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if submitted.Valid {
			record.SubmittedAt, _ = time.Parse(time.RFC3339Nano, submitted.String)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
func (s *Store) Health() error {
	var one int
	if err := s.DB.QueryRow(`SELECT 1`).Scan(&one); err != nil {
		return err
	}
	if one != 1 {
		return fmt.Errorf("unexpected health response")
	}
	return nil
}

func (s *Store) Backup(filename string) error {
	if filename == "" {
		return fmt.Errorf("backup filename is required")
	}
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("backup destination already exists: %s", filename)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	_, err := s.DB.Exec(`VACUUM INTO ?`, filename)
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
