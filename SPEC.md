# gogent — Entity Model & Relationships

## Context

Before writing code, we need the complete domain model: entities, relationships, processes, and how they compose. Critically, the system must be **LLM-generic** — work with Anthropic, OpenAI, Google, or any future provider. The current SPEC.md has Anthropic-specific types (`api.ContentBlock`, `api.StreamEvent`, `api.Client`). This plan replaces that with a provider-agnostic core.

**The rule**: Translation to/from any LLM wire format happens ONLY inside Provider adapters. Every other package uses internal model types exclusively.

---

## Part 1: LLM API Differences (Research)

These differences drive the entity design:

| Aspect | Anthropic | OpenAI | Google Gemini |
|---|---|---|---|
| **System prompt** | separate `system` field | `{role: "system"}` message | `systemInstruction` field |
| **Tool call** | content block `{type: "tool_use", id, name, input}` | `tool_calls[{id, function:{name, arguments:string}}]` | part `{functionCall:{name, args}}` |
| **Tool result** | `{type: "tool_result", tool_use_id}` in USER msg | `{role: "tool", tool_call_id}` separate msg | `{role: "function"}` with `functionResponse` |
| **Stop signal** | `end_turn` / `tool_use` | `stop` / `tool_calls` | `STOP` / `FUNCTION_CALL` |
| **Streaming** | SSE: message_start/content_block_delta/etc | SSE: choices[0].delta | REST streaming |
| **Tool schema** | `{name, description, input_schema}` | `{type:"function", function:{name, description, parameters}}` | `{functionDeclarations:[{name, description, parameters}]}` |
| **Thinking** | `{type: "thinking"}` content block | `reasoning_content` (o-series) | `thinkingConfig` |
| **Tool call IDs** | `toolu_xxx` | `call_xxx` | none (positional) |
| **Tool input format** | JSON object | JSON string (needs parse) | JSON object |

---

## Part 2: Package Layout (Revised)

```
internal/
  model/              ← NEW: Provider-agnostic domain types (the core)
    content.go        ← ContentPart sealed interface + 5 variants
    message.go        ← Message, Role, SystemPrompt
    conversation.go   ← Conversation (ordered messages + metadata)
    tool.go           ← ToolDef (name + schema for API requests)
    response.go       ← Response (normalized LLM output)
    usage.go          ← TokenUsage, Pricing, CostTracker
    stop.go           ← StopReason enum
    agent.go          ← Agent definition

  provider/           ← NEW: Provider interface + streaming types
    provider.go       ← Provider interface, RequestParams, StreamChunk, Feature
    anthropic/        ← Anthropic adapter (translates model ↔ Anthropic wire)
    openai/           ← OpenAI adapter (translates model ↔ OpenAI wire)
    google/           ← Google adapter (translates model ↔ Google wire)

  tool/               ← Tool system (UNCHANGED interface, references model/)
  tools/              ← Tool implementations (UNCHANGED)
  query/              ← Agentic loop (references model/ + provider/)
  permission/         ← Permission system (UNCHANGED)
  app/                ← AppState + StateStore (references model/)
  tui/                ← Bubbletea TUI (references model/)
  config/             ← Settings + merge
  context/            ← System prompt builder
  session/            ← Persistence (references model/)
  task/               ← Background tasks
  mcp/                ← MCP client + tool adapter
  cli/                ← Cobra CLI wiring
  util/               ← Pure utilities
```

**Dependency rule**: Only `internal/provider/anthropic/`, `internal/provider/openai/`, `internal/provider/google/` know about wire formats. Everything else imports only `internal/model/` and `internal/provider/` (the interface).

---

## Part 3: Core Entities

### 3.1 ContentPart (sealed, 5 variants)

`internal/model/content.go`

```
type ContentType string
const (
    ContentText       ContentType = "text"
    ContentImage      ContentType = "image"
    ContentToolCall   ContentType = "tool_call"
    ContentToolResult ContentType = "tool_result"
    ContentThinking   ContentType = "thinking"
)

type ContentPart interface {
    contentPartSealed()
    PartType() ContentType
}
```

