package slash

import (
	"context"
	"fmt"
	"github.com/artpar/pragma/internal/observe"
	"os"
	"strings"
)

const orchestrateUsage = "Usage: /orchestrate <orchestration.yaml> --persona-dir <persona-dir> [--start-at-state <state>] [--stop-after-state <state>] --prompt <task prompt>"

func handleOrchestrate(_ context.Context, args string, _ Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	req, err := parseOrchestrateArgs(args)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Result{DisplayText: err.Error() + \"\\n\\n\" + orchestrateUsage}, nil")
		return Result{DisplayText: err.Error() + "\n\n" + orchestrateUsage}, nil
	}
	if info, err := os.Stat(req.DefinitionPath); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"orchestration YAML not found: %s\\n\\n%s\", req...")
		return Result{DisplayText: fmt.Sprintf("orchestration YAML not found: %s\n\n%s", req.DefinitionPath, orchestrateUsage)}, nil
	} else if info.IsDir() {
		observe.GlobalTrace("else-if: info.IsDir()")
		return Result{DisplayText: fmt.Sprintf("orchestration YAML is a directory: %s\n\n%s", req.DefinitionPath, orchestrateUsage)}, nil
	}
	if info, err := os.Stat(req.PersonaDir); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"persona directory not found: %s\\n\\n%s\", req....")
		return Result{DisplayText: fmt.Sprintf("persona directory not found: %s\n\n%s", req.PersonaDir, orchestrateUsage)}, nil
	} else if !info.IsDir() {
		observe.GlobalTrace("else-if: !info.IsDir()")
		return Result{DisplayText: fmt.Sprintf("persona path is not a directory: %s\n\n%s", req.PersonaDir, orchestrateUsage)}, nil
	}
	observe.GlobalTrace("return: Result{Orchestrate: &req}, nil")
	return Result{Orchestrate: &req}, nil
}

func parseOrchestrateArgs(args string) (OrchestrationRequest, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		observe.GlobalTrace("if: len(fields) == 0")
		observe.GlobalTrace("return: OrchestrationRequest{}, fmt.Errorf(\"missing orchestration YAML path\")")
		return OrchestrationRequest{}, fmt.Errorf("missing orchestration YAML path")
	}

	req := OrchestrationRequest{DefinitionPath: fields[0]}
	var trailing []string
	for i := 1; i < len(fields); i++ {
		observe.GlobalTrace("for: i < len(fields)")
		field := fields[i]
		switch {
		case field == "--persona-dir":
			observe.GlobalTrace("case: field == \"--persona-dir\"")
			i++
			if i >= len(fields) {
				observe.GlobalTrace("return: OrchestrationRequest{}, fmt.Errorf(\"missing value for --persona-dir\")")
				return OrchestrationRequest{}, fmt.Errorf("missing value for --persona-dir")
			}
			req.PersonaDir = fields[i]
		case strings.HasPrefix(field, "--persona-dir="):
			observe.GlobalTrace("case: strings.HasPrefix(field, \"--persona-dir=\")")
			req.PersonaDir = strings.TrimPrefix(field, "--persona-dir=")
		case field == "--start-at-state":
			observe.GlobalTrace("case: field == \"--start-at-state\"")
			i++
			if i >= len(fields) {
				observe.GlobalTrace("return: OrchestrationRequest{}, fmt.Errorf(\"missing value for --start-at-state\")")
				return OrchestrationRequest{}, fmt.Errorf("missing value for --start-at-state")
			}
			req.StartAtState = fields[i]
		case strings.HasPrefix(field, "--start-at-state="):
			observe.GlobalTrace("case: strings.HasPrefix(field, \"--start-at-state=\")")
			req.StartAtState = strings.TrimPrefix(field, "--start-at-state=")
		case field == "--stop-after-state":
			observe.GlobalTrace("case: field == \"--stop-after-state\"")
			i++
			if i >= len(fields) {
				observe.GlobalTrace("return: OrchestrationRequest{}, fmt.Errorf(\"missing value for --stop-after-state\")")
				return OrchestrationRequest{}, fmt.Errorf("missing value for --stop-after-state")
			}
			req.StopAfterState = fields[i]
		case strings.HasPrefix(field, "--stop-after-state="):
			observe.GlobalTrace("case: strings.HasPrefix(field, \"--stop-after-state=\")")
			req.StopAfterState = strings.TrimPrefix(field, "--stop-after-state=")
		case field == "--prompt":
			observe.GlobalTrace("case: field == \"--prompt\"")
			i++
			if i >= len(fields) {
				observe.GlobalTrace("return: OrchestrationRequest{}, fmt.Errorf(\"missing value for --prompt\")")
				return OrchestrationRequest{}, fmt.Errorf("missing value for --prompt")
			}
			req.Prompt = strings.Join(fields[i:], " ")
			i = len(fields)
		case strings.HasPrefix(field, "--prompt="):
			observe.GlobalTrace("case: strings.HasPrefix(field, \"--prompt=\")")
			req.Prompt = strings.TrimPrefix(field, "--prompt=")
		case strings.HasPrefix(field, "--"):
			observe.GlobalTrace("case: strings.HasPrefix(field, \"--\")")
			return OrchestrationRequest{}, fmt.Errorf("unknown flag %s", field)
		default:
			observe.GlobalTrace("default")
			trailing = append(trailing, field)
		}
	}
	if req.Prompt == "" && len(trailing) > 0 {
		observe.GlobalTrace("if: req.Prompt == \"\" && len(trailing) > 0")
		req.Prompt = strings.Join(trailing, " ")
	}
	if strings.TrimSpace(req.PersonaDir) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(req.PersonaDir) == \"\"")
		observe.GlobalTrace("return: OrchestrationRequest{}, fmt.Errorf(\"missing --persona-dir\")")
		return OrchestrationRequest{}, fmt.Errorf("missing --persona-dir")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(req.Prompt) == \"\"")
		observe.GlobalTrace("return: OrchestrationRequest{}, fmt.Errorf(\"missing task prompt\")")
		return OrchestrationRequest{}, fmt.Errorf("missing task prompt")
	}
	observe.GlobalTrace("return: req, nil")
	return req, nil
}

