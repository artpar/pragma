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
