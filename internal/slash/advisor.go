package slash

import (
	"context"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/observe"
)

func handleAdvisor(_ context.Context, args string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	arg := strings.TrimSpace(strings.ToLower(args))

	if arg == "" {
		observe.GlobalTrace("if: arg == \"\"")
		snap := deps.Store.Snapshot()
		if snap.AdvisorModel == "" {
			observe.GlobalTrace("if: snap.AdvisorModel == \"\"")
			observe.GlobalTrace("return: Result{DisplayText: not set}, nil")
			observe.GlobalTrace("return: Result{DisplayText: \"Advisor: not set\\nUse \\\"/advisor <model>\\\" to enable (e....")
			return Result{DisplayText: "Advisor: not set\nUse \"/advisor <model>\" to enable (e.g. \"/advisor opus\")."}, nil
		}
		observe.GlobalTrace("return: Result{DisplayText: current advisor}, nil")
		observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"Advisor: %s\\nUse \\\"/advisor unset\\\" to disab...")
		return Result{DisplayText: fmt.Sprintf("Advisor: %s\nUse \"/advisor unset\" to disable or \"/advisor <model>\" to change.", snap.AdvisorModel)}, nil
	}

	if arg == "unset" || arg == "off" {
		observe.GlobalTrace("if: arg == unset || off")
		prev := deps.Store.Snapshot().AdvisorModel
		deps.Store.Update(func(s *app.AppState) {
			s.AdvisorModel = ""
		})
		if prev != "" {
			observe.GlobalTrace("if: prev != \"\"")
			observe.GlobalTrace("return: Result{DisplayText: advisor disabled}, nil")
			observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"Advisor disabled (was %s).\", prev)}, nil")
			return Result{DisplayText: fmt.Sprintf("Advisor disabled (was %s).", prev)}, nil
		}
		observe.GlobalTrace("return: Result{DisplayText: advisor already unset}, nil")
		observe.GlobalTrace("return: Result{DisplayText: \"Advisor already unset.\"}, nil")
		return Result{DisplayText: "Advisor already unset."}, nil
	}

	deps.Store.Update(func(s *app.AppState) {
		s.AdvisorModel = arg
	})
	observe.GlobalTrace("return: Result{DisplayText: advisor set}, nil")
	observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"Advisor set to %s.\", arg)}, nil")
	return Result{DisplayText: fmt.Sprintf("Advisor set to %s.", arg)}, nil
}
