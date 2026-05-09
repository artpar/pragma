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
	projectID   string
	location    string
	endpointID  string
	domain      string
	httpClient  *http.Client
	bus         *observe.EventBus
}

// New creates a new Vertex AI provider.
func New(projectID, location, endpointID, domain string, bus *observe.EventBus) (*Provider, error) {
	if domain == "" {
		// Fallback: construct the dedicated domain based on the observed pattern:
		// {endpointID}.{location}-{projectID}.prediction.vertexai.goog
		domain = fmt.Sprintf("%s.%s-%s.prediction.vertexai.goog", endpointID, location, projectID)
	}
	
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
	return "google-vertex"
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	// Custom endpoints usually support basic completion. 
	// Streaming and Tool Use depend on the specific model deployment.
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages:
		return false
	default:
		return false
	}
}

// getAuthToken executes gcloud auth print-access-token to get a bearer token.
func (p *Provider) getAuthToken(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gcloud auth print-access-token: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Complete sends a request to the Vertex AI predict endpoint.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	token, err := p.getAuthToken(ctx)
	if err != nil {
		return model.Response{}, err
	}

	// Construct the prompt from messages and system prompt
	prompt := p.buildPrompt(params)

	// Construct the payload for Gemma / Vertex Predict API
	// The standard format for these endpoints is usually: {"instances": [{"prompt": "..."}]}
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
		payload["parameters"].(map[string]any)["temperature"] = *params.Temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return model.Response{}, fmt.Errorf("marshal payload: %w", err)
	}

	// URL: https://{domain}/v1/projects/{project}/locations/{location}/endpoints/{endpoint}:predict
	url := fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/endpoints/%s:predict", 
		p.domain, p.projectID, p.location, p.endpointID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return model.Response{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return model.Response{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return model.Response{}, fmt.Errorf("vertex api error (status %d): %s", resp.StatusCode, string(b))
	}

	var result struct {
		Predictions []any `json:"predictions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return model.Response{}, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Predictions) == 0 {
		return model.Response{}, fmt.Errorf("no predictions returned")
	}

	// Gemma predictions are often returned as strings or objects with a 'content' field
	pred := result.Predictions[0]
	var text string
	switch v := pred.(type) {
	case string:
		text = v
	case map[string]any:
		if t, ok := v["content"].(string); ok {
			text = t
		} else if t, ok := v["prompt"].(string); ok {
			// Some versions return the prompt back or the result in a different field
			text = t
		}
	}

	if text == "" {
		return model.Response{}, fmt.Errorf("could not extract text from prediction: %v", pred)
	}

	return model.Response{
		Model:    p.endpointID,
		Content:  []model.ContentPart{model.TextPart{Text: text}},
		StopReason: model.StopEndTurn,
	}, nil
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	// Fallback: since the Vertex Predict API is unary, we'll simulate a stream 
	// by calling Complete and emitting the result as a single chunk.
	ch := make(chan provider.StreamChunk, 1)
	
	go func() {
		defer close(ch)
		resp, err := p.Complete(ctx, params)
		if err != nil {
			ch <- provider.StreamChunk{Error: err}
			return
		}

		for _, part := range resp.Content {
			if tp, ok := part.(model.TextPart); ok {
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

	return ch, nil
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	return model.Pricing{}, false
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	return 8192, true // Default for many Gemma deployments, can be updated
}

func (p *Provider) buildPrompt(params provider.RequestParams) string {
	var sb strings.Builder
	
	// Add system prompt
	for _, block := range params.System.Blocks {
		sb.WriteString(block.Text + "\n\n")
	}

	// Simple chat formatting for Gemma: <start_of_turn>user\n...\n<end_of_turn>\n<start_of_turn>model\n
	for _, msg := range params.Messages {
		role := "user"
		if msg.Role == model.RoleAssistant {
			role = "model"
		}
		
		sb.WriteString(fmt.Sprintf("<start_of_turn>%s\n", role))
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				sb.WriteString(tp.Text)
			}
		}
		sb.WriteString("\n<end_of_turn>\n")
	}
	
	sb.WriteString("<start_of_turn>model\n")
	return sb.String()
}
