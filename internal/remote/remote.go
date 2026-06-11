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
			observe.GlobalTrace("return: fmt.Errorf(\"trigger_id required for get\")")
			return fmt.Errorf("trigger_id required for get")
		}
		return nil
	case ActionCreate:
		observe.GlobalTrace("case: ActionCreate")
		if len(req.Body) == 0 {
			observe.GlobalTrace("if: len(req.Body) == 0")
			observe.GlobalTrace("return: fmt.Errorf(\"body required for create\")")
			return fmt.Errorf("body required for create")
		}
		return validateBodyObject(req.Action, req.Body)
	case ActionUpdate:
		observe.GlobalTrace("case: ActionUpdate")
		if req.TriggerID == "" {
			observe.GlobalTrace("if: req.TriggerID == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"trigger_id required for update\")")
			return fmt.Errorf("trigger_id required for update")
		}
		if len(req.Body) == 0 {
			observe.GlobalTrace("if: len(req.Body) == 0")
			observe.GlobalTrace("return: fmt.Errorf(\"body required for update\")")
			return fmt.Errorf("body required for update")
		}
		return validateBodyObject(req.Action, req.Body)
	case ActionRun:
		observe.GlobalTrace("case: ActionRun")
		if req.TriggerID == "" {
			observe.GlobalTrace("if: req.TriggerID == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"trigger_id required for run\")")
			return fmt.Errorf("trigger_id required for run")
		}
		return nil
	default:
		observe.GlobalTrace("default")
		return fmt.Errorf("unknown action: %s", req.Action)
	}
}

func validateBodyObject(action Action, body json.RawMessage) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"body for %s must be a JSON object: %w\", action, err)")
		return fmt.Errorf("body for %s must be a JSON object: %w", action, err)
	}
	if obj == nil {
		observe.GlobalTrace("if: obj == nil")
		observe.GlobalTrace("return: fmt.Errorf(\"body for %s must be a JSON object\", action)")
		return fmt.Errorf("body for %s must be a JSON object", action)
	}
	observe.GlobalTrace("return: nil")
	return nil
}
