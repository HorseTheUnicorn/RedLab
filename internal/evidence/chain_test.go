package evidence

import (
	"testing"
	"time"
)

func TestChainDetectsTampering(t *testing.T) {
	chain := Chain{}
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	chain.Append(Event{SessionID: "s", Timestamp: when, VirtualTimestamp: when, Type: "command", Actor: "trainee", Command: "id"})
	chain.Append(Event{SessionID: "s", Timestamp: when, VirtualTimestamp: when, Type: "command", Actor: "trainee", Command: "pwd"})
	if err := chain.Verify(); err != nil {
		t.Fatal(err)
	}
	chain.Events[1].Command = "cat /host-secret"
	if err := chain.Verify(); err == nil {
		t.Fatal("tampering was not detected")
	}
}

func TestManifestSignature(t *testing.T) {
	public, private, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{EventID: "event", SessionID: "session", Files: map[string]string{"report.md": "hash"}}
	signature, err := SignManifest(manifest, private)
	if err != nil {
		t.Fatal(err)
	}
	if signature.PublicKey != fmtHex(public) {
		t.Fatalf("unexpected public key")
	}
	if err := VerifyManifest(manifest, signature); err != nil {
		t.Fatal(err)
	}
	manifest.EventID = "altered"
	if err := VerifyManifest(manifest, signature); err == nil {
		t.Fatal("altered manifest verified")
	}
}

func fmtHex(value []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(value)*2)
	for i, b := range value {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&15]
	}
	return string(out)
}
