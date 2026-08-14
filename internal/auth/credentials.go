package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type Record struct {
	Algorithm string `json:"algorithm,omitempty"`
	Salt      string `json:"salt,omitempty"`
	Hash      string `json:"hash"`
}
type File struct {
	Organizer Record            `json:"organizer"`
	Link      Record            `json:"link"`
	Teams     map[string]Record `json:"teams"`
}

func GenerateCode() (string, error) {
	buffer := make([]byte, 10)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer), "=")[:12], nil
}

// GenerateToken returns a high-entropy URL-safe token for event linking.
func GenerateToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
func NewRecord(secret string) (Record, error) {
	if secret == "" {
		return Record{}, errors.New("credential secret cannot be empty")
	}
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return Record{}, err
	}
	return Record{Algorithm: "bcrypt", Hash: string(hashBytes)}, nil
}
func Verify(record Record, secret string) bool {
	if secret == "" || record.Hash == "" {
		return false
	}
	switch record.Algorithm {
	case "bcrypt":
		return bcrypt.CompareHashAndPassword([]byte(record.Hash), []byte(secret)) == nil
	case "", "sha256":
		if record.Salt == "" {
			return false
		}
		expected := hash(record.Salt, secret)
		return subtle.ConstantTimeCompare([]byte(record.Hash), []byte(expected)) == 1
	default:
		return false
	}
}

// NeedsUpgrade identifies legacy or lower-cost records that should be replaced
// after a successful authentication.
func NeedsUpgrade(record Record) bool {
	if record.Algorithm != "bcrypt" {
		return true
	}
	cost, err := bcrypt.Cost([]byte(record.Hash))
	return err != nil || cost < bcrypt.DefaultCost
}
func Load(filename string) (File, error) {
	// #nosec G304 -- callers provide the fixed event data credential path, not participant input.
	data, err := os.ReadFile(filename)
	if err != nil {
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, err
	}
	return file, nil
}
func Save(filename string, file File) error {
	if file.Teams == nil {
		file.Teams = map[string]Record{}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".credentials-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return err
	}
	return os.Chmod(filename, 0600)
}
func EnsureValid(file File) error {
	if err := validateRecord(file.Organizer); err != nil {
		return errors.New("organizer credential is missing")
	}
	if len(file.Teams) == 0 {
		return fmt.Errorf("no team credentials configured")
	}
	for teamID, record := range file.Teams {
		if strings.TrimSpace(teamID) == "" {
			return errors.New("team credential has an empty team id")
		}
		if err := validateRecord(record); err != nil {
			return fmt.Errorf("team credential %s is invalid: %w", teamID, err)
		}
	}
	if file.Link.Hash != "" {
		if err := validateRecord(file.Link); err != nil {
			return fmt.Errorf("event link credential is invalid: %w", err)
		}
	}
	return nil
}

func validateRecord(record Record) error {
	switch record.Algorithm {
	case "bcrypt":
		if _, err := bcrypt.Cost([]byte(record.Hash)); err != nil {
			return errors.New("malformed bcrypt hash")
		}
		return nil
	case "", "sha256":
		if len(record.Salt) != 32 || len(record.Hash) != sha256.Size*2 {
			return errors.New("malformed legacy credential hash")
		}
		if _, err := hex.DecodeString(record.Salt); err != nil {
			return errors.New("malformed legacy credential salt")
		}
		if _, err := hex.DecodeString(record.Hash); err != nil {
			return errors.New("malformed legacy credential hash")
		}
		return nil
	default:
		return fmt.Errorf("unsupported credential algorithm %q", record.Algorithm)
	}
}
func hash(salt, secret string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + secret))
	return hex.EncodeToString(sum[:])
}
