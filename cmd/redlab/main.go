package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	redlab "github.com/redlab/redlab"
	"github.com/redlab/redlab/internal/auth"
	"github.com/redlab/redlab/internal/bundle"
	"github.com/redlab/redlab/internal/catalog"
	"github.com/redlab/redlab/internal/runtime"
	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/server"
	"github.com/redlab/redlab/internal/store"
	"github.com/redlab/redlab/internal/version"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{Use: "redlab", Short: "Deterministic RHEL 8 hackathon emulator", SilenceUsage: true}
	root.AddCommand(newEventCommand(), newScenarioCommand(), newCatalogCommand(), newPlayCommand(), newServeCommand(), newJoinCommand(), newEvidenceCommand(), newStatusCommand(), newSubmissionsCommand(), newJudgeCommand(), newVersionCommand())
	return root
}
func newVersionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Run: func(*cobra.Command, []string) { fmt.Printf("redlab %s (schema %s)\n", version.Build, version.Schema) }}
}

func newEvidenceCommand() *cobra.Command {
	evidenceCommand := &cobra.Command{Use: "evidence"}
	evidenceCommand.AddCommand(&cobra.Command{Use: "verify <bundle>", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		manifest, err := bundle.Verify(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("verified submission %s for team %s (scenario %s)\n", manifest.SessionID, manifest.TeamID, manifest.ScenarioID)
		return nil
	}})
	return evidenceCommand
}

func newStatusCommand() *cobra.Command {
	var address string
	status := &cobra.Command{Use: "status", RunE: func(_ *cobra.Command, _ []string) error {
		if address == "" {
			address = "http://127.0.0.1:8443"
		}
		response, err := http.Get(strings.TrimRight(address, "/") + "/api/v1/healthz")
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("server health: %s", response.Status)
		}
		io.Copy(os.Stdout, response.Body)
		return nil
	}}
	status.Flags().StringVar(&address, "url", "", "server URL")
	return status
}

func newSubmissionsCommand() *cobra.Command {
	submissions := &cobra.Command{Use: "submissions"}
	list := &cobra.Command{Use: "list [directory]", Args: cobra.MaximumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		directory := "data/submissions"
		if len(args) == 1 {
			directory = args[0]
		}
		return listSubmissions(directory)
	}}
	var output string
	var all bool
	export := &cobra.Command{Use: "export [source-directory]", Args: cobra.MaximumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		source := "data/submissions"
		if len(args) == 1 {
			source = args[0]
		}
		destination := output
		if destination == "" {
			destination = filepath.Join(source, "exported")
		}
		if !all {
			fmt.Println("exporting all verified bundles; pass --all explicitly to make this choice visible")
		}
		return exportSubmissions(source, destination)
	}}
	export.Flags().StringVar(&output, "output", "", "destination directory (overrides the positional output directory)")
	export.Flags().BoolVar(&all, "all", false, "export every verified bundle")
	submissions.AddCommand(list, export)
	return submissions
}

func submissionFiles(directory string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	files := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".rlab.zip") {
			continue
		}
		files = append(files, entry)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	return files, nil
}

func listSubmissions(directory string) error {
	files, err := submissionFiles(directory)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Printf("no submission bundles in %s\n", directory)
		return nil
	}
	for _, file := range files {
		filename := filepath.Join(directory, file.Name())
		manifest, verifyErr := bundle.Verify(filename)
		if verifyErr != nil {
			fmt.Printf("INVALID\t%s\t%v\n", file.Name(), verifyErr)
			continue
		}
		fmt.Printf("%s\tteam=%s\tscenario=%s\tsession=%s\n", file.Name(), manifest.TeamID, manifest.ScenarioID, manifest.SessionID)
	}
	return nil
}

