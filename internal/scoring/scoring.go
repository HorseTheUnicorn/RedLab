package scoring

import (
	"github.com/redlab/redlab/internal/rules"
	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/system"
)

type ObjectiveResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Points      int    `json:"points"`
	Earned      int    `json:"earned"`
	Passed      bool   `json:"passed"`
	Explanation string `json:"explanation"`
}
type GuardrailResult struct {
	ID        string `json:"id"`
	Points    int    `json:"points"`
	Triggered bool   `json:"triggered"`
}
type Result struct {
	Objectives []ObjectiveResult `json:"objectives"`
	Guardrails []GuardrailResult `json:"guardrails"`
	Automated  int               `json:"automated"`
	Maximum    int               `json:"maximum"`
	HintsCost  int               `json:"hintsCost"`
}

func Evaluate(spec scenario.ScenarioSpec, state *system.State, hintsCost int) Result {
	result := Result{Maximum: spec.Scoring.AutomatedMaximum, HintsCost: hintsCost}
	for _, objective := range spec.Objectives {
		passed := rules.Evaluate(state, objective.Checks)
		earned := 0
		if passed {
			earned = objective.Points
		}
		result.Objectives = append(result.Objectives, ObjectiveResult{ID: objective.ID, Title: objective.Title, Points: objective.Points, Earned: earned, Passed: passed, Explanation: map[bool]string{true: "objective state is satisfied", false: "objective state is not satisfied"}[passed]})
		result.Automated += earned
	}
	for _, guardrail := range spec.Guardrails {
		triggered := rules.Evaluate(state, guardrail.When)
		if triggered {
			result.Automated += guardrail.Points
		}
		result.Guardrails = append(result.Guardrails, GuardrailResult{ID: guardrail.ID, Points: guardrail.Points, Triggered: triggered})
	}
	result.Automated -= hintsCost
	return result
}
