package slash

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const orchestrateUsage = "Usage: /orchestrate <orchestration.yaml> --persona-dir <persona-dir> --prompt <task prompt>"

func handleOrchestrate(_ context.Context, args string, _ Deps) (Result, error) {
	req, err := parseOrchestrateArgs(args)
	if err != nil {
		return Result{DisplayText: err.Error() + "\n\n" + orchestrateUsage}, nil
	}
	if info, err := os.Stat(req.DefinitionPath); err != nil {
		return Result{DisplayText: fmt.Sprintf("orchestration YAML not found: %s\n\n%s", req.DefinitionPath, orchestrateUsage)}, nil
	} else if info.IsDir() {
		return Result{DisplayText: fmt.Sprintf("orchestration YAML is a directory: %s\n\n%s", req.DefinitionPath, orchestrateUsage)}, nil
	}
	if info, err := os.Stat(req.PersonaDir); err != nil {
		return Result{DisplayText: fmt.Sprintf("persona directory not found: %s\n\n%s", req.PersonaDir, orchestrateUsage)}, nil
	} else if !info.IsDir() {
		return Result{DisplayText: fmt.Sprintf("persona path is not a directory: %s\n\n%s", req.PersonaDir, orchestrateUsage)}, nil
	}
	return Result{Orchestrate: &req}, nil
}

func parseOrchestrateArgs(args string) (OrchestrationRequest, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return OrchestrationRequest{}, fmt.Errorf("missing orchestration YAML path")
	}

	req := OrchestrationRequest{DefinitionPath: fields[0]}
	var trailing []string
	for i := 1; i < len(fields); i++ {
		field := fields[i]
		switch {
		case field == "--persona-dir":
			i++
			if i >= len(fields) {
				return OrchestrationRequest{}, fmt.Errorf("missing value for --persona-dir")
			}
			req.PersonaDir = fields[i]
		case strings.HasPrefix(field, "--persona-dir="):
			req.PersonaDir = strings.TrimPrefix(field, "--persona-dir=")
		case field == "--prompt":
			i++
			if i >= len(fields) {
				return OrchestrationRequest{}, fmt.Errorf("missing value for --prompt")
			}
			req.Prompt = strings.Join(fields[i:], " ")
			i = len(fields)
		case strings.HasPrefix(field, "--prompt="):
			req.Prompt = strings.TrimPrefix(field, "--prompt=")
		case strings.HasPrefix(field, "--"):
			return OrchestrationRequest{}, fmt.Errorf("unknown flag %s", field)
		default:
			trailing = append(trailing, field)
		}
	}
	if req.Prompt == "" && len(trailing) > 0 {
		req.Prompt = strings.Join(trailing, " ")
	}
	if strings.TrimSpace(req.PersonaDir) == "" {
		return OrchestrationRequest{}, fmt.Errorf("missing --persona-dir")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return OrchestrationRequest{}, fmt.Errorf("missing task prompt")
	}
	return req, nil
}

type OrchestrateCompletionState struct {
	HasDefinition bool
	HasPersonaDir bool
	HasPrompt     bool
	CurrentRole   string
}

func ParseOrchestrateCompletionState(args []string, currentIndex int) OrchestrateCompletionState {
	state := OrchestrateCompletionState{}
	expectPersona := false
	inPrompt := false
	for i, arg := range args {
		role := "prompt"
		switch {
		case inPrompt:
			role = "prompt"
		case expectPersona:
			role = "persona"
			expectPersona = false
		case arg == "--persona-dir":
			role = "flag"
			state.HasPersonaDir = true
			expectPersona = true
		case strings.HasPrefix(arg, "--persona-dir="):
			role = "flag"
			state.HasPersonaDir = true
		case arg == "--prompt":
			role = "flag"
			state.HasPrompt = true
			inPrompt = true
		case strings.HasPrefix(arg, "--prompt="):
			role = "flag"
			state.HasPrompt = true
		case strings.HasPrefix(arg, "--"):
			role = "flag"
		case !state.HasDefinition:
			role = "definition"
			state.HasDefinition = true
		}
		if i == currentIndex {
			state.CurrentRole = role
		}
	}
	if state.CurrentRole == "" && currentIndex >= 0 {
		state.CurrentRole = "prompt"
	}
	return state
}

func (s OrchestrateCompletionState) FlagCandidates() []string {
	if !s.HasPersonaDir {
		return []string{"--persona-dir"}
	}
	if !s.HasPrompt {
		return []string{"--prompt"}
	}
	return nil
}

func OrchestrateFlagDetail(flag string) string {
	switch flag {
	case "--persona-dir":
		return "persona directory"
	case "--prompt":
		return "task prompt"
	default:
		return "flag"
	}
}