func exportSubmissions(source, destination string) error {
	files, err := submissionFiles(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	exported := 0
	for _, file := range files {
		input := filepath.Join(source, file.Name())
		if _, err := bundle.Verify(input); err != nil {
			return fmt.Errorf("verify %s: %w", input, err)
		}
		data, err := os.ReadFile(input)
		if err != nil {
			return err
		}
		output := filepath.Join(destination, file.Name())
		if _, err := os.Stat(output); err == nil {
			return fmt.Errorf("refusing to overwrite existing export: %s", output)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			return err
		}
		exported++
	}
	fmt.Printf("exported %d verified submission bundle(s) to %s\n", exported, destination)
	return nil
}

func newJudgeCommand() *cobra.Command {
	return &cobra.Command{Use: "judge <bundle>", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		manifest, err := bundle.Verify(args[0])
		if err != nil {
			return err
		}
		model, err := bundle.ReadReport(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("verified bundle: team=%s scenario=%s session=%s automated=%d/%d judge=%d/%d\n", manifest.TeamID, manifest.ScenarioID, manifest.SessionID, model.Score.Automated, model.Score.Maximum, model.JudgeScore, model.JudgeMaximum)
		return nil
	}}
}

func newEventCommand() *cobra.Command {
	event := &cobra.Command{Use: "event"}
	teams := &cobra.Command{Use: "teams"}
	count, prefix := 10, "TEAM-"
	generate := &cobra.Command{Use: "generate <teams.csv>", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return eventTeamsGenerate(args[0], count, prefix) }}
	generate.Flags().IntVar(&count, "count", count, "number of teams to generate")
	generate.Flags().StringVar(&prefix, "prefix", prefix, "team identifier prefix")
	teams.AddCommand(generate)
	event.AddCommand(&cobra.Command{Use: "init <directory>", Args: cobra.ExactArgs(1), RunE: eventInit}, &cobra.Command{Use: "validate <event.yaml>", Args: cobra.ExactArgs(1), RunE: eventValidate}, &cobra.Command{Use: "backup <event.yaml> <destination.db>", Args: cobra.ExactArgs(2), RunE: eventBackup}, teams)
	return event
}

func eventTeamsGenerate(filename string, count int, prefix string) error {
	if count < 1 || count > 10000 {
		return errors.New("team count must be between 1 and 10000")
	}
	if strings.TrimSpace(prefix) == "" || strings.ContainsAny(prefix, ",\r\n") {
		return errors.New("team prefix must be a non-empty CSV-safe value")
	}
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("refusing to overwrite existing team file: %s", filename)
	} else if !os.IsNotExist(err) {
		return err
	}
	credentialsFile := filepath.Join(filepath.Dir(filename), "data", "credentials.json")
	credentials, err := auth.Load(credentialsFile)
	if err != nil {
		return fmt.Errorf("load event credentials: %w", err)
	}
	if err := auth.EnsureValid(credentials); err != nil {
		return err
	}
	var csv strings.Builder
	csv.WriteString("teamID,displayName\n")
	codes := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		teamID := fmt.Sprintf("%s%d", prefix, i)
		if _, exists := credentials.Teams[teamID]; exists {
			return fmt.Errorf("team already exists: %s", teamID)
		}
		code, err := auth.GenerateCode()
		if err != nil {
			return err
		}
		record, err := auth.NewRecord(code)
		if err != nil {
			return err
		}
		credentials.Teams[teamID] = record
		fmt.Fprintf(&csv, "%s,Team %d\n", teamID, i)
		codes = append(codes, teamID+"="+code)
	}
	if err := os.WriteFile(filename, []byte(csv.String()), 0600); err != nil {
		return err
	}
	if err := auth.Save(credentialsFile, credentials); err != nil {
		return err
	}
	fmt.Printf("generated %d teams in %s\n", count, filename)
	for _, code := range codes {
		fmt.Println(code)
	}
	return nil
}

