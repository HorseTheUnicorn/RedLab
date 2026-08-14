package redlab

import "testing"

func TestBuiltinScenarioIDsResolve(t *testing.T) {
	ids, err := BuiltinScenarioIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 10 {
		t.Fatalf("built-in scenarios = %d, want 10", len(ids))
	}
	for _, id := range ids {
		files, err := BuiltinScenarioFiles(id)
		if err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		if len(files["scenario.yaml"]) == 0 {
			t.Fatalf("%s has no scenario.yaml", id)
		}
	}
}