| Variant | Fields | JSON tags | Notes |
|---|---|---|---|
| `TextPart` | `Text string` | `json:"text"` | Plain text content |
| `ImagePart` | `MimeType string`, `Data []byte` | `json:"mime_type"`, `json:"data"` | Raw bytes, provider base64-encodes |
| `ToolCallPart` | `ID string`, `Name string`, `Input json.RawMessage` | `json:"id"`, `json:"name"`, `json:"input"` | ID is internal UUID, provider maps to wire ID |
| `ToolResultPart` | `ToolCallID string`, `Content string`, `IsError bool` | `json:"tool_call_id"`, `json:"content"`, `json:"is_error,omitempty"` | ToolCallID correlates to ToolCallPart.ID |
| `ThinkingPart` | `Text string` | `json:"text"` | Reasoning trace (if provider supports) |

Custom JSON marshal/unmarshal dispatches on `"type"` discriminator field.

**Key decisions**:
- `ToolCallPart.Input` is `json.RawMessage` (parsed JSON object). Anthropic sends object, OpenAI sends string — the provider parses it before emitting.
- `ImagePart.Data` is raw `[]byte`. Provider encodes to base64 or whatever format needed.
- `ToolCallPart.ID` is an internal UUID. Provider adapters maintain bidirectional ID mapping (internal ↔ wire).

### 3.2 Message

`internal/model/message.go`

```
type Role string
const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

type Message struct {
    ID        string        `json:"id"`
    Role      Role          `json:"role"`
    Content   []ContentPart `json:"content"`
    Timestamp time.Time     `json:"timestamp"`
    Flags     MessageFlags  `json:"flags,omitempty"`
}

type MessageFlags struct {
    IsInternal       bool `json:"is_internal,omitempty"`
    IsCompactSummary bool `json:"is_compact_summary,omitempty"`
    IsMeta           bool `json:"is_meta,omitempty"`
}
```

**Only 2 roles.** Tool results live as `ToolResultPart` inside a `RoleUser` message. The provider adapter handles translation:
- Anthropic: stays as-is (tool_result blocks in user message)
- OpenAI: extracts ToolResultParts into separate `{role: "tool"}` messages
- Google: extracts into `{role: "function"}` messages

**MessageFlags**: `IsInternal` messages are never sent to the LLM (progress, status). `IsCompactSummary` marks compaction summaries. `IsMeta` marks metadata injections.

### 3.3 SystemPrompt

```
type SystemPrompt struct {
    Blocks []SystemBlock `json:"blocks"`
}

type SystemBlock struct {
    Text      string `json:"text"`
    Cacheable bool   `json:"cacheable,omitempty"`
}
```

Provider-agnostic. Cacheable is a hint:
- Anthropic: `cache_control: {type: "ephemeral"}`
- OpenAI: ignored (or their caching mechanism)
- Google: context caching API

### 3.4 Conversation

`internal/model/conversation.go`

```
type Conversation struct {
    ID        string       `json:"id"`
    Messages  []Message    `json:"messages"`
    System    SystemPrompt `json:"system"`
    Model     string       `json:"model"`
    Provider  string       `json:"provider"`
    WorkDir   string       `json:"work_dir"`
    ParentID  string       `json:"parent_id,omitempty"`
    CreatedAt time.Time    `json:"created_at"`
    UpdatedAt time.Time    `json:"updated_at"`
}

func NewConversation(system SystemPrompt, model string, provider string, workDir string) Conversation
func (c *Conversation) Append(msg Message)
func (c Conversation) Fork(newID string) Conversation
func (c Conversation) APIMessages() []Message
```

- `Fork()` deep-copies messages for sub-agent isolation. Sets `ParentID`.
- `APIMessages()` filters out `IsInternal` messages — returns only what should go to the LLM.
- `Provider` field records which provider was used (for session restore + cost tracking).

### 3.5 ToolDef (for LLM requests)

`internal/model/tool.go`