func eventBackup(_ *cobra.Command, args []string) error {
	eventFile := args[0]
	event, diagnostics := scenario.LoadEvent(eventFile)
	if err := printDiagnostics(diagnostics); err != nil {
		return err
	}
	database := event.Spec.Server.Database
	if database == "" {
		database = filepath.Join("data", "event.db")
	}
	if !filepath.IsAbs(database) {
		database = filepath.Join(filepath.Dir(eventFile), database)
	}
	storage, err := store.Open(database)
	if err != nil {
		return err
	}
	defer storage.Close()
	if err := storage.Backup(args[1]); err != nil {
		return err
	}
	fmt.Printf("backed up event database to %s\n", args[1])
	return nil
}
func eventInit(_ *cobra.Command, args []string) error {
	root := args[0]
	if err := os.MkdirAll(filepath.Join(root, "scenarios"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "data"), 0700); err != nil {
		return err
	}
	event := `apiVersion: redlab/v1
kind: Event
metadata:
  id: redlab-event
  title: RedLab Event
spec:
  schedule:
    timezone: UTC
    opensAt: 2026-01-01T09:00:00Z
    submissionsCloseAt: 2099-01-01T17:00:00Z
  scenarios: []
  teams:
    source: ./teams.csv
    joinCodeMode: generated
    maxMembers: 6
  sessions:
    assignment: organizer
    allowRestart: true
    maxRestarts: 1
    idleTimeout: 30m
    maxConcurrent: 40
  scoring:
    hintsEnabled: true
    liveScoreVisible: false
    tiebreakers: [fewerHints, earlierCompletion]
  reports:
    formats: [markdown, json]
    includeTranscript: true
    includeStateDiff: true
    redactSecrets: true
  server:
    listen: 127.0.0.1:8443
    tls:
      mode: generated
    database: ./data/event.db
`
	if err := os.WriteFile(filepath.Join(root, "event.yaml"), []byte(event), 0600); err != nil {
		return err
	}
	organizerSecret, err := auth.GenerateCode()
	if err != nil {
		return err
	}
	teamCode, err := auth.GenerateCode()
	if err != nil {
		return err
	}
	organizerRecord, err := auth.NewRecord(organizerSecret)
	if err != nil {
		return err
	}
	teamRecord, err := auth.NewRecord(teamCode)
	if err != nil {
		return err
	}
	linkToken, err := auth.GenerateToken()
	if err != nil {
		return err
	}
	linkRecord, err := auth.NewRecord(linkToken)
	if err != nil {
		return err
	}
	if err := auth.Save(filepath.Join(root, "data", "credentials.json"), auth.File{Organizer: organizerRecord, Link: linkRecord, Teams: map[string]auth.Record{"TEAM-1": teamRecord}}); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "teams.csv"), []byte("teamID,displayName\nTEAM-1,Practice Team\n"), 0600); err != nil {
		return err
	}
	fmt.Printf("initialized event in %s\nOrganizer recovery secret: %s\nEvent link token: %s\nTEAM-1 join code: %s\n", root, organizerSecret, linkToken, teamCode)
	return nil
}
func eventValidate(_ *cobra.Command, args []string) error {
	event, diagnostics := scenario.LoadEvent(args[0])
	for _, reference := range event.Spec.Scenarios {
		if !reference.Enabled {
			continue
		}
		packagePath := reference.Package
		if !filepath.IsAbs(packagePath) {
			packagePath = filepath.Join(filepath.Dir(args[0]), packagePath)
		}
		_, packageDiagnostics := scenario.LoadScenario(packagePath)
		diagnostics = append(diagnostics, packageDiagnostics...)
	}
	return printDiagnostics(diagnostics)
}

func newScenarioCommand() *cobra.Command {
	scenarioCmd := &cobra.Command{Use: "scenario"}
	initID, initTitle := "custom-scenario", "Custom RedLab Scenario"
	init := &cobra.Command{Use: "init <directory>", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		return scenarioInitTemplate(args[0], initID, initTitle)
	}}
	init.Flags().StringVar(&initID, "id", initID, "scenario metadata ID (lowercase letters, numbers, and hyphens)")
	init.Flags().StringVar(&initTitle, "title", initTitle, "scenario title")
	scenarioCmd.AddCommand(init, &cobra.Command{Use: "validate <directory-or-package>", Args: cobra.ExactArgs(1), RunE: scenarioValidate}, &cobra.Command{Use: "test <directory-or-package>", Args: cobra.ExactArgs(1), RunE: scenarioTest}, &cobra.Command{Use: "pack <directory>", Args: cobra.ExactArgs(1), RunE: scenarioPack}, &cobra.Command{Use: "export <directory-or-package> <archive>", Args: cobra.ExactArgs(2), RunE: scenarioExport}, &cobra.Command{Use: "import <archive> <directory>", Args: cobra.ExactArgs(2), RunE: scenarioImport}, &cobra.Command{Use: "inspect <package>", Args: cobra.ExactArgs(1), RunE: scenarioInspect})
	return scenarioCmd
}

