package report_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/redlab/redlab/internal/report"
	redruntime "github.com/redlab/redlab/internal/runtime"
	"github.com/redlab/redlab/internal/scenario"
)

func TestFirstPartyReportsMatchGoldenStructure(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	packs, err := os.ReadDir(filepath.Join(root, "scenario-packs", "core"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pack := range packs {
		if !pack.IsDir() {
			continue
		}
		id := pack.Name()
		t.Run(id, func(t *testing.T) {
			pkg, diagnostics := scenario.LoadScenario(filepath.Join(root, "scenario-packs", "core", id))
			if len(diagnostics) > 0 {
				t.Fatalf("load diagnostics: %v", diagnostics)
			}
			session, err := redruntime.NewSession("golden-"+id, "GOLDEN", pkg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range pkg.Scenario.Spec.ReferenceSolution {
				if result := session.RunLine(command); result.ExitCode != 0 {
					t.Fatalf("reference command %q failed: %+v", command, result)
				}
			}
			text := report.Markdown(session.Report("golden-event"), pkg.Scenario.Spec)
			fixture, err := os.ReadFile(filepath.Join(root, "testdata", "golden", id+"-report.md"))
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
				if strings.HasPrefix(marker, "#") && !strings.Contains(text, marker) {
					t.Fatalf("report missing golden marker %q", marker)
				}
			}
		})
	}
}