```
type ToolDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"input_schema"`
}
```

This is what `tool.Registry.ToolDefs()` returns. The provider translates to its format:
- Anthropic: `{name, description, input_schema}` (direct)
- OpenAI: `{type: "function", function: {name, description, parameters}}`
- Google: `{functionDeclarations: [{name, description, parameters}]}`

### 3.6 Response (normalized LLM output)

`internal/model/response.go`

```
type Response struct {
    ID         string        `json:"id"`
    Model      string        `json:"model"`
    Content    []ContentPart `json:"content"`
    StopReason StopReason    `json:"stop_reason"`
    Usage      TokenUsage    `json:"usage"`
}
```

Every provider call resolves to this. The query engine never sees wire format.

### 3.7 StopReason

`internal/model/stop.go`

```
type StopReason string
const (
    StopEndTurn   StopReason = "end_turn"
    StopToolUse   StopReason = "tool_use"
    StopMaxTokens StopReason = "max_tokens"
    StopError     StopReason = "error"
)
```

Each provider maps its native stop reason:
- Anthropic: `end_turn→StopEndTurn`, `tool_use→StopToolUse`, `max_tokens→StopMaxTokens`
- OpenAI: `stop→StopEndTurn`, `tool_calls→StopToolUse`, `length→StopMaxTokens`
- Google: `STOP→StopEndTurn`, `FUNCTION_CALL→StopToolUse`, `MAX_TOKENS→StopMaxTokens`

### 3.8 TokenUsage + Pricing + CostTracker

`internal/model/usage.go`

```
type TokenUsage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type Pricing struct {
    InputPerMToken       float64 `json:"input_per_m_token"`
    OutputPerMToken      float64 `json:"output_per_m_token"`
    CacheCreatePerMToken float64 `json:"cache_create_per_m_token,omitempty"`
    CacheReadPerMToken   float64 `json:"cache_read_per_m_token,omitempty"`
}

type CostEntry struct {
    Timestamp time.Time  `json:"timestamp"`
    Model     string     `json:"model"`
    Provider  string     `json:"provider"`
    Usage     TokenUsage `json:"usage"`
    CostUSD   float64    `json:"cost_usd"`
}

type CostTracker struct { mu sync.Mutex; entries []CostEntry; totalUSD float64 }

func NewCostTracker() *CostTracker
func (ct *CostTracker) Record(model string, provider string, usage TokenUsage, pricing Pricing)
func (ct *CostTracker) TotalUSD() float64
func (ct *CostTracker) Snapshot() []CostEntry
```

Cache fields are zero for providers that don't support caching.

### 3.9 Agent

`internal/model/agent.go`

```
type Agent struct {
    ID           string   `json:"id"`
    Name         string   `json:"name"`
    Description  string   `json:"description"`
    Model        string   `json:"model"`
    Provider     string   `json:"provider,omitempty"`
    Tools        []string `json:"tools,omitempty"`
    SystemPrompt string   `json:"system_prompt,omitempty"`
}
```

Agents are definitions loaded from `.pragma/agents/`. At runtime, spawning an agent = fork conversation + create Engine with agent's config. The `Provider` field allows sub-agents to use a different LLM entirely.

---

## Part 4: Provider Interface (The Translation Boundary)

`internal/provider/provider.go`

### 4.1 Provider Interface

```
type Provider interface {
    Name() string
    Stream(ctx context.Context, params RequestParams) (<-chan StreamChunk, error)
    Complete(ctx context.Context, params RequestParams) (model.Response, error)
    SupportsFeature(feature Feature) bool
    Pricing(modelID string) model.Pricing
}

type Feature string
const (
    FeaturePrefixCaching Feature = "prefix_caching"
    FeatureThinking      Feature = "thinking"
    FeatureImages        Feature = "images"
    FeatureToolUse       Feature = "tool_use"
    FeatureStreaming      Feature = "streaming"
)
```

### 4.2 RequestParams (internal types IN)

```
type RequestParams struct {
    Model       string              `json:"model"`
    MaxTokens   int                 `json:"max_tokens"`
    Messages    []model.Message     `json:"messages"`
    System      model.SystemPrompt  `json:"system"`
    Tools       []model.ToolDef     `json:"tools,omitempty"`
    Temperature *float64            `json:"temperature,omitempty"`
    Thinking    *ThinkingConfig     `json:"thinking,omitempty"`
}

type ThinkingConfig struct {
    Enabled      bool `json:"enabled"`
    BudgetTokens int  `json:"budget_tokens,omitempty"`
}
```

### 4.3 StreamChunk (internal types OUT)

```
type StreamChunk struct {
    TextDelta         string
    ThinkingDelta     string
    ToolCallStart     *model.ToolCallPart
    ToolCallInputDelta *ToolCallDelta
    Done              *StreamDone
    Error             error
}