func scenarioInitTemplate(root, id, title string) error {
	if _, err := os.Stat(filepath.Join(root, "scenario.yaml")); err == nil {
		return fmt.Errorf("refusing to overwrite existing scenario: %s", root)
	} else if !os.IsNotExist(err) {
		return err
	}
	if !validScenarioID(id) {
		return errors.New("scenario id must contain 1-64 lowercase letters, numbers, and hyphens")
	}
	if strings.TrimSpace(title) == "" {
		return errors.New("scenario title is required")
	}
	if err := os.MkdirAll(filepath.Join(root, "files"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "judge"), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "scenario.yaml"), scenario.TemplateYAML(id, title), 0600); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "files", "etc", "redlab"), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "files", "etc", "redlab", "README.conf"), []byte("# Add scenario fixture files here.\n"), 0600); err != nil {
		return err
	}
	fmt.Printf("initialized scenario %s (%s) in %s\n", id, title, root)
	fmt.Printf("edit %s, then run: redlab scenario validate %s\n", filepath.Join(root, "scenario.yaml"), root)
	return nil
}

func validScenarioID(id string) bool {
	if len(id) == 0 || len(id) > 64 || id[0] == '-' || id[len(id)-1] == '-' {
		return false
	}
	for index, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || (index > 0 && char == '-') {
			continue
		}
		return false
	}
	return true
}

