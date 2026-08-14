package store

import (
	"testing"
	"time"
)

func TestBackupCreatesIndependentSQLiteDatabase(t *testing.T) {
	root := t.TempDir()
	db, err := Open(root + "/event.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveSession("session-1", "TEAM-1", "scenario", []byte(`{}`), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	backup := root + "/backup/event.db"
	if err := db.Backup(backup); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	records, err := restored.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "session-1" {
		t.Fatalf("backup sessions = %+v", records)
	}
}
