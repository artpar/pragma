package slash

import (
	"context"

	"github.com/artpar/pragma/internal/observe"
)

func handleTeams(_ context.Context, _ string, _ Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: Result{ShowTeamsDialog: true}, nil")
	return Result{ShowTeamsDialog: true}, nil
}
