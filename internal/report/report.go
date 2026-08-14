package report

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/redlab/redlab/internal/evidence"
	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/scoring"
)

type Model struct {
	EventID          string           `json:"eventID"`
	ScenarioID       string           `json:"scenarioID"`
	TeamID           string           `json:"teamID"`
	SessionID        string           `json:"sessionID"`
	SubmissionID     string           `json:"submissionID"`
	ScenarioDigest   string           `json:"scenarioDigest"`
	BuildVersion     string           `json:"buildVersion"`
	SchemaVersion    string           `json:"schemaVersion"`
	StartedAt        time.Time        `json:"startedAt"`
	SubmittedAt      time.Time        `json:"submittedAt,omitempty"`
	ElapsedSeconds   int64            `json:"elapsedSeconds"`
	VirtualTime      time.Time        `json:"virtualTime"`
	Score            scoring.Result   `json:"score"`
	JudgeScore       int              `json:"judgeScore"`
	JudgeMaximum     int              `json:"judgeMaximum"`
	JudgeNotes       string           `json:"judgeNotes,omitempty"`
	Judged           bool             `json:"judged"`
	TotalScore       int              `json:"totalScore"`
	TotalMaximum     int              `json:"totalMaximum"`
	Passing          bool             `json:"passing"`
	RootCause        string           `json:"rootCause,omitempty"`
	Resolution       string           `json:"resolution,omitempty"`
	Notes            []string         `json:"notes,omitempty"`
	Hints            []HintUse        `json:"hints,omitempty"`
	Rubrics          []Rubric         `json:"rubrics,omitempty"`
	StateDiff        StateDiff        `json:"stateDiff"`
	Timeline         []evidence.Event `json:"timeline"`
	Transcript       string           `json:"transcript,omitempty"`
	EvidenceVerified bool             `json:"evidenceVerified"`
}

type HintUse struct {
	ID   string `json:"id"`
	Cost int    `json:"cost"`
}
type Rubric struct {
	ID       string      `json:"id"`
	Maximum  int         `json:"maximum"`
	Criteria []Criterion `json:"criteria"`
}
type Criterion struct {
	Label  string `json:"label"`
	Points int    `json:"points"`
}

