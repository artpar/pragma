package toolremote

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/remote"
)

type testState struct{ cwd string }

func (s testState) WorkDir() string { return s.cwd }

type alwaysAllow struct{}

func (alwaysAllow) Check(_ context.Context, _, _ string) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}

type fakeTriggerService struct {
	gotReq remote.Request
	resp   remote.Response
	err    error
}

func (s *fakeTriggerService) Execute(_ context.Context, req remote.Request) (remote.Response, error) {
	s.gotReq = req
	if s.err != nil {
		return remote.Response{}, s.err
	}
	return s.resp, nil
}

func TestRemoteTriggerActions(t *testing.T) {
	tests := []struct {
		name    string
		input   remoteInput
		wantReq remote.Request
		wantErr bool
	}{
		{
			name:    "list",
			input:   remoteInput{Action: "list"},
			wantReq: remote.Request{Action: remote.ActionList},
		},
		{
			name:    "get",
			input:   remoteInput{Action: "get", TriggerID: "tr-123"},
			wantReq: remote.Request{Action: remote.ActionGet, TriggerID: "tr-123"},
		},
		{
			name:    "create",
			input:   remoteInput{Action: "create", Body: json.RawMessage(`{"name":"test"}`)},
			wantReq: remote.Request{Action: remote.ActionCreate, Body: json.RawMessage(`{"name":"test"}`)},
		},
		{
			name:    "update",
			input:   remoteInput{Action: "update", TriggerID: "tr-123", Body: json.RawMessage(`{"name":"updated"}`)},
			wantReq: remote.Request{Action: remote.ActionUpdate, TriggerID: "tr-123", Body: json.RawMessage(`{"name":"updated"}`)},
		},
		{
			name:    "run",
			input:   remoteInput{Action: "run", TriggerID: "tr-123"},
			wantReq: remote.Request{Action: remote.ActionRun, TriggerID: "tr-123"},
		},
		{
			name:    "get missing trigger_id",
			input:   remoteInput{Action: "get"},
			wantErr: true,
		},
		{
			name:    "create missing body",
			input:   remoteInput{Action: "create"},
			wantErr: true,
		},
		{
			name:    "unknown action",
			input:   remoteInput{Action: "delete"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeTriggerService{resp: remote.Response{StatusCode: 200, Body: []byte(`{"ok":true}`)}}
			tl := &Tool{Service: remote.NewService(svc)}
			inputJSON, _ := json.Marshal(tt.input)

			result, err := tl.Invoke(context.Background(), inputJSON, testState{cwd: "/tmp"})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if svc.gotReq.Action != tt.wantReq.Action {
				t.Errorf("action: got %s, want %s", svc.gotReq.Action, tt.wantReq.Action)
			}
			if svc.gotReq.TriggerID != tt.wantReq.TriggerID {
				t.Errorf("trigger_id: got %s, want %s", svc.gotReq.TriggerID, tt.wantReq.TriggerID)
			}
			if string(svc.gotReq.Body) != string(tt.wantReq.Body) {
				t.Errorf("body: got %s, want %s", svc.gotReq.Body, tt.wantReq.Body)
			}
			if !strings.Contains(result.Content, "HTTP 200") {
				t.Errorf("expected HTTP 200 in result, got: %s", result.Content)
			}
		})
	}
}

func TestRemoteTriggerAuthError(t *testing.T) {
	tl := &Tool{Service: remote.NewService(&fakeTriggerService{err: context.Canceled})}

	input, _ := json.Marshal(remoteInput{Action: "list"})
	_, err := tl.Invoke(context.Background(), input, testState{cwd: "/tmp"})
	if err == nil {
		t.Fatal("expected error from token source")
	}
}

func TestRemoteTriggerHTTPError(t *testing.T) {
	tl := &Tool{Service: remote.NewService(&fakeTriggerService{resp: remote.Response{StatusCode: 500, Body: []byte(`{"error":"internal server error"}`)}})}
	input, _ := json.Marshal(remoteInput{Action: "list"})

	result, err := tl.Invoke(context.Background(), input, testState{cwd: "/tmp"})
	if err != nil {
		t.Fatalf("HTTP errors should be returned as content, not Go errors: %v", err)
	}
	if !strings.Contains(result.Content, "HTTP 500") {
		t.Errorf("expected HTTP 500 in result, got: %s", result.Content)
	}
}