type ToolCallDelta struct {
    ToolCallID string
    JSONDelta  string
}

type StreamDone struct {
    StopReason model.StopReason
    Usage      model.TokenUsage
}
```

**Flat struct, not sealed interface.** Only one field is non-zero per chunk. Cheaper than interface boxing for high-frequency channel values.

### 4.4 ID Mapping (inside each provider adapter)

```
// Inside internal/provider/anthropic/
type IDMapper struct {
    mu             sync.RWMutex
    internalToWire map[string]string  // our UUID → toolu_xxx
    wireToInternal map[string]string  // toolu_xxx → our UUID
}

func (m *IDMapper) ToWire(internalID string) string
func (m *IDMapper) ToInternal(wireID string) string
func (m *IDMapper) RegisterPair(internalID string, wireID string)
```

When provider receives tool call from LLM with wire ID `toolu_abc`:
1. Generate internal UUID
2. Register pair
3. Emit `ToolCallPart{ID: internalUUID}`

When sending tool result back:
1. Look up wire ID from internal UUID
2. Send `{tool_use_id: "toolu_abc"}` in wire format

For Google (no explicit IDs): synthesize wire ID from `{name}_{index}`.

---

## Part 5: Entity Relationships

```
┌─────────────┐
│    Agent     │ (definition, loadable from .pragma/agents/)
│ name, model  │
│ tools, prompt│
└──────┬──────┘
       │ spawns (forks conversation + creates Engine)
       ▼
┌─────────────────────────────────────────────────────┐
│                     Engine                           │
│ provider: Provider ←── translation boundary          │
│ registry: *Registry                                  │
│ orchestrator: *Orchestrator                          │
│ conversation: Conversation                           │
│ costTracker: *CostTracker                           │
│                                                      │
│ Run(ctx, userMessage) → <-chan LoopEvent             │
└───────────┬─────────────────────────┬───────────────┘
            │                         │
            ▼                         ▼
┌───────────────────┐    ┌─────────────────────────┐
│   Conversation    │    │     Provider             │
│                   │    │ (interface)              │
│ ID, ParentID?     │    │                          │
│ System: SystemPrompt   │ Stream(RequestParams)    │
│ Messages: []Message│   │  → <-chan StreamChunk    │
│ Model, Provider   │    │                          │
│ WorkDir           │    │ Implementations:         │
│                   │    │ ├─ anthropic.Provider    │
│ Fork() → new Conv │    │ ├─ openai.Provider      │
│ Append(msg)       │    │ └─ google.Provider      │
│ APIMessages()     │    └─────────────────────────┘
└───────┬───────────┘
        │ contains
        ▼
┌──────────────────────────────────────────────────┐
│                   Message                         │
│ ID (UUID), Role (user|assistant), Timestamp       │
│ Flags: {IsInternal, IsCompactSummary, IsMeta}    │
│ Content: []ContentPart                            │
│   ├─ TextPart{Text}                              │
│   ├─ ImagePart{MimeType, Data}                   │
│   ├─ ToolCallPart{ID, Name, Input}  ──────┐     │
│   ├─ ToolResultPart{ToolCallID, Content} ◄─┘     │
│   └─ ThinkingPart{Text}             (correlates) │
└──────────────────────────────────────────────────┘

┌──────────────────┐     ┌──────────────────────────┐
│   tool.Registry  │     │   tool.Descriptor        │
│                  │     │                          │
│ tools: map[name] ├────►│ Name, InputSchema        │
│                  │     │ Invoke(ctx, json, snap)  │
│ ToolDefs() →     │     │ CheckPerm(ctx, json, chk)│
│  []model.ToolDef │     │ Flags: readonly,         │
└──────────────────┘     │   concurrent, destructive│
                         └──────────────────────────┘

┌──────────────────────────────────────────────────┐
│              tool.Orchestrator                     │
│                                                    │
│ Execute(ctx, []ToolCallPart, snap)                │
│   → []ToolResultPart                              │
│                                                    │
│ 1. Partition: concurrent[] vs serial[]            │
│ 2. Run concurrent via errgroup.Group              │
│ 3. Run serial sequentially                        │
│ 4. For each: CheckPerm → Invoke → wrap result    │
└──────────────────────────────────────────────────┘
```

### Correlation Chain

```
ToolCallPart.ID (internal UUID)
    │
    ├─ Provider maps to wire ID (toolu_xxx / call_xxx / positional)
    │   via IDMapper inside provider adapter
    │
    └─ ToolResultPart.ToolCallID matches ToolCallPart.ID
        │
        └─ Orchestrator returns results in same order as calls
