package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzStrictScenarioDecode(f *testing.F) {
	f.Add([]byte("apiVersion: redlab/v1\nkind: Scenario\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		filename := filepath.Join(t.TempDir(), "scenario.yaml")
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		_, _ = LoadScenario(filepath.Dir(filename))
	})
}