func scenarioInit(_ *cobra.Command, args []string) error {
	root := args[0]
	if err := os.MkdirAll(filepath.Join(root, "files", "etc", "httpd", "conf"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "judge"), 0700); err != nil {
		return err
	}
	yaml := `apiVersion: redlab/v1
kind: Scenario
metadata:
  id: broken-httpd
  title: Customer Portal Outage
  version: 1.0.0
  authors: [RedLab]
  tags: [rhel8, systemd, httpd, firewalld, selinux]
spec:
  rhel:
    major: 8
    minorProfile: "8.10"
    hostname: app01.example.test
    architecture: x86_64
    selinux: enforcing
    commandPacks: [coreutils, systemd, networking, firewalld, selinux, web]
  briefing:
    difficulty: intermediate
    duration: 60m
    summary: Restore the customer portal and identify both contributing faults.
    objectivesShownToParticipants:
      - Restore the web service.
      - Make the health endpoint return HTTP 200.
      - Preserve SELinux enforcing mode.
  actors:
    initialUser: trainee
    users:
      - name: trainee
        uid: 1000
        groups: [wheel]
        shell: /bin/bash
    sudo:
      - subject: "%wheel"
        commands: [ALL]
        requirePassword: true
  filesystem:
    templates:
      - source: files/
        target: /
    entries:
      - path: /etc/httpd/conf/httpd.conf
        owner: root
        group: root
        mode: "0644"
        selinuxType: httpd_config_t
      - path: /var/log/httpd/error_log
        owner: root
        group: adm
        mode: "0640"
        append: |
          [error] AH00526: Syntax error on line 42
  packages:
    installed:
      - {name: httpd, version: 2.4.37-65.el8}
      - {name: firewalld, version: 1.3.4-9.el8}
  services:
    - name: httpd.service
      enabled: true
      state: failed
      startConditions:
        - type: fileContains
          path: /etc/httpd/conf/httpd.conf
          pattern: "Listen 80"
        - type: selinuxAllows
          subject: httpd_t
          object: httpd_sys_content_t
          permission: read
      onFailure:
        journal:
          - priority: err
            message: "httpd: configuration validation failed"
    - name: firewalld.service
      enabled: true
      state: running
  network:
    interfaces:
      - name: ens192
        state: up
        addresses: [10.20.30.15/24]
    dns:
      servers: [10.20.30.10]
      records:
        app01.example.test: 10.20.30.15
    firewall:
      defaultZone: public
      zones:
        public:
          interfaces: [ens192]
          services: [ssh]
  rules:
    - id: publish-http-port
      when:
        all:
          - {type: serviceRunning, name: httpd.service}
          - {type: firewallServiceAllowed, zone: public, service: http}
      effects:
        - {type: setPort, address: 10.20.30.15, port: 80, protocol: tcp, state: open}
        - {type: setHttpResponse, url: "http://app01.example.test/health", status: 200, body: "ok\n"}
  objectives:
    - id: httpd-running
      title: Restore httpd
      points: 25
      checks: {type: serviceRunning, name: httpd.service}
    - id: health-check
      title: Restore the health endpoint
      points: 25
      checks: {type: httpStatus, url: "http://app01.example.test/health", status: 200}
    - id: keep-selinux-enforcing
      title: Preserve mandatory access controls
      points: 15
      checks: {type: selinuxMode, value: enforcing}
    - id: root-cause
      title: Identify both contributing faults
      points: 20
      response: {type: judgeReviewedText, rubricId: root-cause-rubric}
  guardrails:
    - id: no-global-firewall-disable
      severity: deduction
      points: -10
      when: {type: serviceDisabled, name: firewalld.service}
    - id: no-selinux-disable
      severity: deduction
      points: -15
      when: {type: selinuxModeIn, values: [permissive, disabled]}
  hints:
    - id: check-journal
      cost: 3
      text: Check the service journal before changing configuration.
    - id: check-firewall
      cost: 5
      unlockAfter: 20m
      text: Confirm that the active zone allows the application protocol.
  scoring:
    automatedMaximum: 80
    judgeMaximum: 20
    completionBonus: 5
    minimumPassingScore: 65
  referenceSolution:
    - "printf 'Listen 80\\n' | sudo tee /etc/httpd/conf/httpd.conf"
    - "sudo systemctl start httpd.service"
    - "sudo firewall-cmd --add-service=http"
    - "curl http://app01.example.test/health"
  judgeRubrics:
    - id: root-cause-rubric
      maximum: 12
      criteria:
        - {label: Identifies invalid HTTP configuration, points: 5}
        - {label: Identifies missing firewall permission, points: 5}
        - {label: Explains evidence, points: 2}
`
	if err := os.WriteFile(filepath.Join(root, "scenario.yaml"), []byte(yaml), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "files", "etc", "httpd", "conf", "httpd.conf"), []byte("Listen 0.0.0.0:bad\n"), 0600); err != nil {
		return err
	}
	fmt.Printf("initialized scenario in %s\n", root)
	return nil
}
func scenarioValidate(_ *cobra.Command, args []string) error {
	_, diagnostics := scenario.LoadScenario(args[0])
	return printDiagnostics(diagnostics)
}
func scenarioTest(_ *cobra.Command, args []string) error {
	pkg, diagnostics := scenario.LoadScenario(args[0])
	if err := printDiagnostics(diagnostics); err != nil {
		return err
	}
	session, err := runtime.NewSession("test-session", "TEST", pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return err
	}
	commands := append([]string(nil), pkg.Scenario.Spec.ReferenceSolution...)
	if len(commands) == 0 {
		return errors.New("scenario has no referenceSolution")
	}
	initial := session.Report("test").Score
	for _, line := range commands {
		result := session.RunLine(line)
		if result.ExitCode != 0 {
			return fmt.Errorf("reference solution failed at %q: %s", line, result.Stderr)
		}
	}
	score := session.Report("test").Score
	if score.Automated <= initial.Automated {
		return fmt.Errorf("reference solution did not improve score: initial %d, final %d", initial.Automated, score.Automated)
	}
	for index, alternate := range pkg.Scenario.Spec.ReferenceSolutions {
		candidate, err := runtime.NewSession(fmt.Sprintf("alternate-%d", index), "TEST", pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			return err
		}
		for _, line := range alternate {
			result := candidate.RunLine(line)
			if result.ExitCode != 0 {
				return fmt.Errorf("alternate reference solution %d failed at %q: %s", index+1, line, result.Stderr)
			}
		}
		candidateScore := candidate.Report("test").Score
		if candidateScore.Automated != score.Automated {
			return fmt.Errorf("alternate reference solution %d scored %d, expected %d", index+1, candidateScore.Automated, score.Automated)
		}
	}
	unsafe, err := runtime.NewSession("unsafe-test", "TEST", pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return err
	}
	unsafeCommand := "sudo setenforce permissive"
	if result := unsafe.RunLine(unsafeCommand); result.ExitCode != 0 {
		return fmt.Errorf("guardrail setup failed: %s", result.Stderr)
	}
	if unsafe.Report("test").Score.Automated >= score.Automated {
		return errors.New("unsafe workaround was not deducted")
	}
	replay, err := runtime.NewSession("replay", "TEST", pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return err
	}
	for _, line := range commands {
		replay.RunLine(line)
	}
	left, _ := session.State.SnapshotJSON()
	right, _ := replay.State.SnapshotJSON()
	if !bytes.Equal(left, right) {
		return errors.New("deterministic replay produced a different state")
	}
	if result := replay.RunLine("lab reset"); result.ExitCode != 0 {
		return fmt.Errorf("reset failed: %s", result.Stderr)
	}
	fmt.Printf("scenario test passed: score %d/%d\n", score.Automated, score.Maximum)
	return nil
}
func scenarioPack(_ *cobra.Command, args []string) error {
	root := args[0]
	pkg, diagnostics := scenario.LoadScenario(root)
	if err := printDiagnostics(diagnostics); err != nil {
		return err
	}
	output := strings.TrimSuffix(root, string(filepath.Separator)) + ".rlab"
	if err := pkg.WriteArchiveFile(output); err != nil {
		return err
	}
	fmt.Printf("packed %s (%s)\n", output, pkg.Digest)
	return nil
}
func scenarioExport(_ *cobra.Command, args []string) error {
	pkg, diagnostics := scenario.LoadScenario(args[0])
	if err := printDiagnostics(diagnostics); err != nil {
		return err
	}
	if err := pkg.WriteArchiveFile(args[1]); err != nil {
		return err
	}
	fmt.Printf("exported %s (%s)\n", args[1], pkg.Digest)
	return nil
}
func scenarioImport(_ *cobra.Command, args []string) error {
	if _, err := os.Stat(filepath.Join(args[1], "scenario.yaml")); err == nil {
		return fmt.Errorf("refusing to overwrite existing scenario: %s", args[1])
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := scenario.ExtractPackage(args[0], args[1]); err != nil {
		return err
	}
	pkg, diagnostics := scenario.LoadScenario(args[1])
	if err := printDiagnostics(diagnostics); err != nil {
		return err
	}
	fmt.Printf("imported %s (%s)\n", args[1], pkg.Digest)
	return nil
}
func scenarioInspect(_ *cobra.Command, args []string) error {
	pkg, diagnostics := scenario.LoadScenario(args[0])
	if err := printDiagnostics(diagnostics); err != nil {
		return err
	}
	fmt.Printf("id: %s\ntitle: %s\nversion: %s\ndigest: %s\nfiles: %d\n", pkg.Scenario.Metadata.ID, pkg.Scenario.Metadata.Title, pkg.Scenario.Metadata.Version, pkg.Digest, len(pkg.Files))
	return nil
}

func newCatalogCommand() *cobra.Command {
	var pack, level string
	commands := &cobra.Command{Use: "commands", Args: cobra.NoArgs, Run: func(_ *cobra.Command, _ []string) {
		for _, entry := range catalog.Entries() {
			if pack != "" && entry.Pack != pack || level != "" && string(entry.Level) != strings.ToUpper(level) {
				continue
			}
			fmt.Printf("%-18s %-12s %s\n", entry.Name, entry.Level, entry.Summary)
		}
	}}
	commands.Flags().StringVar(&pack, "pack", "", "filter by functional pack")
	commands.Flags().StringVar(&level, "level", "", "filter by compatibility level A, B, or C")
	root := &cobra.Command{Use: "catalog", Args: cobra.NoArgs}
	root.AddCommand(commands)
	return root
}
func newPlayCommand() *cobra.Command {
	var team, exportPath string
	play := &cobra.Command{Use: "play <scenario>", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		pkg, diagnostics := loadScenarioArgument(args[0])
		if err := printDiagnostics(diagnostics); err != nil {
			return err
		}
		session, err := runtime.NewSession("local", team, pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			return err
		}
		if err := playLoop(os.Stdin, os.Stdout, session); err != nil {
			return err
		}
		if exportPath != "" {
			if !session.IsSubmitted() {
				return errors.New("--export requires `lab submit` before exiting")
			}
			if err := session.ExportBundle(exportPath, "local-practice"); err != nil {
				return err
			}
			fmt.Printf("exported local submission to %s\n", exportPath)
		}
		return nil
	}}
	play.Flags().StringVar(&team, "team", "PRACTICE", "team identifier")
	play.Flags().StringVar(&exportPath, "export", "", "write a signed bundle after `lab submit`")
	return play
}

