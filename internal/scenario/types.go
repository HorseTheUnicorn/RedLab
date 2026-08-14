package scenario

import "time"

type DocumentMeta struct {
	ID      string   `yaml:"id" json:"id"`
	Title   string   `yaml:"title" json:"title"`
	Version string   `yaml:"version,omitempty" json:"version,omitempty"`
	Authors []string `yaml:"authors,omitempty" json:"authors,omitempty"`
	Tags    []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

type Event struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   DocumentMeta `yaml:"metadata" json:"metadata"`
	Spec       EventSpec    `yaml:"spec" json:"spec"`
}

type EventSpec struct {
	Schedule  Schedule        `yaml:"schedule" json:"schedule"`
	Scenarios []EventScenario `yaml:"scenarios" json:"scenarios"`
	Teams     TeamsSpec       `yaml:"teams" json:"teams"`
	Sessions  SessionsSpec    `yaml:"sessions" json:"sessions"`
	Scoring   EventScoring    `yaml:"scoring" json:"scoring"`
	Reports   ReportsSpec     `yaml:"reports" json:"reports"`
	Server    ServerSpec      `yaml:"server" json:"server"`
}

type Schedule struct {
	Timezone           string    `yaml:"timezone" json:"timezone"`
	OpensAt            time.Time `yaml:"opensAt" json:"opensAt"`
	SubmissionsCloseAt time.Time `yaml:"submissionsCloseAt" json:"submissionsCloseAt"`
}

type EventScenario struct {
	Package string `yaml:"package" json:"package"`
	Enabled bool   `yaml:"enabled" json:"enabled"`
}
type TeamsSpec struct {
	Source       string `yaml:"source" json:"source"`
	JoinCodeMode string `yaml:"joinCodeMode" json:"joinCodeMode"`
	MaxMembers   int    `yaml:"maxMembers" json:"maxMembers"`
}
type SessionsSpec struct {
	Assignment    string   `yaml:"assignment" json:"assignment"`
	AllowRestart  bool     `yaml:"allowRestart" json:"allowRestart"`
	MaxRestarts   int      `yaml:"maxRestarts" json:"maxRestarts"`
	IdleTimeout   Duration `yaml:"idleTimeout" json:"idleTimeout"`
	MaxConcurrent int      `yaml:"maxConcurrent" json:"maxConcurrent"`
}
type EventScoring struct {
	HintsEnabled     bool     `yaml:"hintsEnabled" json:"hintsEnabled"`
	LiveScoreVisible bool     `yaml:"liveScoreVisible" json:"liveScoreVisible"`
	Tiebreakers      []string `yaml:"tiebreakers" json:"tiebreakers"`
}
type ReportsSpec struct {
	Formats           []string `yaml:"formats" json:"formats"`
	IncludeTranscript bool     `yaml:"includeTranscript" json:"includeTranscript"`
	IncludeStateDiff  bool     `yaml:"includeStateDiff" json:"includeStateDiff"`
	RedactSecrets     bool     `yaml:"redactSecrets" json:"redactSecrets"`
}
type ServerSpec struct {
	Listen   string  `yaml:"listen" json:"listen"`
	TLS      TLSSpec `yaml:"tls" json:"tls"`
	Database string  `yaml:"database" json:"database"`
}
type TLSSpec struct {
	Mode        string `yaml:"mode" json:"mode"`
	Certificate string `yaml:"certificate,omitempty" json:"certificate,omitempty"`
	Key         string `yaml:"key,omitempty" json:"key,omitempty"`
}

type Scenario struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   DocumentMeta `yaml:"metadata" json:"metadata"`
	Spec       ScenarioSpec `yaml:"spec" json:"spec"`
}

type ScenarioSpec struct {
	RHEL               RHELSpec        `yaml:"rhel" json:"rhel"`
	Briefing           Briefing        `yaml:"briefing" json:"briefing"`
	Actors             ActorsSpec      `yaml:"actors" json:"actors"`
	Filesystem         FilesystemSpec  `yaml:"filesystem" json:"filesystem"`
	Packages           PackagesSpec    `yaml:"packages" json:"packages"`
	Services           []ServiceSpec   `yaml:"services" json:"services"`
	Network            NetworkSpec     `yaml:"network" json:"network"`
	Faults             []FaultSpec     `yaml:"faults" json:"faults"`
	Rules              []RuleSpec      `yaml:"rules" json:"rules"`
	Objectives         []ObjectiveSpec `yaml:"objectives" json:"objectives"`
	Guardrails         []GuardrailSpec `yaml:"guardrails" json:"guardrails"`
	Hints              []HintSpec      `yaml:"hints" json:"hints"`
	Scoring            ScoringSpec     `yaml:"scoring" json:"scoring"`
	JudgeRubrics       []RubricSpec    `yaml:"judgeRubrics" json:"judgeRubrics"`
	ReferenceSolution  []string        `yaml:"referenceSolution,omitempty" json:"referenceSolution,omitempty"`
	ReferenceSolutions [][]string      `yaml:"referenceSolutions,omitempty" json:"referenceSolutions,omitempty"`
}

