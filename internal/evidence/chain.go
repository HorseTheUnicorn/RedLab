package evidence

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Event struct {
	SessionID        string    `json:"sessionID"`
	Sequence         uint64    `json:"sequence"`
	Timestamp        time.Time `json:"timestamp"`
	VirtualTimestamp time.Time `json:"virtualTimestamp"`
	Type             string    `json:"type"`
	Actor            string    `json:"actor"`
	Command          string    `json:"command,omitempty"`
	ExitCode         int       `json:"exitCode,omitempty"`
	Mutations        []string  `json:"mutations,omitempty"`
	PreviousHash     string    `json:"previousHash"`
	Hash             string    `json:"hash"`
}

type Chain struct {
	mu       sync.Mutex
	Events   []Event `json:"events"`
	previous string
}

func (c *Chain) Append(event Event) Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	event.Sequence = uint64(len(c.Events) + 1)
	event.PreviousHash = c.previous
	payload, _ := json.Marshal(struct {
		SessionID            string    `json:"sessionID"`
		Sequence             uint64    `json:"sequence"`
		Timestamp            time.Time `json:"timestamp"`
		VirtualTimestamp     time.Time `json:"virtualTimestamp"`
		Type, Actor, Command string
		ExitCode             int
		Mutations            []string
		PreviousHash         string
	}{event.SessionID, event.Sequence, event.Timestamp, event.VirtualTimestamp, event.Type, event.Actor, event.Command, event.ExitCode, event.Mutations, event.PreviousHash})
	sum := sha256.Sum256(append([]byte(event.PreviousHash), payload...))
	event.Hash = hex.EncodeToString(sum[:])
	c.Events = append(c.Events, event)
	c.previous = event.Hash
	return event
}
func (c *Chain) Verify() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.verifyUnlockedWithHashes()
}

func (c *Chain) verifyUnlockedWithHashes() error {
	var previous string
	for i, event := range c.Events {
		if event.Sequence != uint64(i+1) || event.PreviousHash != previous {
			return fmt.Errorf("evidence sequence or predecessor mismatch at %d", i+1)
		}
		payload, _ := json.Marshal(struct {
			SessionID            string    `json:"sessionID"`
			Sequence             uint64    `json:"sequence"`
			Timestamp            time.Time `json:"timestamp"`
			VirtualTimestamp     time.Time `json:"virtualTimestamp"`
			Type, Actor, Command string
			ExitCode             int
			Mutations            []string
			PreviousHash         string
		}{event.SessionID, event.Sequence, event.Timestamp, event.VirtualTimestamp, event.Type, event.Actor, event.Command, event.ExitCode, event.Mutations, event.PreviousHash})
		sum := sha256.Sum256(append([]byte(previous), payload...))
		if hex.EncodeToString(sum[:]) != event.Hash {
			return fmt.Errorf("evidence hash mismatch at %d", i+1)
		}
		previous = event.Hash
	}
	return nil
}
func (c *Chain) Snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.Events...)
}

func (c *Chain) Restore(events []Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	copyEvents := append([]Event(nil), events...)
	c.Events = copyEvents
	c.previous = ""
	for _, event := range copyEvents {
		c.previous = event.Hash
	}
	return c.verifyUnlockedWithHashes()
}

type Manifest struct {
	EventID        string            `json:"eventID"`
	ScenarioID     string            `json:"scenarioID"`
	TeamID         string            `json:"teamID"`
	SessionID      string            `json:"sessionID"`
	ScenarioDigest string            `json:"scenarioDigest"`
	Files          map[string]string `json:"files"`
	ChainHead      string            `json:"chainHead"`
}
type Signature struct {
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
}

func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}
func SignManifest(manifest Manifest, private ed25519.PrivateKey) (Signature, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return Signature{}, err
	}
	return Signature{PublicKey: hex.EncodeToString(private.Public().(ed25519.PublicKey)), Signature: hex.EncodeToString(ed25519.Sign(private, data))}, nil
}
func VerifyManifest(manifest Manifest, signature Signature) error {
	publicBytes, err := hex.DecodeString(signature.PublicKey)
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(signature.Signature)
	if err != nil {
		return err
	}
	if len(publicBytes) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return errors.New("invalid Ed25519 signature size")
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicBytes), data, sig) {
		return errors.New("manifest signature verification failed")
	}
	return nil
}
func Redact(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}
