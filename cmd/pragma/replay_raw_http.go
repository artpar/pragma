package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/rawcapture"
)

type rawHTTPRequestMeta struct {
	Sequence  int    `json:"sequence,omitempty"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	StartedAt string `json:"started_at,omitempty"`
}

type rawHTTPChatRequest struct {
	Model       string               `json:"model"`
	MaxTokens   int                  `json:"max_tokens,omitempty"`
	Temperature *float64             `json:"temperature,omitempty"`
	Messages    []rawHTTPChatMessage `json:"messages"`
	Tools       []json.RawMessage    `json:"tools,omitempty"`
}

type rawHTTPChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

var createRawHTTPReplayProvider = cli.CreateProvider

func replayRawHTTPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw-http <capture-dir>",
		Short: "Replay a captured raw HTTP LLM request",
		Args:  cobra.ExactArgs(1),
		RunE:  replayRawHTTPRun,
	}
	cmd.Flags().String("base-url", "", "override scheme/host/base path while preserving captured endpoint")
	cmd.Flags().String("api-key-env", "OPENROUTER_API_KEY", "environment variable containing the replay API key")
	cmd.Flags().String("format", "raw", "output format: raw, pretty, content")
	cmd.Flags().Bool("pretty", false, "print a human-readable response report")
	cmd.Flags().String("out", "", "write response body to file instead of stdout")
	cmd.Flags().Duration("timeout", 10*time.Minute, "HTTP request timeout for the replay")
	cmd.AddCommand(replayRawHTTPDumpCmd())
	cmd.AddCommand(replayRawHTTPAuditCmd())
	return cmd
}

func replayRawHTTPDumpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump <run-dir-or-capture-dir>",
		Short: "Dump captured raw HTTP turns into inspection files",
		Args:  cobra.ExactArgs(1),
		RunE:  replayRawHTTPDumpRun,
	}
	cmd.Flags().String("out", "", "output directory (default: <input>/turn-payloads)")
	cmd.Flags().Bool("overwrite", false, "replace output directory if it exists")
	return cmd
}

func replayRawHTTPAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit <dir>",
		Short: "Audit raw HTTP replay case directories for response evidence",
		Args:  cobra.ExactArgs(1),
		RunE:  replayRawHTTPAuditRun,
	}
	cmd.Flags().Bool("require-responses", false, "return an error if any request lacks usable response evidence")
	return cmd
}

func replayRawHTTPRun(cmd *cobra.Command, args []string) error {
	dir := args[0]
	body, err := os.ReadFile(filepath.Join(dir, "request.json"))
	if err != nil {
		return fmt.Errorf("read request.json: %w", err)
	}
	format := rawHTTPReplayFormatFromFlags(cmd)
	if !isRawHTTPReplayFormat(format) {
		return fmt.Errorf("unsupported raw HTTP replay format %q; expected raw, pretty, or content", format)
	}
	var meta rawHTTPRequestMeta
	if err := readJSONFile(filepath.Join(dir, "request.meta.json"), &meta); err != nil {
		return fmt.Errorf("read request.meta.json: %w", err)
	}
	if meta.Method == "" {
		meta.Method = http.MethodPost
	}
	defaultProvider := inferRawHTTPReplayProvider(meta.URL)
	capturedModel, _ := rawHTTPRequestModel(body)
	resolved, err := cli.ResolveProviderConfig(cmd, cli.ProviderResolutionOptions{
		DefaultProvider: defaultProvider,
		DefaultModel:    capturedModel,
	})
	if err != nil {
		return err
	}
	if resolved.Provider == "google" {
		return replayRawHTTPGoogle(cmd, body, resolved, capturedModel, format)
	}
	if !isRawHTTPReplayProvider(resolved.Provider) {
		return fmt.Errorf("raw HTTP replay supports OpenAI-compatible providers or google; got %q", resolved.Provider)
	}
	model := resolved.Model
	if resolved.ProviderExplicit && !resolved.ModelExplicit {
		model = cli.DefaultModelFor(resolved.Provider)
	}
	if model != "" && model != capturedModel {
		body, err = rewriteRawHTTPRequestModel(body, model)
		if err != nil {
			return err
		}
	}

	targetURL := meta.URL
	if override, _ := cmd.Flags().GetString("base-url"); override != "" {
		targetURL, err = overrideCapturedURL(meta.URL, override)
		if err != nil {
			return err
		}
	} else if resolved.BaseURL != "" {
		targetURL, err = overrideCapturedURL(meta.URL, resolved.BaseURL)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(cmd.Context(), meta.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	var headers map[string][]string
	_ = readJSONFile(filepath.Join(dir, "request.headers.json"), &headers)
	for k, values := range headers {
		if skipReplayHeader(k) {
			continue
		}
		for _, v := range values {
			if v != "<redacted>" {
				req.Header.Add(k, v)
			}
		}
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	apiKey := resolved.APIKey
	apiKeyEnv, _ := cmd.Flags().GetString("api-key-env")
	if envKey := strings.TrimSpace(os.Getenv(apiKeyEnv)); envKey != "" && !cmd.Flags().Changed("api-key") {
		apiKey = envKey
	}
	if apiKey == "" {
		return fmt.Errorf("API key required for %s: set --api-key, %s, or ~/.pragma/credentials.yml", resolved.Provider, apiKeyEnv)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	timeout, _ := cmd.Flags().GetDuration("timeout")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Fprintf(cmd.ErrOrStderr(), "HTTP %s\n", resp.Status)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	rendered, err := renderRawHTTPReplayOutput(resp.Status, respBody, format)
	if err != nil {
		return err
	}
	if format == "pretty" {
		requestReport, err := renderRawHTTPPrettyRequest(body)
		if err != nil {
			return err
		}
		rendered = appendPrettySections(requestReport, rendered)
	}
	outPath, _ := cmd.Flags().GetString("out")
	if outPath != "" {
		if err := os.WriteFile(outPath, rendered, 0o644); err != nil {
			return err
		}
	} else {
		_, _ = cmd.OutOrStdout().Write(rendered)
		if len(rendered) > 0 && rendered[len(rendered)-1] != '\n' {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	if format == "raw" {
		printRawHTTPSummary(cmd.ErrOrStderr(), respBody)
	}
	return nil
}

func isRawHTTPReplayFormat(format string) bool {
	switch format {
	case "", "raw", "pretty", "content":
		return true
	default:
		return false
	}
}

func replayRawHTTPGoogle(cmd *cobra.Command, body []byte, resolved cli.ResolvedProviderConfig, capturedModel, format string) error {
	params, err := rawHTTPChatRequestToParams(body)
	if err != nil {
		return err
	}
	params.Model = resolved.Model
	if !resolved.ModelExplicit {
		params.Model = cli.DefaultModelFor("google")
	}
	if params.Model == "" || params.Model == capturedModel {
		params.Model = cli.DefaultModelFor("google")
	}

	prov, err := createRawHTTPReplayProvider(resolved.Config, observe.NewEventBus(1024))
	if err != nil {
		return err
	}
	resp, err := prov.Complete(cmd.Context(), params)
	if err != nil {
		return err
	}
	rendered, err := renderNativeReplayOutput(resolved.Provider, resp, format)
	if err != nil {
		return err
	}
	if format == "pretty" {
		requestReport, err := renderRawHTTPPrettyRequest(body)
		if err != nil {
			return err
		}
		rendered = appendPrettySections(requestReport, rendered)
	}
	outPath, _ := cmd.Flags().GetString("out")
	if outPath != "" {
		return os.WriteFile(outPath, rendered, 0o644)
	}
	_, _ = cmd.OutOrStdout().Write(rendered)
	if len(rendered) > 0 && rendered[len(rendered)-1] != '\n' {
		fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}

func rawHTTPChatRequestToParams(body []byte) (provider.RequestParams, error) {
	var req rawHTTPChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return provider.RequestParams{}, fmt.Errorf("parse captured chat request: %w", err)
	}
	if len(req.Tools) > 0 {
		return provider.RequestParams{}, fmt.Errorf("google raw-http replay supports text-only chat payloads; tools are not supported")
	}
	var systemBlocks []model.SystemBlock
	var messages []model.Message
	for i, msg := range req.Messages {
		if len(msg.ToolCalls) > 0 && string(msg.ToolCalls) != "null" && string(msg.ToolCalls) != "[]" {
			return provider.RequestParams{}, fmt.Errorf("google raw-http replay supports text-only chat payloads; message %d contains tool_calls", i)
		}
		if msg.ToolCallID != "" {
			return provider.RequestParams{}, fmt.Errorf("google raw-http replay supports text-only chat payloads; message %d is a tool result", i)
		}
		content, err := rawHTTPStringContent(msg.Content)
		if err != nil {
			return provider.RequestParams{}, fmt.Errorf("google raw-http replay supports text-only chat payloads; message %d: %w", i, err)
		}
		switch msg.Role {
		case "system":
			if content != "" {
				systemBlocks = append(systemBlocks, model.SystemBlock{Text: content, Cacheable: true})
			}
		case "user":
			messages = append(messages, model.Message{
				ID:      model.NewUUID(),
				Role:    model.RoleUser,
				Content: []model.ContentPart{model.TextPart{Text: content}},
			})
		case "assistant":
			messages = append(messages, model.Message{
				ID:      model.NewUUID(),
				Role:    model.RoleAssistant,
				Content: []model.ContentPart{model.TextPart{Text: content}},
			})
		default:
			return provider.RequestParams{}, fmt.Errorf("message %d has unsupported role %q", i, msg.Role)
		}
	}
	return provider.RequestParams{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Messages:    messages,
		System:      model.SystemPrompt{Blocks: systemBlocks},
		Temperature: req.Temperature,
	}, nil
}

func rawHTTPStringContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	return "", fmt.Errorf("content is not a string")
}

func rawHTTPReplayFormatFromFlags(cmd *cobra.Command) string {
	if pretty, _ := cmd.Flags().GetBool("pretty"); pretty {
		return "pretty"
	}
	format, _ := cmd.Flags().GetString("format")
	return format
}

func inferRawHTTPReplayProvider(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Hostname()) {
	case "openrouter.ai":
		return "openrouter"
	case "api.morphllm.com":
		return "morphllm"
	case "api.openai.com":
		return "openai"
	case "api.groq.com":
		return "groq"
	default:
		return ""
	}
}

func isRawHTTPReplayProvider(provider string) bool {
	switch provider {
	case "openrouter", "morphllm", "openai", "groq":
		return true
	default:
		return false
	}
}

func rawHTTPRequestModel(body []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	model, _ := payload["model"].(string)
	return model, nil
}

func rewriteRawHTTPRequestModel(body []byte, model string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse captured request JSON before model rewrite: %w", err)
	}
	payload["model"] = model
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal captured request JSON after model rewrite: %w", err)
	}
	return rewritten, nil
}

func overrideCapturedURL(captured, base string) (string, error) {
	capturedURL, err := url.Parse(captured)
	if err != nil {
		return "", fmt.Errorf("parse captured URL: %w", err)
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return "", fmt.Errorf("base-url must include scheme and host")
	}
	result := *baseURL
	basePath := strings.TrimRight(baseURL.Path, "/")
	capturedPath := capturedURL.EscapedPath()
	if basePath != "" && strings.HasPrefix(capturedPath, basePath+"/") {
		result.Path = capturedPath
	} else {
		endpointPath := strings.TrimLeft(capturedPath, "/")
		if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(endpointPath, "v1/") {
			endpointPath = strings.TrimPrefix(endpointPath, "v1/")
		}
		result.Path = basePath + "/" + endpointPath
	}
	result.RawQuery = capturedURL.RawQuery
	return result.String(), nil
}

func skipReplayHeader(k string) bool {
	switch strings.ToLower(k) {
	case "authorization", "content-length", "host":
		return true
	default:
		return false
	}
}

func printRawHTTPSummary(w io.Writer, body []byte) {
	summary := summarizeSSE(body)
	if summary == "" {
		summary = summarizeJSONResponse(body)
	}
	if summary != "" {
		fmt.Fprintln(w, summary)
	}
}

func renderRawHTTPReplayOutput(status string, body []byte, format string) ([]byte, error) {
	switch format {
	case "", "raw":
		return body, nil
	case "pretty":
		return renderRawHTTPPrettyResponse(status, body)
	case "content":
		return renderRawHTTPContent(body)
	default:
		return nil, fmt.Errorf("unsupported raw HTTP replay format %q; expected raw, pretty, or content", format)
	}
}

func renderNativeReplayOutput(providerName string, resp model.Response, format string) ([]byte, error) {
	switch format {
	case "", "raw":
		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("render native replay response: %w", err)
		}
		return append(data, '\n'), nil
	case "pretty":
		return renderNativePrettyResponse(providerName, resp), nil
	case "content":
		return []byte(nativeResponseContent(resp)), nil
	default:
		return nil, fmt.Errorf("unsupported raw HTTP replay format %q; expected raw, pretty, or content", format)
	}
}

func renderRawHTTPPrettyRequest(body []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("render pretty request: %w", err)
	}

	var b strings.Builder
	b.WriteString("# Raw HTTP Replay Request\n\n")
	if model, _ := obj["model"].(string); model != "" {
		fmt.Fprintf(&b, "Model: %s\n", model)
	}
	writeRawHTTPScalar(&b, "Max tokens", obj["max_tokens"])
	writeRawHTTPScalar(&b, "Temperature", obj["temperature"])
	messages := asSlice(obj["messages"])
	fmt.Fprintf(&b, "Messages: %d\n", len(messages))
	tools := asSlice(obj["tools"])
	if len(tools) == 0 {
		b.WriteString("Tools: none\n")
	} else {
		fmt.Fprintf(&b, "Tools: %d\n", len(tools))
	}

	b.WriteString("\n## Messages\n\n")
	if len(messages) == 0 {
		b.WriteString("none\n")
	} else {
		for i, item := range messages {
			msg, _ := item.(map[string]any)
			role, _ := msg["role"].(string)
			if role == "" {
				role = "unknown"
			}
			fmt.Fprintf(&b, "### %d. %s\n\n", i+1, strings.ToUpper(role))
			content := readableContent(msg["content"])
			if content == "" {
				b.WriteString("none\n")
			} else {
				b.WriteString(content)
				if !strings.HasSuffix(content, "\n") {
					b.WriteString("\n")
				}
			}
			if toolCalls := asSlice(msg["tool_calls"]); len(toolCalls) > 0 {
				b.WriteString("\nTool calls:\n")
				writeRawHTTPJSONBlock(&b, toolCalls)
			}
			if toolCallID, _ := msg["tool_call_id"].(string); toolCallID != "" {
				fmt.Fprintf(&b, "\nTool call ID: %s\n", toolCallID)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Tools\n\n")
	if len(tools) == 0 {
		b.WriteString("none\n")
	} else {
		writeRawHTTPJSONBlock(&b, tools)
	}
	return []byte(b.String()), nil
}

func writeRawHTTPScalar(b *strings.Builder, label string, value any) {
	switch v := value.(type) {
	case nil:
		return
	case float64:
		fmt.Fprintf(b, "%s: %g\n", label, v)
	case json.Number:
		fmt.Fprintf(b, "%s: %s\n", label, v.String())
	default:
		fmt.Fprintf(b, "%s: %v\n", label, v)
	}
}

func writeRawHTTPJSONBlock(b *strings.Builder, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(b, "%v\n", value)
		return
	}
	b.WriteString("```json\n")
	b.Write(data)
	b.WriteString("\n```\n")
}