type RHELSpec struct {
	Major        int      `yaml:"major" json:"major"`
	MinorProfile string   `yaml:"minorProfile" json:"minorProfile"`
	Hostname     string   `yaml:"hostname" json:"hostname"`
	Architecture string   `yaml:"architecture" json:"architecture"`
	SELinux      string   `yaml:"selinux" json:"selinux"`
	CommandPacks []string `yaml:"commandPacks" json:"commandPacks"`
}
type Briefing struct {
	Difficulty                    string   `yaml:"difficulty" json:"difficulty"`
	Duration                      Duration `yaml:"duration" json:"duration"`
	Summary                       string   `yaml:"summary" json:"summary"`
	ObjectivesShownToParticipants []string `yaml:"objectivesShownToParticipants" json:"objectivesShownToParticipants"`
}
type ActorsSpec struct {
	InitialUser string     `yaml:"initialUser" json:"initialUser"`
	Users       []UserSpec `yaml:"users" json:"users"`
	Sudo        []SudoRule `yaml:"sudo" json:"sudo"`
}
type UserSpec struct {
	Name     string   `yaml:"name" json:"name"`
	UID      int      `yaml:"uid" json:"uid"`
	GID      int      `yaml:"gid,omitempty" json:"gid,omitempty"`
	Groups   []string `yaml:"groups" json:"groups"`
	Shell    string   `yaml:"shell" json:"shell"`
	Password string   `yaml:"password,omitempty" json:"password,omitempty"`
}
type SudoRule struct {
	Subject         string   `yaml:"subject" json:"subject"`
	Commands        []string `yaml:"commands" json:"commands"`
	RequirePassword bool     `yaml:"requirePassword" json:"requirePassword"`
}
type FilesystemSpec struct {
	Templates []TemplateSpec `yaml:"templates" json:"templates"`
	Entries   []FileSpec     `yaml:"entries" json:"entries"`
}
type TemplateSpec struct {
	Source string `yaml:"source" json:"source"`
	Target string `yaml:"target" json:"target"`
}
type FileSpec struct {
	Path        string `yaml:"path" json:"path"`
	Owner       string `yaml:"owner" json:"owner"`
	Group       string `yaml:"group" json:"group"`
	Mode        string `yaml:"mode" json:"mode"`
	SELinuxType string `yaml:"selinuxType" json:"selinuxType"`
	Append      string `yaml:"append,omitempty" json:"append,omitempty"`
}
type PackageSpec struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}
type PackagesSpec struct {
	Installed []PackageSpec `yaml:"installed" json:"installed"`
}
type ServiceSpec struct {
	Name            string          `yaml:"name" json:"name"`
	Enabled         bool            `yaml:"enabled" json:"enabled"`
	State           string          `yaml:"state" json:"state"`
	StartConditions []ConditionSpec `yaml:"startConditions" json:"startConditions"`
	OnFailure       FailureSpec     `yaml:"onFailure" json:"onFailure"`
}
type FailureSpec struct {
	Journal []JournalSpec `yaml:"journal" json:"journal"`
}
type JournalSpec struct {
	Priority string `yaml:"priority" json:"priority"`
	Message  string `yaml:"message" json:"message"`
}
type InterfaceSpec struct {
	Name      string   `yaml:"name" json:"name"`
	State     string   `yaml:"state" json:"state"`
	Addresses []string `yaml:"addresses" json:"addresses"`
}
type ZoneSpec struct {
	Interfaces []string `yaml:"interfaces" json:"interfaces"`
	Services   []string `yaml:"services" json:"services"`
	Ports      []string `yaml:"ports,omitempty" json:"ports,omitempty"`
}
type SimHostSpec struct {
	Name    string     `yaml:"name"`
	Address string     `yaml:"address"`
	Ports   []PortSpec `yaml:"ports"`
}
type PortSpec struct {
	Number   int    `yaml:"number"`
	Protocol string `yaml:"protocol"`
	State    string `yaml:"state"`
	Service  string `yaml:"service"`
}
type FirewallSpec struct {
	DefaultZone string              `yaml:"defaultZone"`
	Zones       map[string]ZoneSpec `yaml:"zones"`
}
type NetworkSpec struct {
	Interfaces     []InterfaceSpec `yaml:"interfaces"`
	DNS            DNSSpec         `yaml:"dns"`
	Firewall       FirewallSpec    `yaml:"firewall"`
	SimulatedHosts []SimHostSpec   `yaml:"simulatedHosts"`
}
type DNSSpec struct {
	Servers []string          `yaml:"servers"`
	Records map[string]string `yaml:"records"`
}
type FaultSpec struct {
	ID                   string   `yaml:"id"`
	DescriptionForJudges string   `yaml:"descriptionForJudges"`
	Evidence             []string `yaml:"evidence"`
}
type ConditionSpec struct {
	Type       string   `yaml:"type"`
	Name       string   `yaml:"name,omitempty"`
	Path       string   `yaml:"path,omitempty"`
	Pattern    string   `yaml:"pattern,omitempty"`
	Zone       string   `yaml:"zone,omitempty"`
	Service    string   `yaml:"service,omitempty"`
	Subject    string   `yaml:"subject,omitempty"`
	Object     string   `yaml:"object,omitempty"`
	Permission string   `yaml:"permission,omitempty"`
	Values     []string `yaml:"values,omitempty"`
}
type RuleSpec struct {
	ID      string         `yaml:"id"`
	When    ConditionGroup `yaml:"when"`
	Effects []EffectSpec   `yaml:"effects"`
}
type ConditionGroup struct {
	Type    string           `yaml:"type,omitempty"`
	Name    string           `yaml:"name,omitempty"`
	User    string           `yaml:"user,omitempty"`
	Group   string           `yaml:"group,omitempty"`
	Path    string           `yaml:"path,omitempty"`
	Pattern string           `yaml:"pattern,omitempty"`
	Zone    string           `yaml:"zone,omitempty"`
	Service string           `yaml:"service,omitempty"`
	Value   string           `yaml:"value,omitempty"`
	URL     string           `yaml:"url,omitempty"`
	Status  int              `yaml:"status,omitempty"`
	Values  []string         `yaml:"values,omitempty"`
	All     []ConditionGroup `yaml:"all,omitempty"`
	Any     []ConditionGroup `yaml:"any,omitempty"`
	Not     *ConditionGroup  `yaml:"not,omitempty"`
}
type EffectSpec struct {
	Type     string `yaml:"type"`
	Address  string `yaml:"address,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Protocol string `yaml:"protocol,omitempty"`
	State    string `yaml:"state,omitempty"`
	URL      string `yaml:"url,omitempty"`
	Status   int    `yaml:"status,omitempty"`
	Body     string `yaml:"body,omitempty"`
}
type ObjectiveSpec struct {
	ID       string         `yaml:"id"`
	Title    string         `yaml:"title"`
	Points   int            `yaml:"points"`
	Checks   ConditionGroup `yaml:"checks,omitempty"`
	Response *ResponseSpec  `yaml:"response,omitempty"`
}
type ResponseSpec struct {
	Type     string `yaml:"type"`
	RubricID string `yaml:"rubricId"`
}
type GuardrailSpec struct {
	ID       string         `yaml:"id"`
	Severity string         `yaml:"severity"`
	Points   int            `yaml:"points"`
	When     ConditionGroup `yaml:"when"`
}
type HintSpec struct {
	ID          string   `yaml:"id"`
	Cost        int      `yaml:"cost"`
	UnlockAfter Duration `yaml:"unlockAfter,omitempty"`
	Text        string   `yaml:"text"`
}
type ScoringSpec struct {
	AutomatedMaximum    int `yaml:"automatedMaximum"`
	JudgeMaximum        int `yaml:"judgeMaximum"`
	CompletionBonus     int `yaml:"completionBonus"`
	MinimumPassingScore int `yaml:"minimumPassingScore"`
}
type RubricSpec struct {
	ID       string          `yaml:"id"`
	Maximum  int             `yaml:"maximum"`
	Criteria []CriterionSpec `yaml:"criteria"`
}
type CriterionSpec struct {
	Label  string `yaml:"label"`
	Points int    `yaml:"points"`
}
