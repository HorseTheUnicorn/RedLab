package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRecordVerification(t *testing.T) {
	record, err := NewRecord("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(record, "secret") {
		t.Fatal("correct secret was rejected")
	}
	if Verify(record, "wrong") {
		t.Fatal("wrong secret was accepted")
	}
	if record.Algorithm != "bcrypt" || NeedsUpgrade(record) {
		t.Fatalf("new credential was not stored with current bcrypt settings: %#v", record)
	}
}

func TestLegacyRecordVerificationAndUpgradeSignal(t *testing.T) {
	record := Record{Salt: "00112233445566778899aabbccddeeff"}
	record.Hash = hash(record.Salt, "secret")
	if !Verify(record, "secret") {
		t.Fatal("legacy credential was rejected")
	}
	if !NeedsUpgrade(record) {
		t.Fatal("legacy credential was not marked for upgrade")
	}
}

func TestCredentialFileValidationAndAtomicSave(t *testing.T) {
	organizer, err := NewRecord("organizer-secret")
	if err != nil {
		t.Fatal(err)
	}
	team, err := NewRecord("team-secret")
	if err != nil {
		t.Fatal(err)
	}
	credentials := File{Organizer: organizer, Teams: map[string]Record{"TEAM-1": team}}
	if err := EnsureValid(credentials); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "nested", "credentials.json")
	if err := Save(filename, credentials); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(loaded.Organizer, "organizer-secret") || !Verify(loaded.Teams["TEAM-1"], "team-secret") {
		t.Fatal("saved credentials could not be verified")
	}
	if info, err := os.Stat(filename); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("credential permissions are too broad: %o", info.Mode().Perm())
	}
	loaded.Organizer = Record{Algorithm: "unknown", Hash: "x"}
	if err := EnsureValid(loaded); err == nil {
		t.Fatal("unsupported credential algorithm was accepted")
	}
}

func TestGeneratedCodeShape(t *testing.T) {
	code, err := GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 12 {
		t.Fatalf("code length = %d", len(code))
	}
}
