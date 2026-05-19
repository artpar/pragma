package googlevertex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// Provider implements provider.Provider for Google Cloud Vertex AI custom endpoints (e.g., Gemma 4).
type Provider struct {
	projectID  string
	location   string
	endpointID string
	domain     string
	httpClient *http.Client
	bus        *observe.EventBus
}

// New creates a new Vertex AI provider.
func New(projectID, location, endpointID, domain string, bus *observe.EventBus) (*Provider, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if domain == "" {
		observe.GlobalTrace("if: domain == \"\"")

		domain = fmt.Sprintf("%s.%s-%s.prediction.vertexai.goog", endpointID, location, projectID)
	}
	observe.GlobalTrace("return: &Provider{\n\tprojectID:\tprojectID,\n\tlocation:\tlocation,\n\tendpointID:\tendpointI...")

	return &Provider{
		projectID:  projectID,
		location:   location,
		endpointID: endpointID,
		domain:     domain,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		bus:        bus,
	}, nil
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"google-vertex\"")
	return "google-vertex"
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages:
		observe.GlobalTrace("case: provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages")
		return false
	default:
		observe.GlobalTrace("default")
		return false
	}
}

// getAuthToken executes gcloud auth print-access-token to get a bearer token.
func (p *Provider) getAuthToken(ctx context.Context) (string, error) {
	observe.TraceCtx(ctx, "googlevertex", "Provider.getAuthToken", "enter")
	defer observe.TraceCtx(ctx, "googlevertex", "Provider.getAuthToken", "exit")
	cmd := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token")
	out, err := cmd.Output()
	if err != nil {
		observe.TraceCtx(ctx, "googlevertex", "Provider.getAuthToken", "if: err != nil")
		observe.TraceCtx(ctx, "googlevertex", "Provider.getAuthToken", "return: \"\", fmt.Errorf(\"gcloud auth print-access-token: %w\", err)")
		return "", fmt.Errorf("gcloud auth print-access-token: %w", err)
	}
	observe.TraceCtx(ctx, "googlevertex", "Provider.getAuthToken", "return: strings.TrimSpace(string(out)), nil")
	return strings.TrimSpace(string(out)), nil
}

// Complete sends a request to the Vertex AI predict endpoint.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "exit")
	token, err := p.getAuthToken(ctx)
	if err != nil {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}

	prompt := p.buildPrompt(params)

	payload := map[string]any{
		"instances": []map[string]any{
			{
				"prompt": prompt,
			},
		},
		"parameters": map[string]any{
			"max_tokens": params.MaxTokens,
		},
	}
	if params.Temperature != nil {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: params.Temperature != nil")
		payload["parameters"].(map[string]any)["temperature"] = *params.Temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"marshal payload: %w\", err)")
		return model.Response{}, fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/endpoints/%s:predict",
		p.domain, p.projectID, p.location, p.endpointID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"create request: %w\", err)")
		return model.Response{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"do request: %w\", err)")
		return model.Response{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: resp.StatusCode != http.StatusOK")
		b, _ := io.ReadAll(resp.Body)
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"vertex api error (status %d): %s\", resp.StatusC...")
		return model.Response{}, fmt.Errorf("vertex api error (status %d): %s", resp.StatusCode, string(b))
	}

	var result struct {
		Predictions []any `json:"predictions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"decode response: %w\", err)")
		return model.Response{}, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Predictions) == 0 {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: len(result.Predictions) == 0")
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"no predictions returned\")")
		return model.Response{}, fmt.Errorf("no predictions returned")
	}

	pred := result.Predictions[0]
	var text string
	switch v := pred.(type) {
	case string:
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "typecase: string")
		text = v
	case map[string]any:
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "typecase: map[string]any")
		if t, ok := v["content"].(string); ok {
			text = t
		} else if t, ok := v["prompt"].(string); ok {

			text = t
		}
	}

	if text == "" {
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "if: text == \"\"")
		observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"could not extract text from prediction: %v\", pred)")
		return model.Response{}, fmt.Errorf("could not extract text from prediction: %v", pred)
	}
	observe.TraceCtx(ctx, "googlevertex", "Provider.Complete", "return: model.Response{\n\tModel:\t\tp.endpointID,\n\tContent:\t[]model.ContentPart{model.Te...")

	return model.Response{
		Model:      p.endpointID,
		Content:    []model.ContentPart{model.TextPart{Text: text}},
		StopReason: model.StopEndTurn,
	}, nil
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "googlevertex", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "googlevertex", "Provider.Stream", "exit")

	ch := make(chan provider.StreamChunk, 1)

	go func() {
		defer close(ch)
		resp, err := p.Complete(ctx, params)
		if err != nil {
			observe.TraceCtx(ctx, "googlevertex", "Provider.Stream", "if: err != nil")
			ch <- provider.StreamChunk{Error: err}
			return
		}

		for _, part := range resp.Content {
			observe.TraceCtx(ctx, "googlevertex", "Provider.Stream", "range resp.Content")
			if tp, ok := part.(model.TextPart); ok {
				observe.TraceCtx(ctx, "googlevertex", "Provider.Stream", "if: ok")
				ch <- provider.StreamChunk{TextDelta: tp.Text}
			}
		}

		ch <- provider.StreamChunk{
			Done: &provider.StreamDone{
				StopReason: resp.StopReason,
				Usage:      resp.Usage,
				Model:      resp.Model,
			},
		}
	}()
	observe.TraceCtx(ctx, "googlevertex", "Provider.Stream", "return: ch, nil")

	return ch, nil
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: model.Pricing{}, false")
	return model.Pricing{}, false
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: 8192, true")
	return 8192, true
}

func (p *Provider) buildPrompt(params provider.RequestParams) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var sb strings.Builder

	for _, block := range params.System.Blocks {
		observe.GlobalTrace("range params.System.Blocks")
		sb.WriteString(block.Text + "\n\n")
	}

	for _, msg := range params.Messages {
		observe.GlobalTrace("range params.Messages")
		role := "user"
		if msg.Role == model.RoleAssistant {
			observe.GlobalTrace("if: msg.Role == model.RoleAssistant")
			role = "model"
		}

		sb.WriteString(fmt.Sprintf("<start_of_turn>%s\n", role))
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tp, ok := part.(model.TextPart); ok {
				observe.GlobalTrace("if: ok")
				sb.WriteString(tp.Text)
			}
		}
		sb.WriteString("\n<end_of_turn>\n")
	}

	sb.WriteString("<start_of_turn>model\n")
	observe.GlobalTrace("return: sb.String()")
	return sb.String()
}