func loadScenarioArgument(argument string) (scenario.Package, []scenario.Diagnostic) {
	if strings.HasPrefix(argument, "builtin:") || strings.HasPrefix(argument, "core/") {
		files, err := redlab.BuiltinScenarioFiles(argument)
		if err != nil {
			return scenario.Package{}, []scenario.Diagnostic{{Filename: argument, Message: err.Error()}}
		}
		return scenario.LoadScenarioYAML(files["scenario.yaml"], "builtin:"+argument+"/scenario.yaml", files)
	}
	if _, err := os.Stat(argument); os.IsNotExist(err) {
		if files, builtinErr := redlab.BuiltinScenarioFiles(argument); builtinErr == nil {
			return scenario.LoadScenarioYAML(files["scenario.yaml"], "builtin:"+argument+"/scenario.yaml", files)
		}
	}
	return scenario.LoadScenario(argument)
}

func newServeCommand() *cobra.Command {
	var eventFile, address string
	var lan bool
	serve := &cobra.Command{Use: "serve --event <event.yaml>", RunE: func(_ *cobra.Command, _ []string) error {
		if eventFile == "" {
			return errors.New("--event is required")
		}
		event, diagnostics := scenario.LoadEvent(eventFile)
		if err := printDiagnostics(diagnostics); err != nil {
			return err
		}
		packages := map[string]scenario.Package{}
		for _, reference := range event.Spec.Scenarios {
			if !reference.Enabled {
				continue
			}
			packagePath := reference.Package
			if !filepath.IsAbs(packagePath) {
				packagePath = filepath.Join(filepath.Dir(eventFile), packagePath)
			}
			pkg, packageDiagnostics := scenario.LoadScenario(packagePath)
			if err := printDiagnostics(packageDiagnostics); err != nil {
				return err
			}
			packages[pkg.Scenario.Metadata.ID] = pkg
		}
		if len(packages) == 0 {
			return errors.New("event has no enabled scenarios")
		}
		database := event.Spec.Server.Database
		if database == "" {
			database = filepath.Join("data", "event.db")
		}
		if !filepath.IsAbs(database) {
			database = filepath.Join(filepath.Dir(eventFile), database)
		}
		if err := os.MkdirAll(filepath.Dir(database), 0700); err != nil {
			return err
		}
		storage, err := store.Open(database)
		if err != nil {
			return err
		}
		defer storage.Close()
		if address == "" {
			address = event.Spec.Server.Listen
		}
		if address == "" {
			address = "127.0.0.1:8443"
		}
		if !lan {
			address = loopbackAddress(address)
		} else if event.Spec.Server.TLS.Mode == "disabled" {
			return errors.New("LAN mode requires TLS; configure generated or provided TLS")
		}
		app := server.New(eventFile, event, packages, storage)
		if err := app.Recover(); err != nil {
			return err
		}
		certFile, keyFile := "", ""
		if lan {
			host, _, _ := net.SplitHostPort(address)
			if host == "" || host == "0.0.0.0" || host == "::" {
				host = "localhost"
			}
			if event.Spec.Server.TLS.Mode == "provided" {
				certFile = event.Spec.Server.TLS.Certificate
				keyFile = event.Spec.Server.TLS.Key
				if !filepath.IsAbs(certFile) {
					certFile = filepath.Join(filepath.Dir(eventFile), certFile)
				}
				if !filepath.IsAbs(keyFile) {
					keyFile = filepath.Join(filepath.Dir(eventFile), keyFile)
				}
				if certFile == "" || keyFile == "" {
					return errors.New("provided TLS mode requires certificate and key")
				}
			} else {
				var fingerprint string
				var err error
				certFile, keyFile, fingerprint, err = server.EnsureCertificate(filepath.Join(filepath.Dir(eventFile), "data", "tls"), host)
				if err != nil {
					return err
				}
				fmt.Println(server.CertificateMessage(address, fingerprint))
			}
		} else {
			fmt.Printf("RedLab server listening on http://%s\n", address)
		}
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)
		errorsCh := make(chan error, 1)
		go func() {
			if lan {
				errorsCh <- app.ListenAndServeTLS(address, certFile, keyFile)
			} else {
				errorsCh <- app.ListenAndServe(address)
			}
		}()
		select {
		case signal := <-stop:
			_ = signal
			return app.Shutdown()
		case err := <-errorsCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		}
	}}
	serve.Flags().StringVar(&eventFile, "event", "", "event YAML file")
	serve.Flags().StringVar(&address, "addr", "", "listen address override")
	serve.Flags().BoolVar(&lan, "lan", false, "explicitly opt into LAN binding")
	return serve
}