type OrchestrateCompletionState struct {
	HasDefinition bool
	HasPersonaDir bool
	HasStartAt    bool
	HasStopAfter  bool
	HasPrompt     bool
	CurrentRole   string
}

func ParseOrchestrateCompletionState(args []string, currentIndex int) OrchestrateCompletionState {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	state := OrchestrateCompletionState{}
	expectPersona := false
	expectStartAt := false
	expectStopAfter := false
	inPrompt := false
	for i, arg := range args {
		observe.GlobalTrace("range args")
		role := "prompt"
		switch {
		case inPrompt:
			observe.GlobalTrace("case: inPrompt")
			role = "prompt"
		case expectPersona:
			observe.GlobalTrace("case: expectPersona")
			role = "persona"
			expectPersona = false
		case expectStartAt:
			observe.GlobalTrace("case: expectStartAt")
			role = "state"
			expectStartAt = false
		case expectStopAfter:
			observe.GlobalTrace("case: expectStopAfter")
			role = "state"
			expectStopAfter = false
		case arg == "--persona-dir":
			observe.GlobalTrace("case: arg == \"--persona-dir\"")
			role = "flag"
			state.HasPersonaDir = true
			expectPersona = true
		case strings.HasPrefix(arg, "--persona-dir="):
			observe.GlobalTrace("case: strings.HasPrefix(arg, \"--persona-dir=\")")
			role = "flag"
			state.HasPersonaDir = true
		case arg == "--start-at-state":
			observe.GlobalTrace("case: arg == \"--start-at-state\"")
			role = "flag"
			state.HasStartAt = true
			expectStartAt = true
		case strings.HasPrefix(arg, "--start-at-state="):
			observe.GlobalTrace("case: strings.HasPrefix(arg, \"--start-at-state=\")")
			role = "flag"
			state.HasStartAt = true
		case arg == "--stop-after-state":
			observe.GlobalTrace("case: arg == \"--stop-after-state\"")
			role = "flag"
			state.HasStopAfter = true
			expectStopAfter = true
		case strings.HasPrefix(arg, "--stop-after-state="):
			observe.GlobalTrace("case: strings.HasPrefix(arg, \"--stop-after-state=\")")
			role = "flag"
			state.HasStopAfter = true
		case arg == "--prompt":
			observe.GlobalTrace("case: arg == \"--prompt\"")
			role = "flag"
			state.HasPrompt = true
			inPrompt = true
		case strings.HasPrefix(arg, "--prompt="):
			observe.GlobalTrace("case: strings.HasPrefix(arg, \"--prompt=\")")
			role = "flag"
			state.HasPrompt = true
		case strings.HasPrefix(arg, "--"):
			observe.GlobalTrace("case: strings.HasPrefix(arg, \"--\")")
			role = "flag"
		case !state.HasDefinition:
			observe.GlobalTrace("case: !state.HasDefinition")
			role = "definition"
			state.HasDefinition = true
		}
		if i == currentIndex {
			observe.GlobalTrace("if: i == currentIndex")
			state.CurrentRole = role
		}
	}
	if state.CurrentRole == "" && currentIndex >= 0 {
		observe.GlobalTrace("if: state.CurrentRole == \"\" && currentIndex >= 0")
		state.CurrentRole = "prompt"
	}
	observe.GlobalTrace("return: state")
	return state
}

func (s OrchestrateCompletionState) FlagCandidates() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !s.HasPersonaDir {
		observe.GlobalTrace("if: !s.HasPersonaDir")
		observe.GlobalTrace("return: []string{\"--persona-dir\"}")
		return []string{"--persona-dir"}
	}
	if !s.HasPrompt {
		observe.GlobalTrace("if: !s.HasPrompt")
		flags := []string{}
		if !s.HasStartAt {
			flags = append(flags, "--start-at-state")
		}
		if !s.HasStopAfter {
			flags = append(flags, "--stop-after-state")
		}
		flags = append(flags, "--prompt")
		observe.GlobalTrace("return: flags")
		return flags
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func OrchestrateFlagDetail(flag string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch flag {
	case "--persona-dir":
		observe.GlobalTrace("case: \"--persona-dir\"")
		return "persona directory"
	case "--start-at-state":
		observe.GlobalTrace("case: \"--start-at-state\"")
		return "orchestration state to start from"
	case "--stop-after-state":
		observe.GlobalTrace("case: \"--stop-after-state\"")
		return "orchestration state to stop after"
	case "--prompt":
		observe.GlobalTrace("case: \"--prompt\"")
		return "task prompt"
	default:
		observe.GlobalTrace("default")
		return "flag"
	}
}
