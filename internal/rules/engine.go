package rules

import (
	"fmt"
	"strings"

	"github.com/redlab/redlab/internal/scenario"
	"github.com/redlab/redlab/internal/system"
)

type Engine struct{ Rules []scenario.RuleSpec }

func New(specs []scenario.RuleSpec) Engine {
	return Engine{Rules: append([]scenario.RuleSpec(nil), specs...)}
}

func (e Engine) Apply(state *system.State) []string {
	var mutations []string
	for _, rule := range e.Rules {
		if !Evaluate(state, rule.When) {
			continue
		}
		for _, effect := range rule.Effects {
			switch effect.Type {
			case "setPort":
				state.SetPort(effect.Address, effect.Port, effect.Protocol, effect.State)
				mutations = append(mutations, fmt.Sprintf("port:%s:%d/%s=%s", effect.Address, effect.Port, effect.Protocol, effect.State))
			case "setHttpResponse":
				state.SetHTTP(effect.URL, system.HTTPResponse{Status: effect.Status, Body: effect.Body})
				mutations = append(mutations, "http:"+effect.URL)
			}
		}
	}
	return mutations
}

func Evaluate(state *system.State, group scenario.ConditionGroup) bool {
	if len(group.All) > 0 {
		for _, child := range group.All {
			if !Evaluate(state, child) {
				return false
			}
		}
		return true
	}
	if len(group.Any) > 0 {
		for _, child := range group.Any {
			if Evaluate(state, child) {
				return true
			}
		}
		return false
	}
	if group.Not != nil {
		return !Evaluate(state, *group.Not)
	}
	switch group.Type {
	case "serviceRunning":
		service, ok := state.Service(group.Name)
		return ok && service.State == "running"
	case "firewallServiceAllowed":
		return state.FirewallAllowed(group.Zone, group.Service)
	case "selinuxMode":
		return state.SELinuxMode() == group.Value
	case "selinuxModeIn":
		for _, value := range group.Values {
			if state.SELinuxMode() == value {
				return true
			}
		}
		return false
	case "serviceDisabled":
		service, ok := state.Service(group.Name)
		return ok && !service.Enabled
	case "fileContains":
		content, err := state.ReadFile(group.Path, "root")
		return err == nil && strings.Contains(content, group.Pattern)
	case "fileAbsent":
		_, err := state.Stat(group.Path)
		return err != nil
	case "userInGroup":
		return state.UserInGroup(group.User, group.Group)
	case "httpStatus":
		target := group.URL
		if target == "" {
			target = group.Value
		}
		response, err := state.HTTPGet(target)
		return err == nil && response.Status == group.Status
	}
	return false
}

func CheckCondition(state *system.State, group scenario.ConditionGroup) (bool, string) {
	ok := Evaluate(state, group)
	if ok {
		return true, "condition satisfied"
	}
	return false, strings.TrimSpace("condition not satisfied")
}