func JSON(model Model) ([]byte, error) { return json.MarshalIndent(model, "", "  ") }
func Markdown(model Model, spec scenario.ScenarioSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# RedLab submission\n\n- Event: `%s`\n- Scenario: `%s`\n- Team: `%s`\n- Session: `%s`\n- Submission: `%s`\n- Scenario digest: `%s`\n- Build: `%s`\n- Schema: `%s`\n- Evidence chain: **%s**\n\n", safe(model.EventID), safe(model.ScenarioID), safe(model.TeamID), safe(model.SessionID), safe(model.SubmissionID), safe(model.ScenarioDigest), safe(model.BuildVersion), safe(model.SchemaVersion), map[bool]string{true: "verified", false: "not verified"}[model.EvidenceVerified])
	b.WriteString("## Timing\n\n")
	fmt.Fprintf(&b, "- Started: `%s`\n- Submitted: `%s`\n- Elapsed: `%d seconds`\n- Virtual time: `%s`\n\n", model.StartedAt.UTC().Format(time.RFC3339), formatTime(model.SubmittedAt), model.ElapsedSeconds, model.VirtualTime.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "## Score\n\nAutomated score: **%d / %d**\n\n", model.Score.Automated, model.Score.Maximum)
	if model.JudgeMaximum > 0 {
		fmt.Fprintf(&b, "Judge score: **%d / %d**\n\n", model.JudgeScore, model.JudgeMaximum)
	}
	fmt.Fprintf(&b, "Total score: **%d / %d** (%s)\n\n", model.TotalScore, model.TotalMaximum, map[bool]string{true: "passing", false: "not passing"}[model.Passing])
	b.WriteString("| Objective | Result | Points |\n| --- | --- | ---: |\n")
	for _, objective := range model.Score.Objectives {
		fmt.Fprintf(&b, "| %s | %s | %d / %d |\n", safe(objective.Title), map[bool]string{true: "passed", false: "not passed"}[objective.Passed], objective.Earned, objective.Points)
	}
	b.WriteString("\n## Guardrails\n\n")
	for _, guardrail := range model.Score.Guardrails {
		fmt.Fprintf(&b, "- `%s`: %s (%d)\n", safe(guardrail.ID), map[bool]string{true: "triggered", false: "not triggered"}[guardrail.Triggered], guardrail.Points)
	}
	if len(model.Hints) > 0 {
		b.WriteString("\n## Hints used\n\n")
		for _, hint := range model.Hints {
			fmt.Fprintf(&b, "- `%s` (cost %d)\n", safe(hint.ID), hint.Cost)
		}
	}
	if len(model.Rubrics) > 0 {
		b.WriteString("\n## Judge rubric\n\n")
		for _, rubric := range model.Rubrics {
			fmt.Fprintf(&b, "### %s — %d points\n\n", safe(rubric.ID), rubric.Maximum)
			for _, criterion := range rubric.Criteria {
				fmt.Fprintf(&b, "- %s (%d)\n", safe(criterion.Label), criterion.Points)
			}
		}
	}
	if model.JudgeNotes != "" {
		fmt.Fprintf(&b, "\n## Judge notes\n\n%s\n\n", fence(model.JudgeNotes))
	}
	if model.RootCause != "" {
		fmt.Fprintf(&b, "\n## Participant explanation\n\n### Root cause\n\n%s\n\n", fence(model.RootCause))
	}
	if model.Resolution != "" {
		fmt.Fprintf(&b, "### Resolution\n\n%s\n\n", fence(model.Resolution))
	}
	if len(model.Notes) > 0 {
		b.WriteString("### Participant notes\n\n")
		for _, note := range model.Notes {
			fmt.Fprintf(&b, "- %s\n", fence(note))
		}
		b.WriteByte('\n')
	}
	if !model.StateDiff.Empty() {
		b.WriteString("## State changes\n\n")
		for _, file := range model.StateDiff.Files {
			fmt.Fprintf(&b, "- File `%s`: %s%s\n", safe(file.Path), file.Change, map[bool]string{true: " (content changed)", false: ""}[file.ContentChanged])
		}
		for _, service := range model.StateDiff.Services {
			fmt.Fprintf(&b, "- Service `%s`: %s\n", safe(service.Name), service.Change)
		}
		for _, user := range model.StateDiff.Users {
			fmt.Fprintf(&b, "- User `%s`: %s\n", safe(user.Name), user.Change)
		}
		for _, firewall := range model.StateDiff.Firewall {
			fmt.Fprintf(&b, "- Firewall zone `%s`: services `%s`, ports `%s`\n", safe(firewall.Zone), safe(strings.Join(firewall.AfterServices, ", ")), safe(strings.Join(firewall.AfterPorts, ", ")))
		}
		for _, pkg := range model.StateDiff.Packages {
			fmt.Fprintf(&b, "- Package `%s`: %s (%s → %s)\n", safe(pkg.Name), pkg.Change, safe(pkg.Before), safe(pkg.After))
		}
		b.WriteByte('\n')
	}
	b.WriteString("## Timeline\n\n")
	for _, event := range model.Timeline {
		fmt.Fprintf(&b, "- `%d` `%s` `%s` exit `%d`", event.Sequence, safe(event.Type), safe(event.Command), event.ExitCode)
		if len(event.Mutations) > 0 {
			fmt.Fprintf(&b, " — %s", safe(strings.Join(event.Mutations, ", ")))
		}
		b.WriteByte('\n')
	}
	if model.Transcript != "" {
		b.WriteString("\n## Transcript\n\n```text\n")
		b.WriteString(strings.ReplaceAll(model.Transcript, "```", "`` `"))
		b.WriteString("\n```\n")
	}
	_ = spec
	return b.String()
}
func formatTime(value time.Time) string {
	if value.IsZero() {
		return "not submitted"
	}
	return value.UTC().Format(time.RFC3339)
}
func safe(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(value, "`", "'"), "\r", ""), "\n", " ")
}
func fence(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(html.EscapeString(value), "```", "`` `"))
}
func SortedTimeline(events []evidence.Event) []evidence.Event {
	out := append([]evidence.Event(nil), events...)
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}
