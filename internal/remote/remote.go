package remote

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/observe"
)

// Action is a provider-neutral remote trigger operation.
type Action string

const (
	ActionList   Action = "list"
	ActionGet    Action = "get"
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionRun    Action = "run"
)

// Request describes a remote trigger operation without provider wire details.
type Request struct {
	Action    Action
	TriggerID string
	Body      json.RawMessage
}

// Response contains the provider adapter response for model-facing display.
type Response struct {
	StatusCode int
	Body       []byte
}

// Client executes provider-specific remote trigger operations.
type Client interface {
	Execute(ctx context.Context, req Request) (Response, error)
}

// Service owns runtime-visible remote trigger operations.
type Service struct {
	Client Client
}

func NewService(client Client) *Service {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Service{Client: client}")
	return &Service{Client: client}
}

func (s *Service) Execute(ctx context.Context, req Request) (Response, error) {
	observe.TraceCtx(ctx, "remote", "Service.Execute", "enter")
	defer observe.TraceCtx(ctx, "remote", "Service.Execute", "exit")
	if s == nil || s.Client == nil {
		observe.TraceCtx(ctx, "remote", "Service.Execute", "return: Response{}, fmt.Errorf(\"remote trigger service is not configured\")")
		return Response{}, fmt.Errorf("remote trigger service is not configured")
	}
	if err := Validate(req); err != nil {
		observe.TraceCtx(ctx, "remote", "Service.Execute", "if: err != nil")
		observe.TraceCtx(ctx, "remote", "Service.Execute", "return: Response{}, err")
		return Response{}, err
	}
	observe.TraceCtx(ctx, "remote", "Service.Execute", "return: s.Client.Execute(ctx, req)")
	return s.Client.Execute(ctx, req)
}

func Validate(req Request) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch req.Action {
	case ActionList:
		observe.GlobalTrace("case: ActionList")
		return nil
	case ActionGet:
		observe.GlobalTrace("case: ActionGet")
		if req.TriggerID == "" {
			observe.GlobalTrace("if: req.TriggerID == \"\"")
			return fmt.Errorf("trigger_id required for get")
		}
		return nil
	case ActionCreate:
		observe.GlobalTrace("case: ActionCreate")
		if len(req.Body) == 0 {
			observe.GlobalTrace("if: len(req.Body) == 0")
			return fmt.Errorf("body required for create")
		}
		return nil
	case ActionUpdate:
		observe.GlobalTrace("case: ActionUpdate")
		if req.TriggerID == "" {
			observe.GlobalTrace("if: req.TriggerID == \"\"")
			return fmt.Errorf("trigger_id required for update")
		}
		if len(req.Body) == 0 {
			observe.GlobalTrace("if: len(req.Body) == 0")
			return fmt.Errorf("body required for update")
		}
		return nil
	case ActionRun:
		observe.GlobalTrace("case: ActionRun")
		if req.TriggerID == "" {
			observe.GlobalTrace("if: req.TriggerID == \"\"")
			return fmt.Errorf("trigger_id required for run")
		}
		return nil
	default:
		observe.GlobalTrace("default")
		return fmt.Errorf("unknown action: %s", req.Action)
	}
}
