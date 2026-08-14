package runtime

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/redlab/redlab/internal/bundle"
	"github.com/redlab/redlab/internal/command"
	"github.com/redlab/redlab/internal/evidence"
	"github.com/redlab/redlab/internal/report"
	"github.com/redlab/redlab/internal/rules"
	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/scoring"
	"github.com/redlab/redlab/internal/shell"
	"github.com/redlab/redlab/internal/system"
	"github.com/redlab/redlab/internal/version"
)

type Session struct {
	mu              sync.Mutex
	ID              string
	TeamID          string
	Package         scenario.Package
	Scenario        scenario.Scenario
	State           *system.State
	Registry        *command.Registry
	Rules           rules.Engine
	Chain           evidence.Chain
	History         []string
	Transcript      strings.Builder
	Notes           []string
	RootCause       string
	Resolution      string
	Hints           map[string]bool
	HintCost        int
	Started         time.Time
	Submitted       bool
	SubmittedAt     time.Time
	JudgeScore      int
	JudgeNotes      string
	Judged          bool
	EventPublicKey  ed25519.PublicKey
	EventPrivateKey ed25519.PrivateKey
}

const maxTranscriptBytes = 1 << 20

func NewSession(id, team string, pkg scenario.Package, seed time.Time) (*Session, error) {
	state, err := system.NewState(pkg, seed)
	if err != nil {
		return nil, err
	}
	publicKey, privateKey, err := evidence.GenerateKey()
	if err != nil {
		return nil, err
	}
	registry := command.NewRegistry()
	command.RegisterCore(registry)
	session := &Session{ID: id, TeamID: team, Package: pkg, Scenario: pkg.Scenario, State: state, Registry: registry, Rules: rules.New(pkg.Scenario.Spec.Rules), Hints: map[string]bool{}, Started: seed, EventPublicKey: publicKey, EventPrivateKey: privateKey}
	return session, nil
}

func Replay(id, team string, pkg scenario.Package, events []evidence.Event) (*Session, error) {
	seed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if len(events) > 0 {
		seed = events[0].VirtualTimestamp.Add(-time.Second)
	}
	session, err := NewSession(id, team, pkg, seed)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.Type == "command" {
			session.RunLine(event.Command)
		}
	}
	if err := session.Chain.Restore(events); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Session) RunLine(input string) command.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	input = strings.TrimSpace(input)
	if input == "" {
		return command.Result{}
	}
	if len(input) > 16<<10 {
		return s.record(input, command.Result{ExitCode: 2, Stderr: "command exceeds 16 KiB\n"})
	}
	statements, err := shell.Parse(input)
	if err != nil {
		return s.record(input, command.Result{ExitCode: 2, Stderr: err.Error() + "\n"})
	}
	env := &command.Env{State: s.State, Variables: s.State.Env, History: s.History, User: s.State.CurrentUser, CWD: s.State.CWD, Lab: s}
	last := 0
	var stdout, stderr strings.Builder
	for _, statement := range statements {
		if statement.Previous == shell.OpAnd && last != 0 {
			continue
		}
		if statement.Previous == shell.OpOr && last == 0 {
			continue
		}
		result := s.runPipeline(env, statement.Pipeline)
		last = result.ExitCode
		stdout.WriteString(result.Stdout)
		stderr.WriteString(result.Stderr)
		s.History = append(s.History, input)
		env.History = s.History
		env.CWD = s.State.CWD
		if result.ExitCode != 0 {
			break
		}
	}
	result := command.Result{ExitCode: last, Stdout: stdout.String(), Stderr: stderr.String()}
	result.Mutations = s.Rules.Apply(s.State)
	s.State.Advance(time.Second)
	return s.record(input, result)
}

func (s *Session) Restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.State.Reset(); err != nil {
		return err
	}
	s.Chain = evidence.Chain{}
	s.History = nil
	s.Transcript.Reset()
	s.Notes = nil
	s.RootCause = ""
	s.Resolution = ""
	s.Hints = map[string]bool{}
	s.HintCost = 0
	s.Submitted = false
	s.SubmittedAt = time.Time{}
	s.Judged = false
	s.JudgeScore = 0
	s.JudgeNotes = ""
	s.Started = s.State.CurrentTime()
	return nil
}

func (s *Session) SetJudge(score int, notes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if score < 0 || score > s.Scenario.Spec.Scoring.JudgeMaximum {
		return fmt.Errorf("judge score must be between 0 and %d", s.Scenario.Spec.Scoring.JudgeMaximum)
	}
	s.JudgeScore = score
	s.JudgeNotes = notes
	s.Judged = true
	return nil
}

func (s *Session) IsSubmitted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Submitted
}

func (s *Session) RestoreSubmission(submittedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Submitted = true
	s.SubmittedAt = submittedAt
}

