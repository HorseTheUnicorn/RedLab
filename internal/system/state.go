package system

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redlab/redlab/internal/scenario"
)

const (
	ModeOwnerRead  = 0400
	ModeOwnerWrite = 0200
	ModeOwnerExec  = 0100
	ModeGroupRead  = 0040
	ModeGroupWrite = 0020
	ModeGroupExec  = 0010
	ModeOtherRead  = 0004
	ModeOtherWrite = 0002
	ModeOtherExec  = 0001
)

type File struct {
	Path        string `json:"path"`
	Content     string `json:"content,omitempty"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	Mode        uint32 `json:"mode"`
	SELinuxType string `json:"selinuxType,omitempty"`
	Directory   bool   `json:"directory"`
}

type User struct {
	Name     string   `json:"name"`
	UID      int      `json:"uid"`
	GID      int      `json:"gid"`
	Groups   []string `json:"groups"`
	Shell    string   `json:"shell"`
	Password string   `json:"-"`
}

type Service struct {
	Name            string                   `json:"name"`
	Enabled         bool                     `json:"enabled"`
	State           string                   `json:"state"`
	StartConditions []scenario.ConditionSpec `json:"startConditions"`
	OnFailure       scenario.FailureSpec     `json:"onFailure"`
}

type JournalEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Priority  string    `json:"priority"`
	Unit      string    `json:"unit,omitempty"`
	Message   string    `json:"message"`
	Actor     string    `json:"actor,omitempty"`
}

type FirewallZone struct {
	Interfaces []string        `json:"interfaces"`
	Services   map[string]bool `json:"services"`
	Ports      map[string]bool `json:"ports"`
}

type Network struct {
	Interfaces  []scenario.InterfaceSpec `json:"interfaces"`
	DNS         scenario.DNSSpec         `json:"dns"`
	DefaultZone string                   `json:"defaultZone"`
	Zones       map[string]FirewallZone  `json:"zones"`
	Hosts       []scenario.SimHostSpec   `json:"hosts"`
	OpenPorts   map[string]bool          `json:"openPorts"`
	HTTP        map[string]HTTPResponse  `json:"http"`
}

type HTTPResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}
type SELinux struct {
	Mode     string          `json:"mode"`
	Booleans map[string]bool `json:"booleans"`
}
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type State struct {
	mu          sync.RWMutex
	Hostname    string              `json:"hostname"`
	Clock       time.Time           `json:"clock"`
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
	Initial     *State              `json:"-"`
}

func NewState(pkg scenario.Package, seed time.Time) (*State, error) {
	s := &State{
		Hostname: pkg.Scenario.Spec.RHEL.Hostname,
		Clock:    seed.UTC(), Files: map[string]File{"/": {Path: "/", Owner: "root", Group: "root", Mode: 0755, Directory: true}},
		Users: map[string]User{}, Services: map[string]Service{}, Packages: map[string]Package{}, Env: map[string]string{}, CWD: "/",
		Network: Network{DNS: pkg.Scenario.Spec.Network.DNS, DefaultZone: pkg.Scenario.Spec.Network.Firewall.DefaultZone, Zones: map[string]FirewallZone{}, OpenPorts: map[string]bool{}, HTTP: map[string]HTTPResponse{}},
		SELinux: SELinux{Mode: pkg.Scenario.Spec.RHEL.SELinux, Booleans: map[string]bool{}},
	}
	s.SudoRules = append([]scenario.SudoRule(nil), pkg.Scenario.Spec.Actors.Sudo...)
	s.Network.Interfaces = append([]scenario.InterfaceSpec(nil), pkg.Scenario.Spec.Network.Interfaces...)
	if s.SELinux.Mode == "" {
		s.SELinux.Mode = "enforcing"
	}
	for _, user := range pkg.Scenario.Spec.Actors.Users {
		gid := user.GID
		if gid == 0 {
			gid = user.UID
		}
		s.Users[user.Name] = User{Name: user.Name, UID: user.UID, GID: gid, Groups: append([]string(nil), user.Groups...), Shell: user.Shell, Password: user.Password}
	}
	if _, ok := s.Users["root"]; !ok {
		s.Users["root"] = User{Name: "root", UID: 0, GID: 0, Groups: []string{"root"}, Shell: "/bin/bash"}
	}
	if _, ok := s.Users[pkg.Scenario.Spec.Actors.InitialUser]; !ok && pkg.Scenario.Spec.Actors.InitialUser != "" {
		return nil, fmt.Errorf("initial user %q is not defined", pkg.Scenario.Spec.Actors.InitialUser)
	}
	s.CurrentUser = pkg.Scenario.Spec.Actors.InitialUser
	if s.CurrentUser == "" {
		s.CurrentUser = "root"
	}
	for _, file := range pkg.Scenario.Spec.Filesystem.Entries {
		p, err := NormalizePath(file.Path, "/")
		if err != nil {
			return nil, err
		}
		mode, err := ParseMode(file.Mode, 0644)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		s.Files[p] = File{Path: p, Content: file.Append, Owner: defaultString(file.Owner, "root"), Group: defaultString(file.Group, "root"), Mode: mode, SELinuxType: file.SELinuxType}
	}
	for name, data := range pkg.Files {
		if !strings.HasPrefix(name, "files/") {
			continue
		}
		virtual := "/" + strings.TrimPrefix(name, "files/")
		if virtual == "/" {
			continue
		}
		if _, exists := s.Files[virtual]; exists {
			continue
		}
		s.ensureParents(virtual)
		s.Files[virtual] = File{Path: virtual, Content: string(data), Owner: "root", Group: "root", Mode: 0644}
	}
	for _, service := range pkg.Scenario.Spec.Services {
		s.Services[service.Name] = Service{Name: service.Name, Enabled: service.Enabled, State: service.State, StartConditions: service.StartConditions, OnFailure: service.OnFailure}
	}
	for _, p := range pkg.Scenario.Spec.Packages.Installed {
		s.Packages[p.Name] = Package{Name: p.Name, Version: p.Version}
	}
	for name, zone := range pkg.Scenario.Spec.Network.Firewall.Zones {
		s.Network.Zones[name] = FirewallZone{Interfaces: append([]string(nil), zone.Interfaces...), Services: sliceSet(zone.Services), Ports: sliceSet(zone.Ports)}
	}
	for _, host := range pkg.Scenario.Spec.Network.SimulatedHosts {
		s.Network.Hosts = append(s.Network.Hosts, host)
	}
	for _, rule := range pkg.Scenario.Spec.Rules {
		_ = rule
	}
	for _, service := range s.Services {
		if service.State == "running" {
			s.refreshServiceEffects(service.Name)
		}
	}
	for _, journal := range pkg.Scenario.Spec.Services {
		for _, item := range journal.OnFailure.Journal {
			s.appendJournalLocked("", journal.Name, item.Priority, item.Message)
		}
	}
	s.Initial = s.cloneUnlocked()
	return s, nil
}

func (s *State) Clone() *State { s.mu.RLock(); defer s.mu.RUnlock(); return s.cloneUnlocked() }

func (s *State) cloneUnlocked() *State {
	copyState := &State{
		Hostname: s.Hostname, Clock: s.Clock, Files: nil, Users: nil,
		SudoRules: append([]scenario.SudoRule(nil), s.SudoRules...),
		Services:  nil, Journal: nil, Network: s.Network, SELinux: s.SELinux,
		Packages: nil, Env: nil, CurrentUser: s.CurrentUser, CWD: s.CWD,
	}
	copyState.Files = map[string]File{}
	for k, v := range s.Files {
		copyState.Files[k] = v
	}
	copyState.Users = map[string]User{}
	for k, v := range s.Users {
		v.Groups = append([]string(nil), v.Groups...)
		copyState.Users[k] = v
	}
	copyState.Services = map[string]Service{}
	for k, v := range s.Services {
		v.StartConditions = append([]scenario.ConditionSpec(nil), v.StartConditions...)
		copyState.Services[k] = v
	}
	copyState.Journal = append([]JournalEntry(nil), s.Journal...)
	copyState.Packages = map[string]Package{}
	for k, v := range s.Packages {
		copyState.Packages[k] = v
	}
	copyState.Env = map[string]string{}
	for k, v := range s.Env {
		copyState.Env[k] = v
	}
	copyState.Network.Interfaces = append([]scenario.InterfaceSpec(nil), s.Network.Interfaces...)
	copyState.Network.DNS.Records = map[string]string{}
	for k, v := range s.Network.DNS.Records {
		copyState.Network.DNS.Records[k] = v
	}
	copyState.Network.DNS.Servers = append([]string(nil), s.Network.DNS.Servers...)
	copyState.Network.Zones = map[string]FirewallZone{}
	for k, v := range s.Network.Zones {
		v.Interfaces = append([]string(nil), v.Interfaces...)
		v.Services = map[string]bool{}
		for x, y := range s.Network.Zones[k].Services {
			v.Services[x] = y
		}
		v.Ports = map[string]bool{}
		for x, y := range s.Network.Zones[k].Ports {
			v.Ports[x] = y
		}
		copyState.Network.Zones[k] = v
	}
	copyState.Network.Hosts = append([]scenario.SimHostSpec(nil), s.Network.Hosts...)
	copyState.Network.OpenPorts = map[string]bool{}
	for k, v := range s.Network.OpenPorts {
		copyState.Network.OpenPorts[k] = v
	}
	copyState.Network.HTTP = map[string]HTTPResponse{}
	for k, v := range s.Network.HTTP {
		copyState.Network.HTTP[k] = v
	}
	copyState.SELinux.Booleans = map[string]bool{}
	for k, v := range s.SELinux.Booleans {
		copyState.SELinux.Booleans[k] = v
	}
	copyState.Initial = nil
	return copyState
}

func (s *State) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Initial == nil {
		return errors.New("initial snapshot is unavailable")
	}
	initial := s.Initial.cloneUnlocked()
	initial.Initial = s.Initial.cloneUnlocked()
	s.Hostname = initial.Hostname
	s.Clock = initial.Clock
	s.Files = initial.Files
	s.Users = initial.Users
	s.SudoRules = initial.SudoRules
	s.Services = initial.Services
	s.Journal = initial.Journal
	s.Network = initial.Network
	s.SELinux = initial.SELinux
	s.Packages = initial.Packages
	s.Env = initial.Env
	s.CurrentUser = initial.CurrentUser
	s.CWD = initial.CWD
	s.Initial = initial.Initial
	return nil
}

func (s *State) ReadFile(filename, user string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return "", err
	}
	file, ok := s.Files[p]
	if !ok || file.Directory {
		return "", fmt.Errorf("%s: No such file or directory", filename)
	}
	if !s.canReadLocked(file, user) {
		return "", fmt.Errorf("%s: Permission denied", filename)
	}
	return file.Content, nil
}
func (s *State) WriteFile(filename, content, user string, appendMode bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return err
	}
	if existing, ok := s.Files[p]; ok {
		if !s.canWriteLocked(existing, user) {
			return fmt.Errorf("%s: Permission denied", filename)
		}
		if appendMode {
			content = existing.Content + content
		}
	} else {
		s.ensureParents(p)
		if user != "root" && !s.canWriteLocked(s.Files[path.Dir(p)], user) {
			return fmt.Errorf("%s: Permission denied", filename)
		}
		s.Files[p] = File{Path: p, Owner: user, Group: s.primaryGroupLocked(user), Mode: 0644}
	}
	file := s.Files[p]
	file.Content = content
	s.Files[p] = file
	return nil
}
func (s *State) Stat(filename string) (File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return File{}, err
	}
	file, ok := s.Files[p]
	if !ok {
		return File{}, fmt.Errorf("%s: No such file or directory", filename)
	}
	return file, nil
}
func (s *State) List(filename, user string) ([]File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return nil, err
	}
	target, ok := s.Files[p]
	if !ok {
		return nil, fmt.Errorf("%s: No such file or directory", filename)
	}
	if !target.Directory {
		return []File{target}, nil
	}
	var out []File
	prefix := strings.TrimSuffix(p, "/") + "/"
	for name, file := range s.Files {
		if name != p && strings.HasPrefix(name, prefix) && !strings.Contains(strings.TrimPrefix(name, prefix), "/") {
			out = append(out, file)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
func (s *State) MakeDir(filename, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return err
	}
	if _, ok := s.Files[p]; ok {
		return fmt.Errorf("%s: File exists", filename)
	}
	parent := s.Files[path.Dir(p)]
	if !s.canWriteLocked(parent, user) {
		return fmt.Errorf("%s: Permission denied", filename)
	}
	s.Files[p] = File{Path: p, Owner: user, Group: s.primaryGroupLocked(user), Mode: 0755, Directory: true}
	return nil
}
func (s *State) Touch(filename, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return err
	}
	if _, ok := s.Files[p]; ok {
		return nil
	}
	s.ensureParents(p)
	parent := s.Files[path.Dir(p)]
	if !s.canWriteLocked(parent, user) {
		return fmt.Errorf("%s: Permission denied", filename)
	}
	s.Files[p] = File{Path: p, Owner: user, Group: s.primaryGroupLocked(user), Mode: 0644}
	return nil
}
func (s *State) Remove(filename, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return err
	}
	file, ok := s.Files[p]
	if !ok {
		return fmt.Errorf("%s: No such file or directory", filename)
	}
	if !s.canWriteLocked(s.Files[path.Dir(p)], user) {
		return fmt.Errorf("%s: Permission denied", filename)
	}
	if file.Directory {
		prefix := strings.TrimSuffix(p, "/") + "/"
		for name := range s.Files {
			if strings.HasPrefix(name, prefix) {
				delete(s.Files, name)
			}
		}
	}
	delete(s.Files, p)
	return nil
}

func (s *State) Copy(source, destination, user string, move bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, err := NormalizePath(source, s.CWD)
	if err != nil {
		return err
	}
	dst, err := NormalizePath(destination, s.CWD)
	if err != nil {
		return err
	}
	file, ok := s.Files[src]
	if !ok {
		return fmt.Errorf("%s: No such file or directory", source)
	}
	if file.Directory {
		return fmt.Errorf("%s: omitting directory", source)
	}
	if !s.canReadLocked(file, user) {
		return fmt.Errorf("%s: Permission denied", source)
	}
	s.ensureParents(dst)
	parent := s.Files[path.Dir(dst)]
	if user != "root" && !s.canWriteLocked(parent, user) {
		return fmt.Errorf("%s: Permission denied", destination)
	}
	if existing, exists := s.Files[dst]; exists && user != "root" && !s.canWriteLocked(existing, user) {
		return fmt.Errorf("%s: Permission denied", destination)
	}
	file.Path = dst
	s.Files[dst] = file
	if move {
		delete(s.Files, src)
	}
	return nil
}

func (s *State) Walk(root, user string) ([]File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, err := NormalizePath(root, s.CWD)
	if err != nil {
		return nil, err
	}
	if _, ok := s.Files[p]; !ok {
		return nil, fmt.Errorf("%s: No such file or directory", root)
	}
	prefix := strings.TrimSuffix(p, "/") + "/"
	out := make([]File, 0)
	for name, file := range s.Files {
		if name == p || strings.HasPrefix(name, prefix) {
			out = append(out, file)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *State) Glob(pattern, user string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	clean, err := NormalizePath(pattern, s.CWD)
	if err != nil {
		return nil
	}
	matches := make([]string, 0)
	for name, file := range s.Files {
		if !s.canReadLocked(file, user) && !file.Directory {
			continue
		}
		matched, matchErr := path.Match(clean, name)
		if matchErr == nil && matched {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	return matches
}

func (s *State) PackagesSnapshot() []Package {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Package, 0, len(s.Packages))
	for _, pkg := range s.Packages {
		out = append(out, pkg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *State) ServicesSnapshot() []Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Service, 0, len(s.Services))
	for _, service := range s.Services {
		out = append(out, service)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *State) VirtualUsage() (bytes, inodes int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, file := range s.Files {
		inodes++
		bytes += len(file.Content)
	}
	return bytes, inodes
}
func (s *State) Chmod(filename, mode, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return err
	}
	file, ok := s.Files[p]
	if !ok {
		return fmt.Errorf("%s: No such file or directory", filename)
	}
	if user != "root" && file.Owner != user {
		return fmt.Errorf("%s: Operation not permitted", filename)
	}
	parsed, err := ParseMode(mode, file.Mode)
	if err != nil {
		return err
	}
	file.Mode = parsed
	s.Files[p] = file
	return nil
}
func (s *State) Chown(filename, owner, group, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user != "root" {
		return fmt.Errorf("%s: Operation not permitted", filename)
	}
	p, err := NormalizePath(filename, s.CWD)
	if err != nil {
		return err
	}
	file, ok := s.Files[p]
	if !ok {
		return fmt.Errorf("%s: No such file or directory", filename)
	}
	file.Owner = owner
	if group != "" {
		file.Group = group
	}
	s.Files[p] = file
	return nil
}

func (s *State) StartService(name, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user != "root" {
		return errors.New("Authentication is required to manage system services")
	}
	service, ok := s.Services[name]
	if !ok {
		return fmt.Errorf("Unit %s.service could not be found", name)
	}
	for _, condition := range service.StartConditions {
		if !s.conditionLocked(condition) {
			service.State = "failed"
			s.Services[name] = service
			for _, j := range service.OnFailure.Journal {
				s.appendJournalLocked(user, name, j.Priority, j.Message)
			}
			return fmt.Errorf("Job for %s failed because a start condition was not met", name)
		}
	}
	service.State = "running"
	s.Services[name] = service
	s.refreshServiceEffectsLocked(name)
	s.appendJournalLocked(user, name, "info", "Started "+name)
	return nil
}
func (s *State) StopService(name, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user != "root" {
		return errors.New("Authentication is required to manage system services")
	}
	service, ok := s.Services[name]
	if !ok {
		return fmt.Errorf("Unit %s.service could not be found", name)
	}
	service.State = "stopped"
	s.Services[name] = service
	s.clearServiceEffectsLocked(name)
	s.appendJournalLocked(user, name, "info", "Stopped "+name)
	return nil
}
func (s *State) EnableService(name, user string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user != "root" {
		return errors.New("Authentication is required to manage system services")
	}
	service, ok := s.Services[name]
	if !ok {
		return fmt.Errorf("Unit %s.service could not be found", name)
	}
	service.Enabled = enabled
	s.Services[name] = service
	return nil
}
func (s *State) Service(name string) (Service, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	service, ok := s.Services[name]
	return service, ok
}
func (s *State) JournalEntries(unit string) []JournalEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]JournalEntry, 0, len(s.Journal))
	for _, e := range s.Journal {
		if unit == "" || e.Unit == unit {
			out = append(out, e)
		}
	}
	return out
}
func (s *State) AddJournal(unit, priority, message, actor string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendJournalLocked(actor, unit, priority, message)
}
func (s *State) SetSELinux(mode, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user != "root" {
		return errors.New("setenforce: Permission denied")
	}
	if mode != "enforcing" && mode != "permissive" {
		return errors.New("setenforce: invalid mode")
	}
	s.SELinux.Mode = mode
	return nil
}
func (s *State) SELinuxMode() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.SELinux.Mode }
func (s *State) AllowFirewall(zone, service, user string, permanent bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user != "root" {
		return errors.New("FirewallD is not running as root")
	}
	z, ok := s.Network.Zones[zone]
	if !ok {
		return fmt.Errorf("ZONE_CONFLICT: %s", zone)
	}
	if z.Services == nil {
		z.Services = map[string]bool{}
	}
	z.Services[service] = true
	s.Network.Zones[zone] = z
	return nil
}
func (s *State) FirewallAllowed(zone, service string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	z, ok := s.Network.Zones[zone]
	return ok && z.Services[service]
}
func (s *State) Resolve(host string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	address, ok := s.Network.DNS.Records[host]
	return address, ok
}
func (s *State) HTTPGet(rawURL string) (HTTPResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" {
		return HTTPResponse{}, errors.New("curl: unsupported URL")
	}
	response, ok := s.Network.HTTP[rawURL]
	if ok {
		return response, nil
	}
	address, ok := s.Network.DNS.Records[parsed.Hostname()]
	if !ok {
		return HTTPResponse{}, fmt.Errorf("curl: Could not resolve host: %s", parsed.Hostname())
	}
	key := address + ":" + portFor(parsed)
	if !s.Network.OpenPorts[key] {
		return HTTPResponse{Status: 503, Body: "Service Unavailable\n"}, nil
	}
	return HTTPResponse{Status: 404, Body: "Not Found\n"}, nil
}
func (s *State) SetHTTP(rawURL string, response HTTPResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Network.HTTP[rawURL] = response
}
func (s *State) SetPort(address string, port int, protocol, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Network.OpenPorts[address+":"+strconv.Itoa(port)+"/"+protocol] = state == "open"
	if protocol == "tcp" {
		s.Network.OpenPorts[address+":"+strconv.Itoa(port)] = state == "open"
	}
}
func (s *State) SetCWD(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := NormalizePath(dir, s.CWD)
	if err != nil {
		return err
	}
	f, ok := s.Files[p]
	if !ok || !f.Directory {
		return fmt.Errorf("cd: %s: No such file or directory", dir)
	}
	s.CWD = p
	return nil
}

func (s *State) UserInGroup(user, group string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.Users[user]
	if !ok {
		return false
	}
	for _, candidate := range u.Groups {
		if candidate == group {
			return true
		}
	}
	return false
}

func (s *State) AddUserToGroup(user, group, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if actor != "root" {
		return errors.New("only root may modify group membership")
	}
	u, ok := s.Users[user]
	if !ok {
		return fmt.Errorf("usermod: user %s does not exist", user)
	}
	if group == "" {
		return errors.New("usermod: group is empty")
	}
	for _, candidate := range u.Groups {
		if candidate == group {
			return nil
		}
	}
	u.Groups = append(u.Groups, group)
	s.Users[user] = u
	return nil
}

func (s *State) AddUser(name, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if actor != "root" {
		return errors.New("only root may create users")
	}
	if name == "" {
		return errors.New("user name is empty")
	}
	if _, exists := s.Users[name]; exists {
		return fmt.Errorf("user %s already exists", name)
	}
	uid := 1000
	for _, user := range s.Users {
		if user.UID >= uid {
			uid = user.UID + 1
		}
	}
	s.Users[name] = User{Name: name, UID: uid, GID: uid, Groups: []string{name}, Shell: "/bin/bash"}
	return nil
}

func (s *State) SudoAllowed(user, command string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if user == "root" {
		return true
	}
	u, ok := s.Users[user]
	if !ok {
		return false
	}
	if len(s.SudoRules) == 0 {
		for _, group := range u.Groups {
			if group == "wheel" {
				return true
			}
		}
		return false
	}
	for _, rule := range s.SudoRules {
		matches := rule.Subject == user
		if strings.HasPrefix(rule.Subject, "%") {
			group := strings.TrimPrefix(rule.Subject, "%")
			for _, candidate := range u.Groups {
				if candidate == group {
					matches = true
				}
			}
		}
		if matches {
			for _, allowed := range rule.Commands {
				if allowed == "ALL" || allowed == command {
					return true
				}
			}
		}
	}
	return false
}

func (s *State) SnapshotJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return marshalState(s)
}
func (s *State) CurrentTime() time.Time { s.mu.RLock(); defer s.mu.RUnlock(); return s.Clock }
func (s *State) Advance(delta time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Clock = s.Clock.Add(delta)
}

func NormalizePath(input, cwd string) (string, error) {
	if input == "" {
		return "", errors.New("path is empty")
	}
	if !strings.HasPrefix(input, "/") {
		input = path.Join(cwd, input)
	}
	clean := path.Clean(input)
	if clean == "." {
		clean = "/"
	}
	if !strings.HasPrefix(clean, "/") || clean == "/.." || strings.HasPrefix(clean, "/../") {
		return "", fmt.Errorf("path escapes virtual root: %s", input)
	}
	return clean, nil
}
func ParseMode(value string, fallback uint32) (uint32, error) {
	if value == "" {
		return fallback, nil
	}
	base := 8
	if strings.HasPrefix(value, "0o") {
		value = value[2:]
	}
	parsed, err := strconv.ParseUint(value, base, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q", value)
	}
	return uint32(parsed), nil
}

func (s *State) ensureParents(filename string) {
	current := path.Dir(filename)
	for current != "." && current != "/" {
		if _, ok := s.Files[current]; !ok {
			s.Files[current] = File{Path: current, Owner: "root", Group: "root", Mode: 0755, Directory: true}
		}
		current = path.Dir(current)
	}
}
func (s *State) primaryGroupLocked(user string) string {
	if u, ok := s.Users[user]; ok && len(u.Groups) > 0 {
		return u.Groups[0]
	}
	return user
}
func (s *State) canReadLocked(file File, user string) bool {
	return s.permissionLocked(file, user, ModeOwnerRead, ModeGroupRead, ModeOtherRead)
}
func (s *State) canWriteLocked(file File, user string) bool {
	if user == "root" {
		return true
	}
	return s.permissionLocked(file, user, ModeOwnerWrite, ModeGroupWrite, ModeOtherWrite)
}
func (s *State) permissionLocked(file File, user string, owner, group, other uint32) bool {
	if user == "root" {
		return true
	}
	u, ok := s.Users[user]
	if !ok {
		return false
	}
	if file.Owner == user {
		return file.Mode&owner != 0
	}
	for _, g := range u.Groups {
		if g == file.Group {
			return file.Mode&group != 0
		}
	}
	return file.Mode&other != 0
}
func (s *State) conditionLocked(c scenario.ConditionSpec) bool {
	switch c.Type {
	case "fileContains":
		file, ok := s.Files[c.Path]
		return ok && strings.Contains(file.Content, c.Pattern)
	case "selinuxAllows":
		return s.SELinux.Mode != "disabled"
	case "serviceRunning":
		service, ok := s.Services[c.Name]
		return ok && service.State == "running"
	case "firewallServiceAllowed":
		return s.Network.Zones[c.Zone].Services[c.Service]
	}
	return false
}
func (s *State) appendJournalLocked(actor, unit, priority, message string) {
	s.Journal = append(s.Journal, JournalEntry{Timestamp: s.Clock, Priority: priority, Unit: unit, Message: message, Actor: actor})
}
func (s *State) refreshServiceEffects(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshServiceEffectsLocked(name)
}
func (s *State) refreshServiceEffectsLocked(name string) {
	if name != "httpd.service" && name != "httpd" {
		return
	}
	if s.Services[name].State != "running" {
		return
	}
	for zone, z := range s.Network.Zones {
		if z.Services["http"] {
			for _, iface := range s.Network.Interfaces {
				for _, addr := range iface.Addresses {
					s.Network.OpenPorts[strings.Split(addr, "/")[0]+":80"] = true
				}
			}
			_ = zone
		}
	}
}
func (s *State) clearServiceEffectsLocked(name string) {
	if name == "httpd.service" || name == "httpd" {
		for key := range s.Network.OpenPorts {
			if strings.HasSuffix(key, ":80") {
				delete(s.Network.OpenPorts, key)
			}
		}
	}
}
func sliceSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		out[v] = true
	}
	return out
}
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func portFor(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	return "80"
}