func newJoinCommand() *cobra.Command {
	var teamID, joinCode, linkToken string
	var trustFingerprint string
	join := &cobra.Command{Use: "join <server-url>", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		base := strings.TrimRight(args[0], "/")
		if teamID == "" || joinCode == "" {
			return errors.New("--team and --join-code are required")
		}
		loginBody, _ := json.Marshal(map[string]string{"teamID": teamID, "joinCode": joinCode, "linkToken": linkToken})
		client, tlsConfig, err := joinHTTPClient(base, trustFingerprint)
		if err != nil {
			return err
		}
		loginPath := "/api/v1/auth/team/login"
		if linkToken != "" {
			loginPath = "/api/v1/auth/link"
		}
		response, err := client.Post(base+loginPath, "application/json", bytes.NewReader(loginBody))
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("team login failed: %s", response.Status)
		}
		var login map[string]string
		if err := json.NewDecoder(response.Body).Decode(&login); err != nil {
			return err
		}
		token := login["accessToken"]
		if token == "" {
			return errors.New("server returned no access token")
		}
		request, _ := http.NewRequest(http.MethodPost, base+"/api/v1/teams/"+url.PathEscape(teamID)+"/session", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err = client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
			return fmt.Errorf("session creation failed: %s", response.Status)
		}
		var created map[string]string
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
			return err
		}
		sessionID := created["sessionID"]
		wsURL := strings.Replace(base, "https://", "wss://", 1)
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1) + "/api/v1/sessions/" + url.PathEscape(sessionID) + "/terminal"
		dialer := *websocket.DefaultDialer
		dialer.TLSClientConfig = tlsConfig
		connection, _, err := dialer.Dial(wsURL, http.Header{"Authorization": []string{"Bearer " + token}})
		if err != nil {
			return err
		}
		defer connection.Close()
		return joinLoop(os.Stdin, os.Stdout, connection)
	}}
	join.Flags().StringVar(&teamID, "team", "", "team identifier")
	join.Flags().StringVar(&joinCode, "join-code", "", "team join code")
	join.Flags().StringVar(&linkToken, "link-token", "", "shared event link token (optional)")
	join.Flags().StringVar(&trustFingerprint, "trust-fingerprint", "", "trust the server certificate SHA-256 fingerprint")
	return join
}