func (s *Session) runPipeline(env *command.Env, pipeline shell.Pipeline) command.Result {
	stdin := ""
	result := command.Result{}
	for _, parsed := range pipeline.Commands {
		if len(parsed.Words) == 0 {
			continue
		}
		words := make([]string, len(parsed.Words))
		words = words[:0]
		for i, word := range parsed.Words {
			expanded := expand(word, env)
			if i > 0 && strings.ContainsAny(expanded, "*?[") {
				matches := s.State.Glob(expanded, env.User)
				if len(matches) > 0 {
					words = append(words, matches...)
					continue
				}
			}
			words = append(words, expanded)
		}
		input := stdin
		for _, redirection := range parsed.Redirections {
			if redirection.Operator == "<" {
				content, err := s.State.ReadFile(expand(redirection.Target, env), env.User)
				if err != nil {
					return command.Result{ExitCode: 1, Stderr: err.Error() + "\n"}
				}
				input = content
			}
		}
		result = s.Registry.Run(words[0], env, words[1:], input)
		for _, redirection := range parsed.Redirections {
			target := expand(redirection.Target, env)
			switch redirection.Operator {
			case ">", ">>":
				if err := s.State.WriteFile(target, result.Stdout, env.User, redirection.Operator == ">>"); err != nil {
					return command.Result{ExitCode: 1, Stderr: err.Error() + "\n"}
				}
				result.Stdout = ""
			case "2>":
				if err := s.State.WriteFile(target, result.Stderr, env.User, false); err != nil {
					return command.Result{ExitCode: 1, Stderr: err.Error() + "\n"}
				}
				result.Stderr = ""
			case "2>&1":
				result.Stdout += result.Stderr
				result.Stderr = ""
			}
		}
		stdin = result.Stdout
	}
	return result
}

func (s *Session) record(input string, result command.Result) command.Result {
	now := s.State.CurrentTime()
	s.Chain.Append(evidence.Event{SessionID: s.ID, VirtualTimestamp: now, Timestamp: now, Type: "command", Actor: s.State.CurrentUser, Command: evidence.Redact(input, nil), ExitCode: result.ExitCode, Mutations: append([]string(nil), result.Mutations...)})
	entry := fmt.Sprintf("$ %s\n%s%s", input, result.Stdout, result.Stderr)
	if s.Transcript.Len() < maxTranscriptBytes {
		remaining := maxTranscriptBytes - s.Transcript.Len()
		if len(entry) > remaining {
			entry = entry[:remaining]
			if !strings.HasSuffix(entry, "\n") {
				entry += "\n"
			}
			entry += "[transcript truncated by RedLab]\n"
		}
		s.Transcript.WriteString(entry)
	}
	return result
}

func (s *Session) RunLab(args []string, env *command.Env) command.Result {
	if len(args) == 0 {
		args = []string{"status"}
	}
	switch args[0] {
	case "briefing":
		return command.Result{Stdout: s.Scenario.Spec.Briefing.Summary + "\n\nObjectives:\n- " + strings.Join(s.Scenario.Spec.Briefing.ObjectivesShownToParticipants, "\n- ") + "\n"}
	case "objectives", "status":
		score := scoring.Evaluate(s.Scenario.Spec, s.State, s.HintCost)
		var b strings.Builder
		fmt.Fprintf(&b, "Score: %d/%d\n", score.Automated, score.Maximum)
		for _, objective := range score.Objectives {
			fmt.Fprintf(&b, "[%s] %s (%d/%d)\n", map[bool]string{true: "x", false: " "}[objective.Passed], objective.Title, objective.Earned, objective.Points)
		}
		return command.Result{Stdout: b.String()}
	case "hint":
		if len(args) < 2 {
			return command.Result{ExitCode: 1, Stderr: "lab hint: specify a hint id\n"}
		}
		for _, hint := range s.Scenario.Spec.Hints {
			if hint.ID != args[1] {
				continue
			}
			if s.Hints[hint.ID] {
				return command.Result{Stdout: "Hint already used.\n"}
			}
			if s.State.CurrentTime().Sub(s.Started) < hint.UnlockAfter.Duration() {
				return command.Result{ExitCode: 1, Stderr: "Hint is not unlocked yet.\n"}
			}
			s.Hints[hint.ID] = true
			s.HintCost += hint.Cost
			return command.Result{Stdout: hint.Text + "\n", Mutations: []string{"hint:" + hint.ID}}
		}
		return command.Result{ExitCode: 1, Stderr: "unknown hint\n"}
	case "note":
		text := strings.Join(args[1:], " ")
		s.Notes = append(s.Notes, text)
		return command.Result{Stdout: "Note saved.\n", Mutations: []string{"note"}}
	case "answer":
		text := strings.Join(args[1:], " ")
		if s.RootCause == "" {
			s.RootCause = text
		} else {
			s.Resolution = text
		}
		return command.Result{Stdout: "Answer saved.\n", Mutations: []string{"answer"}}
	case "check":
		score := scoring.Evaluate(s.Scenario.Spec, s.State, s.HintCost)
		return command.Result{Stdout: fmt.Sprintf("Automated score: %d/%d\n", score.Automated, score.Maximum)}
	case "evidence":
		if err := s.Chain.Verify(); err != nil {
			return command.Result{ExitCode: 1, Stderr: err.Error() + "\n"}
		}
		return command.Result{Stdout: fmt.Sprintf("Evidence chain verified (%d events).\n", len(s.Chain.Snapshot()))}
	case "reset":
		if err := s.State.Reset(); err != nil {
			return command.Result{ExitCode: 1, Stderr: err.Error() + "\n"}
		}
		s.Hints = map[string]bool{}
		s.HintCost = 0
		return command.Result{Stdout: "Session reset.\n", Mutations: []string{"reset"}}
	case "submit":
		s.Submitted = true
		s.SubmittedAt = s.State.CurrentTime()
		return command.Result{Stdout: "Submission recorded.\n", Mutations: []string{"submit"}}
	default:
		return command.Result{ExitCode: 1, Stderr: "lab: unsupported subcommand " + args[0] + "\n"}
	}
}

