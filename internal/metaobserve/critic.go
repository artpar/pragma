package metaobserve

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// Finding is one critic observation about a session sample.
type Finding struct {
	Kind       string `json:"kind"` // waste | unchecked | spec_correction
	Finding    string `json:"finding"`
	Evidence   string `json:"evidence"`
	Confidence string `json:"confidence"` // low | medium | high
}

// validKinds are the finding kinds the operator's three questions admit.
var validKinds = map[string]bool{
	"waste":           true,
	"unchecked":       true,
	"spec_correction": true,
}

// Critic judges one session digest. The production implementation makes
// one small model call; tests use canned critics.
type Critic interface {
	Judge(ctx context.Context, digest string, queueTitles []string) ([]Finding, error)
}

// ProviderCritic runs the critic prompt through any provider.Provider.
type ProviderCritic struct {
	Prov      provider.Provider
	Model     string
	MaxTokens int
}

// criticSystem is the judge's role: the three operator questions and the
// output contract. It is deliberately provider-agnostic plain text.
const criticSystem = `You are the pragma meta-observer: a cheap, periodic critic that watches LIVE pragma sessions for operational waste that mechanical watchers (countable alerts: tool loops, retry storms, context %, stalls) cannot judge.

Answer exactly three questions about the sample you are given:
1. WASTED MOTION: what here is wasted motion — re-reads (the same file or command read again), stamp-noise, redundant re-derivations of already-known facts, low-value turns, context growth per unit of progress?
2. NOT CHECKING: what is this session NOT checking that it should verify before proceeding — claims made without evidence, assumptions not verified against the code, gates skipped?
3. SPEC CORRECTIONS: do any recent OPERATOR messages read as spec corrections (changed requirements, directives, taste decisions) that are not yet covered by the EXISTING QUEUE ITEMS?

Rules:
- Report only what is concretely visible in the sample; quote or point at the evidence.
- Do NOT report anything the EXISTING QUEUE ITEMS already cover.
- Sessions legitimately re-read files when verifying changes; report re-reads only when they carry no new purpose.
- If nothing is notable, return an empty array.

Return ONLY a JSON array (no prose, no code fence), at most 5 items:
[{"kind":"waste|unchecked|spec_correction","finding":"one concise sentence","evidence":"short quote or reference from the sample","confidence":"low|medium|high"}]`

// CriticPrompt builds the system and user halves of the critic request.
func CriticPrompt(digest string, queueTitles []string) (system, user string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	if len(queueTitles) > 0 {
		observe.GlobalTrace("if: len(queueTitles) > 0")
		b.WriteString("EXISTING QUEUE ITEMS (do not duplicate these):\n")
		for _, t := range queueTitles {
			observe.GlobalTrace("range queueTitles")
			b.WriteString("- " + truncateRunes(t, 110) + "\n")
		}
	} else {
		observe.GlobalTrace("else: len(queueTitles) > 0")
		b.WriteString("EXISTING QUEUE ITEMS: none readable.\n")
	}
	b.WriteString("\nSESSION TAIL DIGEST:\n")
	b.WriteString(digest)
	observe.GlobalTrace("return: criticSystem, b.String()")
	return criticSystem, b.String()
}

// Judge runs one small model call over the digest and parses findings.
func (c *ProviderCritic) Judge(ctx context.Context, digest string, queueTitles []string) ([]Finding, error) {
	observe.TraceCtx(ctx, "metaobserve", "ProviderCritic.Judge", "enter")
	defer observe.TraceCtx(ctx, "metaobserve", "ProviderCritic.Judge", "exit")
	system, user := CriticPrompt(digest, queueTitles)
	params := provider.RequestParams{
		Model:     c.Model,
		MaxTokens: c.MaxTokens,
		Messages: []model.Message{{
			Role:    model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: user}},
		}},
		System: model.SystemPrompt{Blocks: []model.SystemBlock{{Text: system}}},
	}
	resp, err := c.Prov.Complete(ctx, params)
	if err != nil {
		observe.TraceCtx(ctx, "metaobserve", "ProviderCritic.Judge", "if: err != nil")
		observe.TraceCtx(ctx, "metaobserve", "ProviderCritic.Judge", "return: nil, fmt.Errorf(\"critic call: %w\", err)")
		return nil, fmt.Errorf("critic call: %w", err)
	}
	observe.TraceCtx(ctx, "metaobserve", "ProviderCritic.Judge", "return: ParseFindings(responseText(resp)), nil")
	return ParseFindings(responseText(resp)), nil
}

// responseText concatenates the text parts of a response.
func responseText(resp model.Response) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, part := range resp.Content {
		observe.GlobalTrace("range resp.Content")
		if tp, ok := part.(model.TextPart); ok {
			observe.GlobalTrace("if: ok")
			b.WriteString(tp.Text)
		}
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// ParseFindings extracts the findings JSON array from critic text,
// tolerating surrounding prose, and normalizes/bounds the result.
func ParseFindings(text string) []Finding {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		observe.GlobalTrace("if: start < 0 || end <= start")
		observe.GlobalTrace("return: nil")
		return nil
	}
	var raw []Finding
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	out := make([]Finding, 0, len(raw))
	for _, f := range raw {
		observe.GlobalTrace("range raw")
		if !validKinds[f.Kind] {
			observe.GlobalTrace("if: !validKinds[f.Kind]")
			continue
		}
		f.Finding = truncateRunes(strings.TrimSpace(f.Finding), 400)
		f.Evidence = truncateRunes(strings.TrimSpace(f.Evidence), 300)
		f.Confidence = strings.ToLower(strings.TrimSpace(f.Confidence))
		switch f.Confidence {
		case "low", "medium", "high":
			observe.GlobalTrace("case: \"low\", \"medium\", \"high\"")
		default:
			observe.GlobalTrace("default")
			f.Confidence = "low"
		}
		if f.Finding == "" {
			observe.GlobalTrace("if: f.Finding == \"\"")
			continue
		}
		out = append(out, f)
		if len(out) >= 5 {
			observe.GlobalTrace("if: len(out) >= 5")
			break
		}
	}
	observe.GlobalTrace("return: out")
	return out
}
