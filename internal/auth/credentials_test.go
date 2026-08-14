package auth

import "testing"

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
