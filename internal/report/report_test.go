package report

import (
	"strings"
	"testing"

	"github.com/redlab/redlab/internal/scenario"
)

func TestMarkdownEscapesParticipantText(t *testing.T) {
	model := Model{EventID: "event", ScenarioID: "scenario", TeamID: "team", SessionID: "session", RootCause: "<script>alert(1)</script>\n```", JudgeNotes: "<script>judge</script>\n```", Timeline: nil}
	text := Markdown(model, scenario.ScenarioSpec{})
	if strings.Contains(text, "<script>") || strings.Contains(text, "```") {
		t.Fatalf("unsafe markdown output: %s", text)
	}
}