func (s *Session) Report(eventID string) report.Model {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reportLocked(eventID)
}

func (s *Session) reportLocked(eventID string) report.Model {
	score := scoring.Evaluate(s.Scenario.Spec, s.State, s.HintCost)
	current := s.State.CurrentTime()
	submittedAt := s.SubmittedAt
	if submittedAt.IsZero() && s.Submitted {
		submittedAt = current
	}
	elapsed := current.Sub(s.Started)
	if !submittedAt.IsZero() {
		elapsed = submittedAt.Sub(s.Started)
	}
	if elapsed < 0 {
		elapsed = 0
	}
	hints := make([]report.HintUse, 0, len(s.Hints))
	for _, hint := range s.Scenario.Spec.Hints {
		if s.Hints[hint.ID] {
			hints = append(hints, report.HintUse{ID: hint.ID, Cost: hint.Cost})
		}
	}
	rubrics := make([]report.Rubric, 0, len(s.Scenario.Spec.JudgeRubrics))
	for _, rubric := range s.Scenario.Spec.JudgeRubrics {
		criteria := make([]report.Criterion, 0, len(rubric.Criteria))
		for _, criterion := range rubric.Criteria {
			criteria = append(criteria, report.Criterion{Label: criterion.Label, Points: criterion.Points})
		}
		rubrics = append(rubrics, report.Rubric{ID: rubric.ID, Maximum: rubric.Maximum, Criteria: criteria})
	}
	return report.Model{EventID: eventID, ScenarioID: s.Scenario.Metadata.ID, TeamID: s.TeamID, SessionID: s.ID, SubmissionID: "submission-" + s.ID, ScenarioDigest: s.Package.Digest, BuildVersion: version.Build, SchemaVersion: version.Schema, StartedAt: s.Started, SubmittedAt: submittedAt, ElapsedSeconds: int64(elapsed / time.Second), VirtualTime: current, Score: score, JudgeScore: s.JudgeScore, JudgeMaximum: s.Scenario.Spec.Scoring.JudgeMaximum, JudgeNotes: s.JudgeNotes, Judged: s.Judged, TotalScore: score.Automated + s.JudgeScore, TotalMaximum: score.Maximum + s.Scenario.Spec.Scoring.JudgeMaximum, Passing: score.Automated+s.JudgeScore >= s.Scenario.Spec.Scoring.MinimumPassingScore, RootCause: s.RootCause, Resolution: s.Resolution, Notes: append([]string(nil), s.Notes...), Hints: hints, Rubrics: rubrics, StateDiff: report.Diff(s.State.Initial, s.State), Timeline: report.SortedTimeline(s.Chain.Snapshot()), Transcript: s.Transcript.String(), EvidenceVerified: s.Chain.Verify() == nil}
}
func (s *Session) TranscriptText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Transcript.String()
}
func (s *Session) WriteTranscript(w io.Writer) error {
	_, err := io.WriteString(w, s.TranscriptText())
	return err
}

func (s *Session) ExportBundle(filename, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	model := s.reportLocked(eventID)
	manifest := evidence.Manifest{EventID: eventID, ScenarioID: s.Scenario.Metadata.ID, TeamID: s.TeamID, SessionID: s.ID, ScenarioDigest: s.Package.Digest}
	return bundle.Write(filename, bundle.Input{Report: model, Scenario: s.Scenario, Events: s.Chain.Snapshot(), Manifest: manifest, PrivateKey: s.EventPrivateKey})
}

func expand(word string, env *command.Env) string {
	if word == "~" {
		if home, ok := env.Variables["HOME"]; ok {
			return home
		}
		return "/root"
	}
	var b strings.Builder
	for i := 0; i < len(word); {
		if word[i] != '$' {
			b.WriteByte(word[i])
			i++
			continue
		}
		i++
		if i < len(word) && word[i] == '{' {
			end := strings.IndexByte(word[i+1:], '}')
			if end < 0 {
				b.WriteByte('$')
				continue
			}
			key := word[i+1 : i+1+end]
			i = i + 1 + end + 1
			b.WriteString(env.Variables[key])
			continue
		}
		start := i
		for i < len(word) && ((word[i] >= 'A' && word[i] <= 'Z') || (word[i] >= 'a' && word[i] <= 'z') || (word[i] >= '0' && word[i] <= '9') || word[i] == '_') {
			i++
		}
		if start == i {
			b.WriteByte('$')
			continue
		}
		b.WriteString(env.Variables[word[start:i]])
	}
	return b.String()
}