func appendPrettySections(first, second []byte) []byte {
	out := append([]byte{}, first...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, '\n')
	out = append(out, second...)
	return out
}

func renderNativePrettyResponse(providerName string, resp model.Response) []byte {
	var b strings.Builder
	b.WriteString("# Raw HTTP Replay Response\n\n")
	b.WriteString("Transport: google provider\n")
	if providerName != "" {
		fmt.Fprintf(&b, "Provider: %s\n", providerName)
	}
	if resp.Model != "" {
		fmt.Fprintf(&b, "Model: %s\n", resp.Model)
	}
	if resp.StopReason != "" {
		fmt.Fprintf(&b, "Stop reason: %s\n", resp.StopReason)
	}
	if usage := nativeUsageSummary(resp.Usage); usage != "" {
		fmt.Fprintf(&b, "Usage: %s\n", usage)
	}

	content := strings.TrimLeft(nativeResponseContent(resp), "\r\n")
	b.WriteString("\n## Assistant Content\n\n")
	if content != "" {
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("none\n")
	}

	b.WriteString("\n## Tool Calls\n\n")
	toolCalls := nativeToolCalls(resp)
	if strings.TrimSpace(toolCalls) == "" {
		b.WriteString("none\n")
	} else {
		b.WriteString(toolCalls)
		if !strings.HasSuffix(toolCalls, "\n") {
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

func nativeResponseContent(resp model.Response) string {
	var parts []string
	for _, part := range resp.Content {
		switch p := part.(type) {
		case model.TextPart:
			if p.Text != "" {
				parts = append(parts, p.Text)
			}
		case model.ThinkingPart:
			if p.Text != "" {
				parts = append(parts, p.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func nativeToolCalls(resp model.Response) string {
	var b strings.Builder
	var index int
	for _, part := range resp.Content {
		tc, ok := part.(model.ToolCallPart)
		if !ok {
			continue
		}
		index++
		fmt.Fprintf(&b, "### Tool Call %d\n", index)
		if tc.ID != "" {
			fmt.Fprintf(&b, "ID: %s\n", tc.ID)
		}
		if tc.Name != "" {
			fmt.Fprintf(&b, "Function: %s\n", tc.Name)
		}
		b.WriteString("\nArguments:\n")
		formatted, lang := prettyRawHTTPJSONArg(string(tc.Input))
		fmt.Fprintf(&b, "```%s\n%s\n```\n\n", lang, formatted)
	}
	return b.String()
}

func nativeUsageSummary(usage model.TokenUsage) string {
	parts := []string{
		nativeUsagePart("prompt", usage.InputTokens),
		nativeUsagePart("cached", usage.CacheReadInputTokens),
		nativeUsagePart("cache_create", usage.CacheCreationInputTokens),
		nativeUsagePart("completion", usage.OutputTokens),
		nativeUsagePart("total", usage.InputTokens+usage.CacheReadInputTokens+usage.CacheCreationInputTokens+usage.OutputTokens),
	}
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

func nativeUsagePart(label string, value int) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%s=%d", label, value)
}

func renderRawHTTPContent(body []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("render content response: %w", err)
	}
	var parts []string
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		msg, _ := ch["message"].(map[string]any)
		if content := readableContent(msg["content"]); content != "" {
			parts = append(parts, content)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(parts, "\n")), nil
}

func renderRawHTTPPrettyResponse(status string, body []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		if isRawHTTPSSE(body) {
			return renderRawHTTPPrettyStreamResponse(status, body)
		}
		return nil, fmt.Errorf("render pretty response: %w", err)
	}

	var b strings.Builder
	b.WriteString("# Raw HTTP Replay Response\n\n")
	if status != "" {
		fmt.Fprintf(&b, "HTTP: %s\n", status)
	}
	if model, _ := obj["model"].(string); model != "" {
		fmt.Fprintf(&b, "Model: %s\n", model)
	}
	if finish := rawHTTPFinishReason(obj); finish != "" {
		fmt.Fprintf(&b, "Finish reason: %s\n", finish)
	}
	if usage := rawHTTPUsageSummary(obj); usage != "" {
		fmt.Fprintf(&b, "Usage: %s\n", usage)
	}

	reasoning := renderRawHTTPReasoning(obj)
	b.WriteString("\n## Reasoning\n\n")
	if strings.TrimSpace(reasoning) == "" {
		b.WriteString("none\n")
	} else {
		b.WriteString(reasoning)
		if !strings.HasSuffix(reasoning, "\n") {
			b.WriteString("\n")
		}
	}

	content, err := renderRawHTTPContent(body)
	if err != nil {
		return nil, err
	}
	b.WriteString("\n## Assistant Content\n\n")
	if len(content) > 0 {
		normalized := strings.TrimLeft(string(content), "\r\n")
		b.WriteString(normalized)
		if !strings.HasSuffix(normalized, "\n") {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("none\n")
	}

	b.WriteString("\n## Tool Calls\n\n")
	toolCalls := renderRawHTTPToolCalls(obj)
	if strings.TrimSpace(toolCalls) == "" {
		b.WriteString("none\n")
	} else {
		b.WriteString(toolCalls)
		if !strings.HasSuffix(toolCalls, "\n") {
			b.WriteString("\n")
		}
	}
	return []byte(b.String()), nil
}

func rawHTTPFinishReason(obj map[string]any) string {
	var reasons []string
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		if reason, _ := ch["finish_reason"].(string); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	return strings.Join(reasons, ", ")
}

func renderRawHTTPReasoning(obj map[string]any) string {
	var parts []string
	for i, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		msg, _ := ch["message"].(map[string]any)
		reasoning := readableContent(msg["reasoning"])
		if strings.TrimSpace(reasoning) == "" {
			continue
		}
		if len(asSlice(obj["choices"])) > 1 {
			parts = append(parts, fmt.Sprintf("### Choice %d\n\n%s", i+1, reasoning))
		} else {
			parts = append(parts, reasoning)
		}
	}
	return strings.Join(parts, "\n\n")
}

func rawHTTPUsageSummary(obj map[string]any) string {
	usage, _ := obj["usage"].(map[string]any)
	if len(usage) == 0 {
		return ""
	}
	parts := []string{
		rawHTTPUsagePart(usage, "prompt", "prompt_tokens"),
	}
	if details, _ := usage["prompt_tokens_details"].(map[string]any); len(details) > 0 {
		parts = append(parts, rawHTTPUsagePart(details, "cached", "cached_tokens"))
	}
	parts = append(parts,
		rawHTTPUsagePart(usage, "completion", "completion_tokens"),
		rawHTTPUsagePart(usage, "total", "total_tokens"),
	)
	if details, _ := usage["completion_tokens_details"].(map[string]any); len(details) > 0 {
		parts = append(parts, rawHTTPUsagePart(details, "reasoning", "reasoning_tokens"))
	}
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

func rawHTTPUsagePart(values map[string]any, label, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%s=%.0f", label, v)
	case int:
		return fmt.Sprintf("%s=%d", label, v)
	case json.Number:
		return fmt.Sprintf("%s=%s", label, v.String())
	default:
		return fmt.Sprintf("%s=%v", label, v)
	}
}

func renderRawHTTPToolCalls(obj map[string]any) string {
	var b strings.Builder
	var index int
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		msg, _ := ch["message"].(map[string]any)
		for _, item := range asSlice(msg["tool_calls"]) {
			call, _ := item.(map[string]any)
			index++
			fmt.Fprintf(&b, "### Tool Call %d\n", index)
			if id, _ := call["id"].(string); id != "" {
				fmt.Fprintf(&b, "ID: %s\n", id)
			}
			if typ, _ := call["type"].(string); typ != "" {
				fmt.Fprintf(&b, "Type: %s\n", typ)
			}
			function, _ := call["function"].(map[string]any)
			if name, _ := function["name"].(string); name != "" {
				fmt.Fprintf(&b, "Function: %s\n", name)
			}
			b.WriteString("\nArguments:\n")
			args := readableContent(function["arguments"])
			formatted, lang := prettyRawHTTPJSONArg(args)
			fmt.Fprintf(&b, "```%s\n%s\n```\n\n", lang, formatted)
		}
	}
	return b.String()
}

func prettyRawHTTPJSONArg(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "text"
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw, "text"
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return raw, "text"
	}
	return string(data), "json"
}

type rawHTTPSSELine struct {
	Raw      string
	Field    string
	Value    string
	DataDone bool
	DataJSON map[string]any
	DataErr  error
}

type rawHTTPStreamToolCall struct {
	Index     int
	ID        string
	Type      string
	Name      string
	Arguments string
}

type rawHTTPStreamResponse struct {
	Lines      []rawHTTPSSELine
	ID         string
	Object     string
	Model      string
	Finish     []string
	Usage      map[string]any
	Reasoning  strings.Builder
	Content    strings.Builder
	ToolCalls  map[int]*rawHTTPStreamToolCall
	ToolOrder  []int
	ParseError error
}

func isRawHTTPSSE(body []byte) bool {
	for _, line := range splitRawHTTPLines(body) {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, "data:") ||
			strings.HasPrefix(line, "event:") ||
			strings.HasPrefix(line, "id:") ||
			strings.HasPrefix(line, "retry:") ||
			strings.HasPrefix(line, ":")
	}
	return false
}

func renderRawHTTPPrettyStreamResponse(status string, body []byte) ([]byte, error) {
	stream := parseRawHTTPSSE(body)
	var b strings.Builder
	b.WriteString("# Raw HTTP Replay Response\n\n")
	b.WriteString("Transport: sse\n")
	if status != "" {
		fmt.Fprintf(&b, "HTTP: %s\n", status)
	}
	if stream.Model != "" {
		fmt.Fprintf(&b, "Model: %s\n", stream.Model)
	}
	if stream.ID != "" {
		fmt.Fprintf(&b, "ID: %s\n", stream.ID)
	}
	if stream.Object != "" {
		fmt.Fprintf(&b, "Object: %s\n", stream.Object)
	}
	if len(stream.Finish) > 0 {
		fmt.Fprintf(&b, "Finish reason: %s\n", strings.Join(stream.Finish, ", "))
	}
	if len(stream.Usage) > 0 {
		if usage := rawHTTPUsageSummary(map[string]any{"usage": stream.Usage}); usage != "" {
			fmt.Fprintf(&b, "Usage: %s\n", usage)
		}
	}
	if stream.ParseError != nil {
		fmt.Fprintf(&b, "Parse warning: %v\n", stream.ParseError)
	}

	b.WriteString("\n## Reasoning\n\n")
	writePrettyTextSection(&b, stream.Reasoning.String())

	b.WriteString("\n## Assistant Content\n\n")
	writePrettyTextSection(&b, strings.TrimLeft(stream.Content.String(), "\r\n"))

	b.WriteString("\n## Tool Calls\n\n")
	writeRawHTTPStreamToolCalls(&b, stream)

	b.WriteString("\n## Stream Details\n\n")
	writeRawHTTPStreamDetails(&b, stream.Lines)
	return []byte(b.String()), nil
}

func writePrettyTextSection(b *strings.Builder, text string) {
	if strings.TrimSpace(text) == "" {
		b.WriteString("none\n")
		return
	}
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteString("\n")
	}
}

func parseRawHTTPSSE(body []byte) rawHTTPStreamResponse {
	stream := rawHTTPStreamResponse{
		ToolCalls: make(map[int]*rawHTTPStreamToolCall),
	}
	for _, rawLine := range splitRawHTTPLines(body) {
		line := strings.TrimSuffix(rawLine, "\r")
		item := rawHTTPSSELine{Raw: line}
		if line != "" && !strings.HasPrefix(line, ":") {
			field, value, ok := strings.Cut(line, ":")
			if ok {
				item.Field = field
				item.Value = strings.TrimPrefix(value, " ")
				if item.Field == "data" {
					if item.Value == "[DONE]" {
						item.DataDone = true
					} else {
						var obj map[string]any
						if err := json.Unmarshal([]byte(item.Value), &obj); err != nil {
							item.DataErr = err
						} else {
							item.DataJSON = obj
							applyRawHTTPStreamChunk(&stream, obj)
						}
					}
				}
			}
		}
		if item.DataErr != nil && stream.ParseError == nil {
			stream.ParseError = item.DataErr
		}
		stream.Lines = append(stream.Lines, item)
	}
	return stream
}

func splitRawHTTPLines(body []byte) []string {
	text := string(body)
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func applyRawHTTPStreamChunk(stream *rawHTTPStreamResponse, obj map[string]any) {
	if id, _ := obj["id"].(string); id != "" && stream.ID == "" {
		stream.ID = id
	}
	if object, _ := obj["object"].(string); object != "" && stream.Object == "" {
		stream.Object = object
	}
	if model, _ := obj["model"].(string); model != "" && stream.Model == "" {
		stream.Model = model
	}
	if usage, _ := obj["usage"].(map[string]any); len(usage) > 0 {
		stream.Usage = usage
	}
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		if finish, _ := ch["finish_reason"].(string); finish != "" {
			stream.Finish = append(stream.Finish, finish)
		}
		delta, _ := ch["delta"].(map[string]any)
		if reasoning, ok := delta["reasoning"]; ok {
			stream.Reasoning.WriteString(readableContent(reasoning))
		}
		if content, ok := delta["content"]; ok {
			stream.Content.WriteString(readableContent(content))
		}
		for _, item := range asSlice(delta["tool_calls"]) {
			call, _ := item.(map[string]any)
			applyRawHTTPStreamToolCall(stream, call)
		}
	}
}

func applyRawHTTPStreamToolCall(stream *rawHTTPStreamResponse, call map[string]any) {
	index := len(stream.ToolOrder)
	if value, ok := call["index"].(float64); ok {
		index = int(value)
	}
	tc := stream.ToolCalls[index]
	if tc == nil {
		tc = &rawHTTPStreamToolCall{Index: index}
		stream.ToolCalls[index] = tc
		stream.ToolOrder = append(stream.ToolOrder, index)
		sort.Ints(stream.ToolOrder)
	}
	if id, _ := call["id"].(string); id != "" {
		tc.ID = id
	}
	if typ, _ := call["type"].(string); typ != "" {
		tc.Type = typ
	}
	function, _ := call["function"].(map[string]any)
	if name, _ := function["name"].(string); name != "" {
		tc.Name = name
	}
	if args, ok := function["arguments"]; ok {
		tc.Arguments += readableContent(args)
	}
}

func writeRawHTTPStreamToolCalls(b *strings.Builder, stream rawHTTPStreamResponse) {
	if len(stream.ToolOrder) == 0 {
		b.WriteString("none\n")
		return
	}
	for i, index := range stream.ToolOrder {
		tc := stream.ToolCalls[index]
		fmt.Fprintf(b, "### Tool Call %d\n", i+1)
		fmt.Fprintf(b, "Index: %d\n", tc.Index)
		if tc.ID != "" {
			fmt.Fprintf(b, "ID: %s\n", tc.ID)
		}
		if tc.Type != "" {
			fmt.Fprintf(b, "Type: %s\n", tc.Type)
		}
		if tc.Name != "" {
			fmt.Fprintf(b, "Function: %s\n", tc.Name)
		}
		b.WriteString("\nArguments:\n")
		formatted, lang := prettyRawHTTPJSONArg(tc.Arguments)
		fmt.Fprintf(b, "```%s\n%s\n```\n\n", lang, formatted)
	}
}

func writeRawHTTPStreamDetails(b *strings.Builder, lines []rawHTTPSSELine) {
	dataChunks, malformed, blanks, done := rawHTTPSSECounts(lines)
	fmt.Fprintf(b, "Data chunks: %d\n", dataChunks)
	if malformed > 0 {
		fmt.Fprintf(b, "Malformed data chunks: %d\n", malformed)
	}
	if blanks > 0 {
		fmt.Fprintf(b, "Event boundaries: %d blank lines\n", blanks)
	}
	fmt.Fprintf(b, "Done marker: %t\n", done)

	writeRawHTTPSSEControlLines(b, lines)
	writeRawHTTPMalformedChunks(b, lines)
	writeRawHTTPDataChunks(b, lines)
}

func rawHTTPSSECounts(lines []rawHTTPSSELine) (dataChunks, malformed, blanks int, done bool) {
	for _, line := range lines {
		switch {
		case line.Raw == "":
			blanks++
		case line.Field == "data" && line.DataDone:
			done = true
		case line.Field == "data" && line.DataErr != nil:
			malformed++
		case line.Field == "data" && line.DataJSON != nil:
			dataChunks++
		}
	}
	return dataChunks, malformed, blanks, done
}

func writeRawHTTPSSEControlLines(b *strings.Builder, lines []rawHTTPSSELine) {
	var controls []string
	for _, line := range lines {
		if line.Raw == "" || line.Field == "data" {
			continue
		}
		if strings.HasPrefix(line.Raw, ":") {
			controls = append(controls, fmt.Sprintf("comment: %s", strings.TrimSpace(strings.TrimPrefix(line.Raw, ":"))))
			continue
		}
		if line.Field != "" {
			controls = append(controls, fmt.Sprintf("%s: %s", line.Field, line.Value))
			continue
		}
		controls = append(controls, fmt.Sprintf("raw: %s", line.Raw))
	}
	if len(controls) == 0 {
		return
	}
	b.WriteString("\n### SSE Control Lines\n\n")
	for _, line := range controls {
		fmt.Fprintf(b, "- %s\n", line)
	}
}

func writeRawHTTPMalformedChunks(b *strings.Builder, lines []rawHTTPSSELine) {
	var chunk int
	var wroteHeader bool
	for _, line := range lines {
		if line.Field != "data" {
			continue
		}
		if line.DataJSON != nil {
			chunk++
			continue
		}
		if line.DataDone {
			continue
		}
		if line.DataErr == nil {
			continue
		}
		if !wroteHeader {
			b.WriteString("\n### Malformed Data Chunks\n\n")
			wroteHeader = true
		}
		chunk++
		fmt.Fprintf(b, "#### Chunk %d\n\n", chunk)
		fmt.Fprintf(b, "Parse error: %v\n\n", line.DataErr)
		fmt.Fprintf(b, "```text\n%s\n```\n", line.Value)
	}
}

func writeRawHTTPDataChunks(b *strings.Builder, lines []rawHTTPSSELine) {
	coverage := collectRawHTTPStreamCoverage(lines)
	b.WriteString("\n### Parsed Chunk Coverage\n\n")
	fmt.Fprintf(b, "- reasoning deltas: %d chunks", coverage.ReasoningChunks)
	if coverage.ReasoningChunks > 0 {
		b.WriteString(" (full text in Reasoning)")
	}
	b.WriteString("\n")
	fmt.Fprintf(b, "- content deltas: %d chunks", coverage.ContentChunks)
	if coverage.ContentChunks > 0 {
		b.WriteString(" (full text in Assistant Content)")
	}
	b.WriteString("\n")
	fmt.Fprintf(b, "- tool call deltas: %d chunks", coverage.ToolCallChunks)
	if coverage.ToolCallChunks > 0 {
		b.WriteString(" (reconstructed in Tool Calls)")
	}
	b.WriteString("\n")
	fmt.Fprintf(b, "- usage chunks: %d\n", coverage.UsageChunks)

	if len(coverage.FieldOccurrences) > 0 {
		b.WriteString("\n### Field Coverage\n\n")
		for _, key := range sortedRawHTTPKeys(coverage.FieldOccurrences) {
			fmt.Fprintf(b, "- %s: %s\n", key, formatRawHTTPChunkRanges(coverage.FieldOccurrences[key]))
		}
	}
	if len(coverage.UsagePayloads) > 0 {
		b.WriteString("\n### Usage Payloads\n\n")
		for _, payload := range coverage.UsagePayloads {
			data, err := json.MarshalIndent(payload.Value, "", "  ")
			if err != nil {
				fmt.Fprintf(b, "#### Chunk %d\n\n%v\n\n", payload.Chunk, payload.Value)
				continue
			}
			fmt.Fprintf(b, "#### Chunk %d\n\n```json\n%s\n```\n\n", payload.Chunk, data)
		}
	}
}

type rawHTTPStreamCoverage struct {
	ReasoningChunks  int
	ContentChunks    int
	ToolCallChunks   int
	UsageChunks      int
	FieldOccurrences map[string][]int
	UsagePayloads    []rawHTTPChunkValue
}

type rawHTTPChunkValue struct {
	Chunk int
	Value any
}

func collectRawHTTPStreamCoverage(lines []rawHTTPSSELine) rawHTTPStreamCoverage {
	coverage := rawHTTPStreamCoverage{
		FieldOccurrences: make(map[string][]int),
	}
	var chunk int
	for _, line := range lines {
		if line.Field != "data" || line.DataJSON == nil {
			continue
		}
		chunk++
		obj := line.DataJSON
		for key, value := range obj {
			if key == "choices" {
				continue
			}
			if key == "usage" {
				coverage.UsageChunks++
				coverage.UsagePayloads = append(coverage.UsagePayloads, rawHTTPChunkValue{Chunk: chunk, Value: value})
				continue
			}
			addRawHTTPOccurrence(coverage.FieldOccurrences, "top."+key+"="+rawHTTPPrettyScalar(value), chunk)
		}
		for _, choice := range asSlice(obj["choices"]) {
			ch, _ := choice.(map[string]any)
			for key, value := range ch {
				if key == "delta" {
					continue
				}
				addRawHTTPOccurrence(coverage.FieldOccurrences, "choice."+key+"="+rawHTTPPrettyScalar(value), chunk)
			}
			delta, _ := ch["delta"].(map[string]any)
			for key, value := range delta {
				switch key {
				case "reasoning":
					coverage.ReasoningChunks++
				case "content":
					coverage.ContentChunks++
				case "tool_calls":
					if calls := asSlice(value); len(calls) > 0 {
						coverage.ToolCallChunks++
						addRawHTTPToolCallCoverage(coverage.FieldOccurrences, chunk, calls)
					}
				default:
					addRawHTTPOccurrence(coverage.FieldOccurrences, "delta."+key+"="+rawHTTPPrettyScalar(value), chunk)
				}
			}
		}
	}
	return coverage
}

func addRawHTTPToolCallCoverage(occurrences map[string][]int, chunk int, calls []any) {
	for _, item := range calls {
		call, _ := item.(map[string]any)
		for key, value := range call {
			if key == "function" {
				continue
			}
			addRawHTTPOccurrence(occurrences, "tool_call."+key+"="+rawHTTPPrettyScalar(value), chunk)
		}
		function, _ := call["function"].(map[string]any)
		for key, value := range function {
			addRawHTTPOccurrence(occurrences, "tool_call.function."+key+"="+rawHTTPPrettyScalar(value), chunk)
		}
	}
}

func addRawHTTPOccurrence(occurrences map[string][]int, key string, chunk int) {
	seen := occurrences[key]
	if len(seen) > 0 && seen[len(seen)-1] == chunk {
		return
	}
	occurrences[key] = append(seen, chunk)
}

func sortedRawHTTPKeys(values map[string][]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatRawHTTPChunkRanges(chunks []int) string {
	if len(chunks) == 0 {
		return ""
	}
	var parts []string
	start := chunks[0]
	prev := chunks[0]
	for _, chunk := range chunks[1:] {
		if chunk == prev+1 {
			prev = chunk
			continue
		}
		parts = append(parts, formatRawHTTPChunkRange(start, prev))
		start = chunk
		prev = chunk
	}
	parts = append(parts, formatRawHTTPChunkRange(start, prev))
	return "chunks " + strings.Join(parts, ", ")
}

func formatRawHTTPChunkRange(start, end int) string {
	if start == end {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func rawHTTPPrettyScalar(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%.0f", v)
		}
		return fmt.Sprintf("%v", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		data, err := json.Marshal(value)
		if err == nil {
			return string(data)
		}
		return fmt.Sprint(value)
	}
}

func summarizeSSE(body []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	var text strings.Builder
	var finish string
	var toolCalls int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(data), &obj); err != nil {
			continue
		}
		for _, choice := range asSlice(obj["choices"]) {
			ch, _ := choice.(map[string]any)
			if f, _ := ch["finish_reason"].(string); f != "" {
				finish = f
			}
			delta, _ := ch["delta"].(map[string]any)
			if c, _ := delta["content"].(string); c != "" {
				text.WriteString(c)
			}
			if calls := asSlice(delta["tool_calls"]); len(calls) > 0 {
				toolCalls += len(calls)
			}
		}
	}
	if finish == "" && toolCalls == 0 && text.Len() == 0 {
		return ""
	}
	return fmt.Sprintf("summary: finish_reason=%q tool_call_deltas=%d text_prefix=%q", finish, toolCalls, trimSummary(text.String()))
}

func summarizeJSONResponse(body []byte) string {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	var finish string
	var toolCalls int
	var text string
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		if f, _ := ch["finish_reason"].(string); f != "" {
			finish = f
		}
		msg, _ := ch["message"].(map[string]any)
		if c, _ := msg["content"].(string); c != "" {
			text += c
		}
		toolCalls += len(asSlice(msg["tool_calls"]))
	}
	return fmt.Sprintf("summary: finish_reason=%q tool_calls=%d text_prefix=%q", finish, toolCalls, trimSummary(text))
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func trimSummary(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		return s[:160] + "..."
	}
	return s
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func replayRawHTTPDumpRun(cmd *cobra.Command, args []string) error {
	input := args[0]
	captureDir, err := resolveRawHTTPCaptureDir(input)
	if err != nil {
		return err
	}
	out, _ := cmd.Flags().GetString("out")
	if out == "" {
		out = filepath.Join(input, "turn-payloads")
	}
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	return dumpRawHTTPCaptures(captureDir, out, overwrite)
}

func replayRawHTTPAuditRun(cmd *cobra.Command, args []string) error {
	requireResponses, _ := cmd.Flags().GetBool("require-responses")
	return auditRawHTTPReplayEvidence(args[0], requireResponses, cmd.OutOrStdout())
}

func auditRawHTTPReplayEvidence(root string, requireResponses bool, w io.Writer) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}

	var cases []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "request.json" {
			cases = append(cases, filepath.Dir(path))
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(cases)
	if len(cases) == 0 {
		return fmt.Errorf("no request.json files found under %s", root)
	}

	fmt.Fprintln(w, "case\trequest_bytes\tresponse_bytes\tstatus\tsummary")
	missing := 0
	for _, dir := range cases {
		requestPath := filepath.Join(dir, "request.json")
		reqInfo, err := os.Stat(requestPath)
		if err != nil {
			return err
		}
		evidence, err := rawcapture.ReadResponseEvidence(dir)
		if err != nil {
			return fmt.Errorf("read %s response evidence: %w", filepath.Base(dir), err)
		}
		status := rawHTTPResponseEvidenceStatus(evidence)
		responseBytes := evidence.FileBytes
		summary := ""
		if evidence.Complete {
			status, summary = classifyRawHTTPResponseEvidence(evidence.Body)
			if status != "response_ok" {
				missing++
			}
		} else {
			missing++
			summary = summarizeRawHTTPResponseEvidenceProblem(evidence)
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n",
			filepath.ToSlash(dir),
			reqInfo.Size(),
			responseBytes,
			status,
			summary,
		)
	}
	if requireResponses && missing > 0 {
		return fmt.Errorf("%d replay case(s) lack usable response evidence", missing)
	}
	return nil
}

func rawHTTPResponseEvidenceStatus(evidence rawcapture.ResponseEvidence) string {
	if evidence.Complete {
		return "response_ok"
	}
	if evidence.Problem != "" {
		return evidence.Problem
	}
	return "incomplete_response"
}

func summarizeRawHTTPResponseEvidenceProblem(evidence rawcapture.ResponseEvidence) string {
	switch evidence.Problem {
	case "response_transport_error":
		return trimSummary(evidence.Meta.Error)
	case "capture_write_error":
		return trimSummary(evidence.Meta.CaptureError)
	case "response_byte_mismatch":
		return fmt.Sprintf("metadata response_bytes=%d file_bytes=%d", evidence.Meta.ResponseBytes, evidence.FileBytes)
	case "response_sha256_mismatch":
		return "metadata response_sha256 does not match response.raw"
	case "missing_response_sha256":
		return "metadata response_sha256 is empty"
	default:
		return ""
	}
}

func classifyRawHTTPResponseEvidence(body []byte) (string, string) {
	if len(body) == 0 {
		return "empty_response", ""
	}
	if strings.TrimSpace(string(body)) == "" {
		return "empty_response", ""
	}
	if summary := summarizeSSE(body); summary != "" {
		return "response_ok", summary
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return "response_non_json", trimSummary(string(body))
	}
	if _, ok := obj["error"]; ok {
		return "response_error", trimSummary(string(body))
	}
	if len(asSlice(obj["choices"])) == 0 {
		return "response_without_choices", trimSummary(string(body))
	}
	return "response_ok", summarizeJSONResponse(body)
}

func resolveRawHTTPCaptureDir(input string) (string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", input)
	}

	nested := filepath.Join(input, "raw-http-pragma")
	if nestedInfo, err := os.Stat(nested); err == nil && nestedInfo.IsDir() {
		return nested, nil
	}

	entries, err := os.ReadDir(input)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if isRawHTTPTurnDir(entry.Name()) {
			return input, nil
		}
	}
	return "", fmt.Errorf("%s is neither a run directory containing raw-http-pragma nor a raw HTTP capture directory", input)
}