```

---

## Part 6: Processes (Step-by-Step)

### 6.1 Agentic Loop

```
Engine.Run(ctx, userMessage) → <-chan LoopEvent:

  goroutine:
    1. Create user Message{Role: User, Content: [TextPart{userMessage}]}
    2. Append to conversation
    3. LOOP:
       a. Build RequestParams{
            Model: config.Model,
            MaxTokens: config.MaxTokens,
            Messages: conversation.APIMessages(),  // filters IsInternal
            System: conversation.System,
            Tools: registry.ToolDefs(),
            Temperature: config.Temperature,
            Thinking: config.Thinking,
          }
       b. chunks, err := provider.Stream(ctx, params)
       c. Accumulate chunks into Response:
          - TextDelta → emit TextEvent, accumulate text
          - ThinkingDelta → emit ThinkingEvent, accumulate thinking
          - ToolCallStart → start accumulator for this tool call
          - ToolCallInputDelta → append JSON to accumulator
          - Done → finalize Response
          - Error → emit ErrorEvent, break
       d. Build assistant Message from Response.Content
       e. Append to conversation
       f. costTracker.Record(usage, pricing)
       g. If StopReason == EndTurn or MaxTokens:
          → emit TurnCompleteEvent, break
       h. If StopReason == ToolUse:
          → Extract []ToolCallPart from assistant message
          → Emit ToolCallEvent for each
          → results := orchestrator.Execute(ctx, toolCalls, stateSnapshot)
          → Emit ToolResultEvent for each
          → Build user Message with ToolResultParts
          → Append to conversation
          → Continue LOOP
    4. Close channel
```

### 6.2 Tool Execution

```
Orchestrator.Execute(ctx, calls []ToolCallPart, snap) → []ToolResultPart:

  1. PARTITION by registry lookup:
     concurrent = calls where Descriptor.Concurrent == true
     serial     = calls where Descriptor.Concurrent == false

  2. RUN CONCURRENT (errgroup.Group):
     For each call in concurrent:
       g.Go → executeSingle(ctx, call, snap)
     g.Wait()

  3. RUN SERIAL:
     For each call in serial:
       executeSingle(ctx, call, snap)

  4. RETURN all results (order preserved)

  executeSingle(ctx, call, snap) → ToolResultPart:
    a. desc := registry.Get(call.Name)
       If not found → ToolResultPart{ToolCallID: call.ID, Content: "unknown tool", IsError: true}
    b. decision := desc.CheckPerm(ctx, call.Input, permChecker)
       If Deny → ToolResultPart{..., Content: "permission denied", IsError: true}
       If Ask → prompt user via TUI; if denied → error result
    c. output, err := desc.Invoke(ctx, call.Input, snap)
       If err → ToolResultPart{..., Content: err.Error(), IsError: true}
    d. Return ToolResultPart{ToolCallID: call.ID, Content: output}
```

### 6.3 Compaction

```
Compact(conversation, budgetTokens, provider) → Conversation:

  1. est := conversation.TokenEstimate()
  2. If est <= budgetTokens → return (no-op)
  3. Protected: first message + last 4 messages
  4. Compactable: messages[1 : len-4]
  5. Build summary request via provider.Complete():
     "Summarize this conversation concisely: ..."
  6. Replace compactable range with:
     Message{Role: User, Content: [TextPart{"[Previous summary]: " + summary}],
             Flags: {IsCompactSummary: true}}
  7. Return compacted conversation
```

Uses the same Provider interface — works with any LLM.

### 6.4 Caching

```
Caching is a provider-level optimization:

  1. SystemBlock.Cacheable = true → hint to provider
  2. Provider translates:
     - Anthropic: adds cache_control: {type: "ephemeral"}
     - OpenAI: uses implicit prefix caching (no action needed)
     - Google: uses context caching API
  3. Usage.CacheCreationInputTokens / CacheReadInputTokens reported back
  4. CostTracker records savings