func joinHTTPClient(base, fingerprint string) (*http.Client, *tls.Config, error) {
	if !strings.HasPrefix(base, "https://") {
		return http.DefaultClient, nil, nil
	}
	if fingerprint == "" {
		return &http.Client{Timeout: 30 * time.Second}, nil, nil
	}
	expected := strings.ReplaceAll(strings.ToLower(fingerprint), ":", "")
	config := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("server did not present a certificate")
		}
		sum := sha256.Sum256(state.PeerCertificates[0].Raw)
		if hex.EncodeToString(sum[:]) != expected {
			return errors.New("server certificate fingerprint mismatch")
		}
		return nil
	}}
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: config}}, config, nil
}

func joinLoop(in io.Reader, out io.Writer, connection *websocket.Conn) error {
	scanner := bufio.NewScanner(in)
	done := make(chan error, 1)
	go func() {
		for {
			var message struct {
				Type     string `json:"type"`
				Stdout   string `json:"stdout"`
				Stderr   string `json:"stderr"`
				ExitCode int    `json:"exitCode"`
			}
			if err := connection.ReadJSON(&message); err != nil {
				done <- err
				return
			}
			io.WriteString(out, message.Stdout)
			io.WriteString(out, message.Stderr)
		}
	}()
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "exit" {
			return nil
		}
		if err := connection.WriteJSON(map[string]string{"type": "input", "data": line}); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return <-done
}

func loopbackAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "::0" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
func playLoop(in io.Reader, out io.Writer, session *runtime.Session) error {
	scanner := bufio.NewScanner(in)
	fmt.Fprintln(out, "RedLab practice session. Type lab briefing or exit.")
	for {
		fmt.Fprint(out, "redlab$ ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "exit" {
			return nil
		}
		result := session.RunLine(line)
		io.WriteString(out, result.Stdout)
		io.WriteString(out, result.Stderr)
	}
}
func printDiagnostics(diagnostics []scenario.Diagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(os.Stderr, diagnostic.Error())
	}
	return errors.New("validation failed")
}

var _ = json.Valid
