package scenario

import (
	"fmt"
	"strconv"
)

// TemplateYAML returns a small, valid scenario that can be expanded in the
// dashboard or with `scenario init`. It intentionally uses only core fields so
// authors can add services, files, rules, objectives, and rubrics incrementally.
func TemplateYAML(id, title string) []byte {
	return []byte(fmt.Sprintf(`apiVersion: redlab/v1
kind: Scenario
metadata:
  id: %s
  title: %s
  version: 1.0.0
  authors: [RedLab]
  tags: [rhel8, custom]
spec:
  rhel:
    major: 8
    minorProfile: "8.10"
    hostname: lab01.example.test
    architecture: x86_64
    selinux: enforcing
    commandPacks: [coreutils, systemd, networking]
  briefing:
    difficulty: beginner
    duration: 30m
    summary: Build a deterministic RHEL troubleshooting scenario.
    objectivesShownToParticipants: [Inspect the virtual system, Repair the fault, Verify the result]
  actors:
    initialUser: trainee
    users: [{name: trainee, uid: 1000, groups: [wheel], shell: /bin/bash}]
    sudo: [{subject: "%%wheel", commands: [ALL], requirePassword: true}]
  filesystem:
    templates: []
    entries:
      - {path: /etc/redlab/example.conf, owner: root, group: root, mode: "0644", append: "state=broken\n"}
  packages:
    installed: []
  services: []
  network:
    interfaces: []
    dns: {servers: [], records: {}}
    firewall: {defaultZone: public, zones: {public: {interfaces: [], services: []}}}
    simulatedHosts: []
  faults:
    - {id: example-fault, descriptionForJudges: Replace this with the observable fault, evidence: [/etc/redlab/example.conf]}
  rules: []
  objectives:
    - {id: repair-example, title: Repair the example configuration, points: 5, checks: {type: fileContains, path: /etc/redlab/example.conf, pattern: "state=fixed"}}
  guardrails: []
  hints: []
  scoring: {automatedMaximum: 5, judgeMaximum: 0, completionBonus: 0, minimumPassingScore: 5}
  referenceSolution: ["printf 'state=fixed\\n' | sudo tee /etc/redlab/example.conf"]
  referenceSolutions: []
  judgeRubrics: []
`, strconv.Quote(id), strconv.Quote(title)))
}
