package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/remote"
)

// RemoteTriggerClient executes Anthropic CCR trigger operations.
type RemoteTriggerClient struct {
	HTTPClient  *http.Client
	BaseURL     string
	TokenSource func() (string, error)
	OrgUUID     func() (string, error)
}

func NewRemoteTriggerClient(httpClient *http.Client, baseURL string, tokenSource func() (string, error), orgUUID func() (string, error)) *RemoteTriggerClient {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if httpClient == nil {
		observe.GlobalTrace("if: httpClient == nil")
		httpClient = http.DefaultClient
	}
	observe.GlobalTrace("return: &RemoteTriggerClient{...}")
	return &RemoteTriggerClient{
		HTTPClient:  httpClient,
		BaseURL:     baseURL,
		TokenSource: tokenSource,
		OrgUUID:     orgUUID,
	}
}

func (c *RemoteTriggerClient) Execute(ctx context.Context, triggerReq remote.Request) (remote.Response, error) {
	observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "enter")
	defer observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "exit")
	method, urlPath, body, err := c.requestParts(triggerReq)
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "if: err != nil")
		return remote.Response{}, err
	}

	token, err := c.TokenSource()
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "if: err != nil")
		return remote.Response{}, fmt.Errorf("get auth token: %w", err)
	}
	orgUUID, err := c.OrgUUID()
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "if: err != nil")
		return remote.Response{}, fmt.Errorf("get org UUID: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.BaseURL+urlPath, body)
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "if: err != nil")
		return remote.Response{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-beta", "ccr-triggers-2026-01-30")
	httpReq.Header.Set("x-organization-uuid", orgUUID)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "if: err != nil")
		return remote.Response{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "if: err != nil")
		return remote.Response{}, fmt.Errorf("read response body: %w", err)
	}
	observe.TraceCtx(ctx, "anthropic", "RemoteTriggerClient.Execute", "return: remote.Response{...}, nil")
	return remote.Response{StatusCode: resp.StatusCode, Body: respBody}, nil
}

func (c *RemoteTriggerClient) requestParts(triggerReq remote.Request) (string, string, io.Reader, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch triggerReq.Action {
	case remote.ActionList:
		observe.GlobalTrace("case: remote.ActionList")
		return http.MethodGet, "/v1/code/triggers", nil, nil
	case remote.ActionGet:
		observe.GlobalTrace("case: remote.ActionGet")
		return http.MethodGet, "/v1/code/triggers/" + triggerReq.TriggerID, nil, nil
	case remote.ActionCreate:
		observe.GlobalTrace("case: remote.ActionCreate")
		return http.MethodPost, "/v1/code/triggers", strings.NewReader(string(triggerReq.Body)), nil
	case remote.ActionUpdate:
		observe.GlobalTrace("case: remote.ActionUpdate")
		return http.MethodPost, "/v1/code/triggers/" + triggerReq.TriggerID, strings.NewReader(string(triggerReq.Body)), nil
	case remote.ActionRun:
		observe.GlobalTrace("case: remote.ActionRun")
		return http.MethodPost, "/v1/code/triggers/" + triggerReq.TriggerID + "/run", nil, nil
	default:
		observe.GlobalTrace("default")
		return "", "", nil, fmt.Errorf("unknown action: %s", triggerReq.Action)
	}
}
