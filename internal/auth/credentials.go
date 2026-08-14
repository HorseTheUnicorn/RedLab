package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Record struct {
	Salt string `json:"salt"`
	Hash string `json:"hash"`
}
type File struct {
	Organizer Record            `json:"organizer"`
	Teams     map[string]Record `json:"teams"`
}

func GenerateCode() (string, error) {
	buffer := make([]byte, 10)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer), "=")[:12], nil
}
func NewRecord(secret string) (Record, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return Record{}, err
	}
	salt := hex.EncodeToString(saltBytes)
	return Record{Salt: salt, Hash: hash(salt, secret)}, nil
}
func Verify(record Record, secret string) bool {
	return record.Salt != "" && record.Hash == hash(record.Salt, secret)
}
func Load(filename string) (File, error) {
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
	if err := os.WriteFile(filename, data, 0600); err != nil {
		return err
	}
	return nil
}
func EnsureValid(file File) error {
	if file.Organizer.Hash == "" {
		return errors.New("organizer credential is missing")
	}
	if len(file.Teams) == 0 {
		return fmt.Errorf("no team credentials configured")
	}
	return nil
}
func hash(salt, secret string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + secret))
	return hex.EncodeToString(sum[:])
}
