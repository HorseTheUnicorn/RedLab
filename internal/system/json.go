package system

import (
	"encoding/json"

	"github.com/redlab/redlab/internal/scenario"
)

func marshalState(s *State) ([]byte, error) {
	return json.Marshal(struct {
		Hostname    string              `json:"hostname"`
		Clock       string              `json:"clock"`
		Files       map[string]File     `json:"files"`
		Users       map[string]User     `json:"users"`
		SudoRules   []scenario.SudoRule `json:"sudoRules"`
		Services    map[string]Service  `json:"services"`
		Journal     []JournalEntry      `json:"journal"`
		Network     Network             `json:"network"`
		SELinux     SELinux             `json:"selinux"`
		Packages    map[string]Package  `json:"packages"`
		Env         map[string]string   `json:"env"`
		CurrentUser string              `json:"currentUser"`
		CWD         string              `json:"cwd"`
	}{s.Hostname, s.Clock.UTC().Format("2006-01-02T15:04:05Z"), s.Files, s.Users, s.SudoRules, s.Services, s.Journal, s.Network, s.SELinux, s.Packages, s.Env, s.CurrentUser, s.CWD})
}