func isRawHTTPTurnDir(name string) bool {
	return name != "" && name[0] >= '0' && name[0] <= '9'
}

func dumpRawHTTPCaptures(captureDir, outDir string, overwrite bool) error {
	turns, err := rawHTTPTurnDirs(captureDir)
	if err != nil {
		return err
	}
	if len(turns) == 0 {
		return fmt.Errorf("no captured turns found in %s", captureDir)
	}

	if _, err := os.Stat(outDir); err == nil {
		if !overwrite {
			return fmt.Errorf("output directory exists; pass --overwrite: %s", outDir)
		}
		if err := os.RemoveAll(outDir); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	index := []string{"turn\tsource_dir\trequest_path\tresponse_path\tresponse_state\tstatus_code\tstarted_at\tcompleted_at"}
	for _, turn := range turns {
		turnNumber := rawHTTPTurnNumber(filepath.Base(turn))
		dest := filepath.Join(outDir, "turn-"+turnNumber)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}

		requestPath := filepath.Join(dest, "request.json")
		if err := writePrettyJSONFile(filepath.Join(turn, "request.json"), requestPath); err != nil {
			return fmt.Errorf("dump %s request.json: %w", filepath.Base(turn), err)
		}
		if err := writeRequestMessages(requestPath, filepath.Join(dest, "request_messages.md")); err != nil {
			return fmt.Errorf("dump %s request_messages.md: %w", filepath.Base(turn), err)
		}

		for _, name := range []string{
			"request.meta.json",
			"request.headers.json",
			"response.meta.json",
			"response.headers.json",
		} {
			if err := copyOptionalPrettyJSON(filepath.Join(turn, name), filepath.Join(dest, name)); err != nil {
				return fmt.Errorf("dump %s %s: %w", filepath.Base(turn), name, err)
			}
		}

		responsePath := ""
		evidence, err := rawcapture.ReadResponseEvidence(turn)
		if err != nil {
			return fmt.Errorf("dump %s response evidence: %w", filepath.Base(turn), err)
		}
		responseState := rawHTTPResponseEvidenceStatus(evidence)
		if evidence.RawPath != "" {
			responsePath = filepath.Join(dest, "response.json")
			if !evidence.Complete {
				responsePath = filepath.Join(dest, "response.raw")
				if err := copyFile(evidence.RawPath, responsePath); err != nil {
					return fmt.Errorf("dump %s response.raw: %w", filepath.Base(turn), err)
				}
			} else if err := writePrettyJSONFile(evidence.RawPath, responsePath); err != nil {
				if !errorsIsJSONSyntax(err) {
					return fmt.Errorf("dump %s response.raw: %w", filepath.Base(turn), err)
				}
				responsePath = filepath.Join(dest, "response.raw")
				if err := copyFile(evidence.RawPath, responsePath); err != nil {
					return fmt.Errorf("dump %s response.raw: %w", filepath.Base(turn), err)
				}
			}
			if evidence.Complete && strings.HasSuffix(responsePath, ".json") {
				if err := writeResponseContent(responsePath, filepath.Join(dest, "response_content.md")); err != nil {
					return fmt.Errorf("dump %s response_content.md: %w", filepath.Base(turn), err)
				}
			}
		}

		var reqMeta rawHTTPRequestMeta
		_ = readJSONFile(filepath.Join(turn, "request.meta.json"), &reqMeta)
		startedAt := firstNonEmptyRawHTTPDump(evidence.Meta.StartedAt, reqMeta.StartedAt)
		index = append(index, strings.Join([]string{
			turnNumber,
			filepath.Base(turn),
			filepath.ToSlash(filepath.Join("turn-"+turnNumber, "request.json")),
			filepath.ToSlash(strings.TrimPrefix(responsePath, outDir+string(os.PathSeparator))),
			responseState,
			fmt.Sprintf("%d", evidence.Meta.StatusCode),
			startedAt,
			evidence.Meta.CompletedAt,
		}, "\t"))
	}

	if err := os.WriteFile(filepath.Join(outDir, "index.tsv"), []byte(strings.Join(index, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	readme := fmt.Sprintf(`# Raw HTTP Turn Payloads

Source capture: %s
Turns exported: %d

This is a plain dump of captured provider HTTP payloads. It does not infer
orchestration phases, personas, checklist items, or completion state.

Each turn directory contains copied or pretty-printed captured files:
- request.json
- response.json for complete JSON responses, or response.raw for non-JSON or
  incomplete response bytes
- request.meta.json / response.meta.json when present
- request.headers.json / response.headers.json when present
- request_messages.md and response_content.md for readable inspection
`, captureDir, len(turns))
	if err := os.WriteFile(filepath.Join(outDir, "README.md"), []byte(readme), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, outDir)
	return nil
}

func rawHTTPTurnDirs(captureDir string) ([]string, error) {
	entries, err := os.ReadDir(captureDir)
	if err != nil {
		return nil, err
	}
	var turns []string
	for _, entry := range entries {
		if entry.IsDir() && isRawHTTPTurnDir(entry.Name()) {
			turns = append(turns, filepath.Join(captureDir, entry.Name()))
		}
	}
	sort.Slice(turns, func(i, j int) bool {
		return rawHTTPTurnNumber(filepath.Base(turns[i])) < rawHTTPTurnNumber(filepath.Base(turns[j]))
	})
	return turns, nil
}

func rawHTTPTurnNumber(name string) string {
	if idx := strings.Index(name, "-"); idx > 0 {
		return name[:idx]
	}
	return name
}

func writePrettyJSONFile(src, dest string) error {
	var v any
	if err := readJSONFile(src, &v); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(dest, data, 0o644)
}

func copyOptionalPrettyJSON(src, dest string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := writePrettyJSONFile(src, dest); err == nil {
		return nil
	} else if !errorsIsJSONSyntax(err) {
		return err
	}
	return copyFile(src, dest)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func errorsIsJSONSyntax(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr)
}

func writeRequestMessages(requestPath, dest string) error {
	var obj map[string]any
	if err := readJSONFile(requestPath, &obj); err != nil {
		return err
	}
	messages := asSlice(obj["messages"])
	var b strings.Builder
	for _, item := range messages {
		msg, _ := item.(map[string]any)
		role, _ := msg["role"].(string)
		content := readableContent(msg["content"])
		if role == "" && content == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", strings.ToUpper(role), content)
	}
	return os.WriteFile(dest, []byte(b.String()), 0o644)
}

func writeResponseContent(responsePath, dest string) error {
	var obj map[string]any
	if err := readJSONFile(responsePath, &obj); err != nil {
		return err
	}
	var b strings.Builder
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		msg, _ := ch["message"].(map[string]any)
		if content := readableContent(msg["content"]); content != "" {
			b.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				b.WriteString("\n")
			}
		}
	}
	return os.WriteFile(dest, []byte(b.String()), 0o644)
}

func readableContent(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func firstNonEmptyRawHTTPDump(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
