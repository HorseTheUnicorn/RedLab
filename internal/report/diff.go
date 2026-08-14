package report

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/redlab/redlab/internal/system"
)

type StateDiff struct {
	Files    []FileChange     `json:"files,omitempty"`
	Services []ServiceChange  `json:"services,omitempty"`
	Users    []UserChange     `json:"users,omitempty"`
	Firewall []FirewallChange `json:"firewall,omitempty"`
	Packages []PackageChange  `json:"packages,omitempty"`
}

type FileSummary struct {
	Path        string `json:"path"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	Mode        uint32 `json:"mode"`
	SELinuxType string `json:"selinuxType,omitempty"`
	Directory   bool   `json:"directory"`
	Size        int    `json:"size"`
}
type FileChange struct {
	Path           string       `json:"path"`
	Change         string       `json:"change"`
	Before         *FileSummary `json:"before,omitempty"`
	After          *FileSummary `json:"after,omitempty"`
	ContentChanged bool         `json:"contentChanged,omitempty"`
}
type ServiceSummary struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
}
type ServiceChange struct {
	Name   string          `json:"name"`
	Change string          `json:"change"`
	Before *ServiceSummary `json:"before,omitempty"`
	After  *ServiceSummary `json:"after,omitempty"`
}
type UserSummary struct {
	Name   string   `json:"name"`
	UID    int      `json:"uid"`
	GID    int      `json:"gid"`
	Groups []string `json:"groups"`
	Shell  string   `json:"shell"`
}
type UserChange struct {
	Name   string       `json:"name"`
	Change string       `json:"change"`
	Before *UserSummary `json:"before,omitempty"`
	After  *UserSummary `json:"after,omitempty"`
}
type FirewallChange struct {
	Zone             string   `json:"zone"`
	BeforeInterfaces []string `json:"beforeInterfaces,omitempty"`
	AfterInterfaces  []string `json:"afterInterfaces,omitempty"`
	BeforeServices   []string `json:"beforeServices,omitempty"`
	AfterServices    []string `json:"afterServices,omitempty"`
	BeforePorts      []string `json:"beforePorts,omitempty"`
	AfterPorts       []string `json:"afterPorts,omitempty"`
}
type PackageChange struct {
	Name   string `json:"name"`
	Change string `json:"change"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

func Diff(before, after *system.State) StateDiff {
	if before == nil || after == nil {
		return StateDiff{}
	}
	b := before.Clone()
	a := after.Clone()
	diff := StateDiff{}
	fileNames := unionFiles(b.Files, a.Files)
	for _, name := range fileNames {
		oldFile, oldOK := b.Files[name]
		newFile, newOK := a.Files[name]
		if oldOK && newOK && reflect.DeepEqual(oldFile, newFile) {
			continue
		}
		change := "modified"
		if !oldOK {
			change = "added"
		} else if !newOK {
			change = "removed"
		}
		item := FileChange{Path: name, Change: change}
		if oldOK {
			item.Before = summarizeFile(oldFile)
		}
		if newOK {
			item.After = summarizeFile(newFile)
		}
		item.ContentChanged = oldOK && newOK && oldFile.Content != newFile.Content
		diff.Files = append(diff.Files, item)
	}

	serviceNames := unionServices(b.Services, a.Services)
	for _, name := range serviceNames {
		oldService, oldOK := b.Services[name]
		newService, newOK := a.Services[name]
		if oldOK && newOK && reflect.DeepEqual(oldService, newService) {
			continue
		}
		item := ServiceChange{Name: name, Change: changeType(oldOK, newOK)}
		if oldOK {
			item.Before = &ServiceSummary{Name: oldService.Name, Enabled: oldService.Enabled, State: oldService.State}
		}
		if newOK {
			item.After = &ServiceSummary{Name: newService.Name, Enabled: newService.Enabled, State: newService.State}
		}
		diff.Services = append(diff.Services, item)
	}

	userNames := unionUsers(b.Users, a.Users)
	for _, name := range userNames {
		oldUser, oldOK := b.Users[name]
		newUser, newOK := a.Users[name]
		if oldOK && newOK && userEqual(oldUser, newUser) {
			continue
		}
		item := UserChange{Name: name, Change: changeType(oldOK, newOK)}
		if oldOK {
			item.Before = summarizeUser(oldUser)
		}
		if newOK {
			item.After = summarizeUser(newUser)
		}
		diff.Users = append(diff.Users, item)
	}

	zoneNames := unionZones(b.Network.Zones, a.Network.Zones)
	for _, name := range zoneNames {
		oldZone, oldOK := b.Network.Zones[name]
		newZone, newOK := a.Network.Zones[name]
		oldInterfaces, oldServices, oldPorts := zoneValues(oldZone, oldOK)
		newInterfaces, newServices, newPorts := zoneValues(newZone, newOK)
		if reflect.DeepEqual(oldInterfaces, newInterfaces) && reflect.DeepEqual(oldServices, newServices) && reflect.DeepEqual(oldPorts, newPorts) {
			continue
		}
		diff.Firewall = append(diff.Firewall, FirewallChange{Zone: name, BeforeInterfaces: oldInterfaces, AfterInterfaces: newInterfaces, BeforeServices: oldServices, AfterServices: newServices, BeforePorts: oldPorts, AfterPorts: newPorts})
	}

	packageNames := unionPackages(b.Packages, a.Packages)
	for _, name := range packageNames {
		oldPackage, oldOK := b.Packages[name]
		newPackage, newOK := a.Packages[name]
		if oldOK && newOK && oldPackage.Version == newPackage.Version {
			continue
		}
		item := PackageChange{Name: name, Change: changeType(oldOK, newOK)}
		if oldOK {
			item.Before = oldPackage.Version
		}
		if newOK {
			item.After = newPackage.Version
		}
		diff.Packages = append(diff.Packages, item)
	}
	return diff
}

func (d StateDiff) Empty() bool {
	return len(d.Files) == 0 && len(d.Services) == 0 && len(d.Users) == 0 && len(d.Firewall) == 0 && len(d.Packages) == 0
}

func Patch(d StateDiff) string {
	var b strings.Builder
	b.WriteString("--- redlab/virtual-state-before\n+++ redlab/virtual-state-after\n")
	for _, item := range d.Files {
		fmt.Fprintf(&b, "file %s %s", item.Change, item.Path)
		if item.ContentChanged {
			b.WriteString(" content-changed")
		}
		b.WriteByte('\n')
	}
	for _, item := range d.Services {
		fmt.Fprintf(&b, "service %s %s\n", item.Change, item.Name)
	}
	for _, item := range d.Users {
		fmt.Fprintf(&b, "user %s %s\n", item.Change, item.Name)
	}
	for _, item := range d.Firewall {
		fmt.Fprintf(&b, "firewall modified zone=%s services=%s ports=%s\n", item.Zone, strings.Join(item.AfterServices, ","), strings.Join(item.AfterPorts, ","))
	}
	for _, item := range d.Packages {
		fmt.Fprintf(&b, "package %s %s\n", item.Change, item.Name)
	}
	return b.String()
}

func changeType(before, after bool) string {
	if !before {
		return "added"
	}
	if !after {
		return "removed"
	}
	return "modified"
}
func summarizeFile(file system.File) *FileSummary {
	return &FileSummary{Path: file.Path, Owner: file.Owner, Group: file.Group, Mode: file.Mode, SELinuxType: file.SELinuxType, Directory: file.Directory, Size: len(file.Content)}
}
func summarizeUser(user system.User) *UserSummary {
	groups := append([]string(nil), user.Groups...)
	sort.Strings(groups)
	return &UserSummary{Name: user.Name, UID: user.UID, GID: user.GID, Groups: groups, Shell: user.Shell}
}
func userEqual(a, b system.User) bool {
	return a.Name == b.Name && a.UID == b.UID && a.GID == b.GID && a.Shell == b.Shell && sameStrings(a.Groups, b.Groups)
}
func sameStrings(a, b []string) bool {
	left, right := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)
	return reflect.DeepEqual(left, right)
}
func zoneValues(zone system.FirewallZone, ok bool) ([]string, []string, []string) {
	if !ok {
		return nil, nil, nil
	}
	interfaces := append([]string(nil), zone.Interfaces...)
	sort.Strings(interfaces)
	return interfaces, mapKeys(zone.Services), mapKeys(zone.Ports)
}
func mapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, enabled := range values {
		if enabled {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
func unionFiles(before, after map[string]system.File) []string {
	return sortedUnion(fileKeys(before), fileKeys(after))
}
func unionServices(before, after map[string]system.Service) []string {
	return sortedUnion(stringKeysService(before), stringKeysService(after))
}
func unionUsers(before, after map[string]system.User) []string {
	return sortedUnion(stringKeysUser(before), stringKeysUser(after))
}
func unionZones(before, after map[string]system.FirewallZone) []string {
	return sortedUnion(stringKeysZone(before), stringKeysZone(after))
}
func unionPackages(before, after map[string]system.Package) []string {
	return sortedUnion(stringKeysPackage(before), stringKeysPackage(after))
}
func sortedUnion(left, right []string) []string {
	seen := map[string]bool{}
	for _, value := range append(left, right...) {
		seen[value] = true
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
func fileKeys(values map[string]system.File) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
func stringKeysService(values map[string]system.Service) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
func stringKeysUser(values map[string]system.User) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
func stringKeysZone(values map[string]system.FirewallZone) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
func stringKeysPackage(values map[string]system.Package) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