Internal model doesn't change. Caching is transparent.
```

### 6.5 Streaming (Chunk → Response Accumulation)

```
accumulateStream(chunks <-chan StreamChunk) → Response:

  textBuf := strings.Builder{}
  thinkBuf := strings.Builder{}
  toolCalls := map[string]*accumulator{}  // keyed by internal UUID
  var usage TokenUsage
  var stopReason StopReason

  for chunk := range chunks:
    if chunk.TextDelta != "":       textBuf.WriteString(chunk.TextDelta)
    if chunk.ThinkingDelta != "":   thinkBuf.WriteString(chunk.ThinkingDelta)
    if chunk.ToolCallStart != nil:  toolCalls[tc.ID] = new accumulator
    if chunk.ToolCallInputDelta:    toolCalls[id].inputBuf.WriteString(delta)
    if chunk.Done != nil:           usage, stopReason = chunk.Done values
    if chunk.Error != nil:          return error

  Build []ContentPart from accumulators
  Return Response{Content, StopReason, Usage}
```

### 6.6 Session Persistence

```
Session{
  ID, CreatedAt, UpdatedAt
  Conversation: model.Conversation  // stored in internal format, not wire format
  CostEntries: []CostEntry
}

Save: JSON marshal Conversation (ContentPart dispatched on "type" field)
Load: JSON unmarshal → Conversation with full ContentPart types restored
Resume: Load session → create Engine with session.Conversation.Provider → continue

Sessions stored in internal format = theoretically portable across providers.
```

### 6.7 Sub-Agent Spawning

```
SpawnSubAgent(parent Conversation, agent Agent, providerFactory, registry):

  1. child := parent.Fork(newID)            // deep copy + new ID + parent link
     OR child := NewConversation(...)        // clean slate for isolated agents
  2. child.Model = agent.Model
  3. child.Provider = agent.Provider (or parent's)
  4. child.System = SystemPrompt{Blocks: [{Text: agent.SystemPrompt}]}
  5. scopedReg := registry.Scoped(agent.Tools)  // only allowed tools
  6. prov := providerFactory(agent.Provider, agent.Model)
  7. engine := NewEngine(prov, scopedReg, orchestrator, store, config)
  8. return engine.Run(ctx, task)

Sub-agent can use a completely different LLM provider + model.
Results flow back as ToolResultPart in parent conversation.
```

---

## Part 7: What Changes from Current SPEC.md

| Current (Anthropic-specific) | New (LLM-generic) |
|---|---|
| `internal/api/` package | Split → `internal/model/` + `internal/provider/` |
| `api.ContentBlock` (tool_use, tool_result) | `model.ContentPart` (ToolCallPart, ToolResultPart) |
| `api.StreamEvent` (Anthropic SSE types) | `provider.StreamChunk` (flat struct, one field set) |
| `api.Client.Stream()` | `provider.Provider.Stream()` |
| `api.MessageParams` | `provider.RequestParams` |
| `api.SystemBlock{CacheControl}` | `model.SystemBlock{Cacheable bool}` |
| `api.ToolParam` | `model.ToolDef` |
| `api.Message{Role, Content}` | `model.Message{ID, Role, Content, Timestamp, Flags}` |
| No stop reason normalization | `model.StopReason` enum mapped per provider |
| No ID mapping | `IDMapper` inside each provider adapter |
| Hardcoded Anthropic client | `provider.Provider` interface, pluggable |
| Session stores API format | Session stores `model.Conversation` (portable) |

---

## Part 8: Persistence of THIS Plan

On exit plan mode, write this to:
1. **`/SPEC.md`** in repo — overwrite Part 3 (Package Architecture) with the entity model
2. **`.pragma/AGENT.md`** — add entity model reference
3. **Memory** — update project memory with "LLM-generic, see SPEC.md"

---

## Verification

- JSON round-trip tests for every ContentPart variant (marshal → unmarshal → compare)
- ID mapping tests: internal UUID ↔ wire ID for each provider
- Provider adapter tests: model types → wire format → model types
- Agentic loop test: mock provider → verify ToolCallPart/ToolResultPart correlation
- Compaction test: conversation exceeds budget → compact → verify token reduction
- Session test: save conversation with mixed ContentParts → restore → verify equality
- Multi-provider test: same conversation played through Anthropic and OpenAI adapters
