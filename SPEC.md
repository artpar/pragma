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
| `ThinkingPart` | `Text string`, `Signature string`, `Redacted bool`, `RedactedData string` | `json:"text"`, `json:"signature,omitempty"`, `json:"redacted,omitempty"`, `json:"redacted_data,omitempty"` | Reasoning trace + provider attestation. Redacted=true for provider-redacted blocks; RedactedData contains opaque encrypted data to send back verbatim |

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
    StopPauseTurn StopReason = "pause_turn"
    StopError     StopReason = "error"
)
```

Each provider maps its native stop reason:
- Anthropic: `end_turn→StopEndTurn`, `tool_use→StopToolUse`, `max_tokens→StopMaxTokens`, `pause_turn→StopPauseTurn`
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
    Pricing(modelID string) (model.Pricing, bool)  // ADR-012: Go map-lookup pattern
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
    Model      string
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
│   └─ ThinkingPart{Text, Redacted}   (correlates) │
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
# gogent — Observability, Debugging, Replayability, Testing

## Context

Pragma has ~45,800 GitHub issues. Common pain points:
- MCP servers hang 16+ hours with zero log output and 70 zombie processes
- Permission denial is literally ignored — user says "No", tool executes anyway, no audit trail
- 70% quota consumed in an hour with zero mid-session visibility
- `.pragma/settings.json` corruption from race conditions reported 8+ times, all closed without fix
- OTel broken for 6 consecutive versions and nobody noticed
- Debug mode is all-or-nothing — no granular control
- Sessions can't be replayed to reproduce bugs

**The root cause**: Ad-hoc logging scattered across the codebase. No single event backbone. No structured audit trail. No replay mechanism. No regression testing from real sessions.

**Our solution**: **Event Sourcing as the observability backbone.** Every significant action produces a typed Event. Events are simultaneously logged, persisted (for replay), aggregated (for metrics), and audited (for permission trails). One mechanism, four purposes.

---

## Part 1: The Event System

### 1.1 Core Concept

Every boundary crossing in the system emits a typed Event:
- API call starts/ends
- Tool permission checked
- Tool execution starts/ends
- Message appended to conversation
- Compaction triggered
- MCP server connected/disconnected
- Sub-agent spawned/completed
- Error occurred
- Session saved/restored

Events flow through an **EventBus**. Subscribers process them for different purposes:
- **Logger** → structured log output (file, stderr, TUI)
- **Recorder** → session replay file (deterministic reproduction)
- **Metrics** → live counters (token usage, latency, cost)
- **Auditor** → permission audit trail
- **TUI** → real-time display updates

### 1.2 Event Types (sealed interface)

`internal/observe/event.go`

```
type Event interface {
    eventSealed()
    EventKind() string
    Timestamp() time.Time
    TraceID() string        // correlates events across a single user turn
    SpanID() string         // correlates events within a single operation
    ParentSpanID() string   // for nested operations (sub-agent, tool-in-tool)
}

type EventHeader struct {
    Kind         string    `json:"kind"`
    Time         time.Time `json:"time"`
    Trace        string    `json:"trace_id"`
    Span         string    `json:"span_id"`
    ParentSpan   string    `json:"parent_span_id,omitempty"`
    AgentID      string    `json:"agent_id,omitempty"`
}
```

All events embed `EventHeader` and implement the sealed interface.

### 1.3 Event Catalog (every event type, exhaustive)

**Conversation Events:**

| Event | Key Fields | When |
|---|---|---|
| `ConversationStarted` | `ConversationID, Model, Provider, WorkDir` | New conversation created |
| `MessageAppended` | `MessageID, Role, ContentTypes[]string, TokenEstimate int` | Any message added |
| `ConversationForked` | `ParentConvID, ChildConvID, AgentName` | Sub-agent fork |

**API Events:**

| Event | Key Fields | When |
|---|---|---|
| `APIRequestStarted` | `Model, MessageCount, ToolCount, TokenEstimate` | Before provider.Stream() |
| `APIStreamChunk` | `ChunkType string, BytesDelta int` | Each StreamChunk (throttled: 1/sec max) |
| `APIRequestCompleted` | `StopReason, Usage, DurationMs, Model` | Stream fully consumed |
| `APIRequestFailed` | `ErrorType, ErrorMessage, Retryable bool, Attempt int` | API error |
| `APIRetryScheduled` | `Attempt, DelayMs, Reason` | Before retry sleep |

**Tool Events:**

| Event | Key Fields | When |
|---|---|---|
| `ToolCallReceived` | `ToolCallID, ToolName, InputSizeBytes` | Tool use block parsed from response |
| `ToolPermissionChecked` | `ToolCallID, ToolName, Decision string, Rule string, Source string` | Permission evaluated |
| `ToolPermissionPrompted` | `ToolCallID, ToolName, UserDecision string, DurationMs` | User prompted, response recorded |
| `ToolExecutionStarted` | `ToolCallID, ToolName, Concurrent bool` | Execution begins |
| `ToolExecutionCompleted` | `ToolCallID, ToolName, DurationMs, OutputSizeBytes, IsError bool` | Execution ends |
| `ToolExecutionFailed` | `ToolCallID, ToolName, ErrorType, ErrorMessage` | Tool threw error |
| `ToolBatchStarted` | `ConcurrentCount, SerialCount, TotalCount` | Orchestrator begins batch |
| `ToolBatchCompleted` | `TotalDurationMs, ConcurrentDurationMs, SerialDurationMs` | Orchestrator ends batch |

**Compaction Events:**

| Event | Key Fields | When |
|---|---|---|
| `CompactionStarted` | `PreTokenCount, BudgetTokens, MessageCount` | Token budget exceeded |
| `CompactionCompleted` | `PostTokenCount, SummarizedCount, DurationMs` | Compaction done |
| `CompactionFailed` | `ErrorType, ErrorMessage` | Compaction error |

**MCP Events:**

| Event | Key Fields | When |
|---|---|---|
| `MCPServerConnecting` | `ServerName, Transport string` | Connection attempt |
| `MCPServerConnected` | `ServerName, ToolCount, DurationMs` | Connected, tools loaded |
| `MCPServerDisconnected` | `ServerName, Reason string` | Connection lost/closed |
| `MCPServerFailed` | `ServerName, ErrorType, ErrorMessage` | Connection failed |
| `MCPToolCallStarted` | `ServerName, ToolName, InputSizeBytes` | MCP tool invoked |
| `MCPToolCallCompleted` | `ServerName, ToolName, DurationMs, OutputSizeBytes` | MCP tool done |
| `MCPHealthCheck` | `ServerName, Status string, PID int, MemoryMB float64` | Periodic health |

**Session Events:**

| Event | Key Fields | When |
|---|---|---|
| `SessionStarted` | `SessionID, ResumedFrom string` | Session begins |
| `SessionSaved` | `SessionID, MessageCount, FileSizeBytes` | Checkpoint saved |
| `SessionEnded` | `SessionID, DurationMs, TurnCount, TotalCostUSD` | Session ends |

**Agent Events:**

| Event | Key Fields | When |
|---|---|---|
| `SubAgentSpawned` | `AgentID, AgentName, Model, Provider, ParentAgentID` | Sub-agent created |
| `SubAgentCompleted` | `AgentID, DurationMs, TurnCount, Usage` | Sub-agent done |
| `SubAgentFailed` | `AgentID, ErrorType, ErrorMessage` | Sub-agent error |

**Error Events:**

| Event | Key Fields | When |
|---|---|---|
| `ErrorOccurred` | `Severity string, Component string, ErrorType, ErrorMessage, Stack string` | Any error |

**Permission Audit Events (the audit trail that pragma lacks):**

| Event | Key Fields | When |
|---|---|---|
| `PermissionRuleMatched` | `ToolName, Pattern, Source, Decision` | A rule matched |
| `PermissionEscalated` | `ToolName, FromDecision, ToDecision, Reason` | Decision overridden |
| `PermissionDenialEnforced` | `ToolCallID, ToolName, WasExecuted bool` | Denial actually stopped execution |

The `WasExecuted` field on `PermissionDenialEnforced` is the key — it catches the pragma bug where denial is ignored. If `Decision == Deny` but `WasExecuted == true`, the audit log screams.

---

## Part 2: EventBus

`internal/observe/bus.go`

```
type EventBus struct {
    subscribers []Subscriber
    mu          sync.RWMutex
    buffer      chan Event    // buffered channel, non-blocking emit
}

type Subscriber interface {
    HandleEvent(event Event)
}

func NewEventBus(bufferSize int) *EventBus
func (b *EventBus) Emit(event Event)
func (b *EventBus) Subscribe(sub Subscriber) func()
func (b *EventBus) Drain()   // flush buffer, call on shutdown
```

`Emit` is non-blocking — writes to buffered channel. A goroutine reads from the channel and fans out to subscribers. Events are never dropped (buffer sized generously, e.g. 10,000). If buffer fills, Emit blocks (backpressure — better than silent drop).

**Every component that does significant work receives `*EventBus` via constructor injection.** Not a global. Not a singleton.

---

## Part 3: Subscribers

### 3.1 Logger

`internal/observe/logger.go`

```
type Logger struct {
    writer   io.Writer       // file, stderr, or both
    level    Level           // minimum level to output
    topics   map[string]bool // topic filter (nil = all)
    format   Format          // json | text | compact
}

type Level int
const (
    LevelTrace Level = iota   // every StreamChunk, every byte
    LevelDebug                // API calls, tool executions, decisions
    LevelInfo                 // turns, tool results, costs
    LevelWarn                 // retries, slow operations, anomalies
    LevelError                // failures
)

type Format int
const (
    FormatJSON Format = iota  // structured JSONL (for machine parsing)
    FormatText                // human-readable with colors
    FormatCompact             // one-line summaries
)

func NewLogger(writer io.Writer, level Level, format Format, topics map[string]bool) *Logger
func (l *Logger) HandleEvent(event Event)
```

**Topic filtering**: `--debug=tool,api,mcp` shows only those event categories. Solves the all-or-nothing problem.

**Level mapping** (Event → Level):
- `APIStreamChunk` → Trace
- `ToolExecutionStarted`, `APIRequestStarted` → Debug
- `ToolExecutionCompleted`, `APIRequestCompleted`, `MessageAppended` → Info
- `APIRetryScheduled`, `CompactionStarted` → Warn
- `*Failed`, `ErrorOccurred` → Error

### 3.2 Recorder (for replay)

`internal/observe/recorder.go`

```
type Recorder struct {
    file     *os.File
    encoder  *json.Encoder
    mu       sync.Mutex
}

func NewRecorder(path string) (*Recorder, error)
func (r *Recorder) HandleEvent(event Event)
func (r *Recorder) Close() error
```

Writes every event as JSONL to a replay file. The file contains the complete sequence of events for deterministic replay.

**Replay file format** (`.gogent-replay.jsonl`):
```jsonl
{"kind":"ConversationStarted","time":"...","trace_id":"...","conversation_id":"...","model":"claude-sonnet-4-20250514"}
{"kind":"MessageAppended","time":"...","role":"user","content_types":["text"],"token_estimate":42}
{"kind":"APIRequestStarted","time":"...","model":"claude-sonnet-4-20250514","message_count":2,"tool_count":15}
{"kind":"APIStreamChunk","time":"...","chunk_type":"text_delta","bytes_delta":23}
{"kind":"APIRequestCompleted","time":"...","stop_reason":"tool_use","usage":{"input_tokens":1500,"output_tokens":200}}
{"kind":"ToolCallReceived","time":"...","tool_call_id":"uuid-1","tool_name":"Bash","input_size_bytes":45}
{"kind":"ToolPermissionChecked","time":"...","tool_call_id":"uuid-1","tool_name":"Bash","decision":"allow","rule":"allow:Bash(git *)","source":"settings"}
{"kind":"ToolExecutionStarted","time":"...","tool_call_id":"uuid-1","tool_name":"Bash"}
{"kind":"ToolExecutionCompleted","time":"...","tool_call_id":"uuid-1","tool_name":"Bash","duration_ms":234,"output_size_bytes":1200,"is_error":false}
```

### 3.3 Metrics Collector

`internal/observe/metrics.go`

```
type Metrics struct {
    mu               sync.RWMutex
    tokenUsage       model.TokenUsage   // accumulated
    totalCostUSD     float64
    turnCount        int
    toolCalls        map[string]int     // tool name → count
    toolDurations    map[string]int64   // tool name → total ms
    toolErrors       map[string]int     // tool name → error count
    apiCalls         int
    apiErrors        int
    apiTotalLatencyMs int64
    compactions      int
}

func NewMetrics() *Metrics
func (m *Metrics) HandleEvent(event Event)
func (m *Metrics) Snapshot() MetricsSnapshot
```

`Snapshot()` returns a value copy — displayed in TUI status bar. **Live token/cost visibility** that pragma lacks.

```
type MetricsSnapshot struct {
    TokenUsage       model.TokenUsage
    TotalCostUSD     float64
    TurnCount        int
    ToolCallCount    int
    ToolErrorCount   int
    APICallCount     int
    APIErrorCount    int
    AvgAPILatencyMs  int64
    Compactions      int
    SessionDurationMs int64
}
```

### 3.4 Auditor (permission trail)

`internal/observe/auditor.go`

```
type Auditor struct {
    mu      sync.Mutex
    entries []AuditEntry
}

type AuditEntry struct {
    Timestamp   time.Time `json:"timestamp"`
    ToolCallID  string    `json:"tool_call_id"`
    ToolName    string    `json:"tool_name"`
    Decision    string    `json:"decision"`      // allow|deny|ask
    UserResponse string   `json:"user_response"` // allow|deny (if asked)
    RuleMatched string    `json:"rule_matched"`
    RuleSource  string    `json:"rule_source"`
    WasExecuted bool      `json:"was_executed"`   // THE KEY FIELD
}

func NewAuditor() *Auditor
func (a *Auditor) HandleEvent(event Event)
func (a *Auditor) Trail() []AuditEntry    // full audit trail
func (a *Auditor) Violations() []AuditEntry  // entries where Decision==deny && WasExecuted==true
```

`Violations()` catches the exact bug pragma has — permission denial ignored.

---

## Part 4: Trace Correlation

Every user turn generates a **TraceID** (UUID). Every operation within that turn generates a **SpanID**. Nested operations (sub-agent spawns, tool-in-tool) set **ParentSpanID**.

```
TraceID: abc-123 (one per user turn)
├── SpanID: def-456 (API request)
│   └── ParentSpanID: (none, root span)
├── SpanID: ghi-789 (tool execution: Bash)
│   └── ParentSpanID: (none, root span)
├── SpanID: jkl-012 (tool execution: Agent)
│   └── ParentSpanID: (none, root span)
│   ├── SpanID: mno-345 (sub-agent API request)
│   │   └── ParentSpanID: jkl-012
│   └── SpanID: pqr-678 (sub-agent tool: FileRead)
│       └── ParentSpanID: jkl-012
```

This enables:
- "Show me everything that happened in turn 3" (filter by TraceID)
- "Show me why this tool call was slow" (follow SpanID chain)
- "Show me what the sub-agent did" (filter by AgentID)

---

## Part 5: Replay System

### 5.1 Recording

Recording is automatic when a Recorder subscriber is attached to the EventBus. CLI flag `--record` or env `GOGENT_RECORD=1` enables it.

But events alone aren't enough for replay — we also need the **tool outputs** (which are non-deterministic). The Recorder captures:

1. All events (including ToolExecutionCompleted with output hash)
2. Tool output snapshots (actual output content, stored separately to keep event log lean)

```
type RecordedToolOutput struct {
    ToolCallID string `json:"tool_call_id"`
    Output     string `json:"output"`
    IsError    bool   `json:"is_error"`
}
```

Replay file structure:
```
session-abc/
  events.jsonl              # all events
  tool-outputs/
    uuid-1.json             # Bash output
    uuid-2.json             # FileRead output
    ...
  api-responses/
    turn-1.json             # raw API response (for provider-level replay)
    turn-2.json
    ...
```

### 5.2 Replay Engine

`internal/observe/replay.go`

```
type ReplayEngine struct {
    events      []Event
    toolOutputs map[string]RecordedToolOutput
    apiResponses map[int]model.Response  // turn number → response
}

func LoadReplay(dir string) (*ReplayEngine, error)
func (r *ReplayEngine) Events() []Event
func (r *ReplayEngine) ToolOutput(toolCallID string) (string, bool)
func (r *ReplayEngine) APIResponse(turn int) (model.Response, bool)
```

### 5.3 Replay Modes

**Event replay** (fastest, for debugging):
- Walk through events, display in TUI or log format
- No actual execution — just view what happened
- `gogent replay --events session-abc/`

**Deterministic replay** (for regression testing):
- Mock provider returns recorded API responses
- Mock tools return recorded outputs
- Verify the system produces the same events
- `gogent replay --deterministic session-abc/`

**Partial replay** (for reproducing bugs):
- Replay up to turn N, then switch to live execution
- Use recorded conversation state as starting point
- `gogent replay --until-turn=5 --then-live session-abc/`

---

## Part 6: Testing Strategy

### 6.1 Test Harness

`internal/testing/` (not `internal/test/` which is a Go keyword)

```
type Harness struct {
    provider     *MockProvider
    bus          *observe.EventBus
    recorder     *observe.Recorder
    registry     *tool.Registry
    store        *app.StateStore[app.AppState]
    events       []observe.Event  // captured for assertions
}

func NewHarness() *Harness
func (h *Harness) WithTool(name string, handler func(json.RawMessage) (string, error)) *Harness
func (h *Harness) WithProviderResponse(responses ...model.Response) *Harness
func (h *Harness) Run(ctx context.Context, userMessage string) ([]observe.Event, error)
func (h *Harness) AssertEvent(kind string, matcher func(Event) bool) error
func (h *Harness) AssertNoEvent(kind string) error
func (h *Harness) AssertEventSequence(kinds ...string) error
```

### 6.2 Mock Provider

```
type MockProvider struct {
    responses []model.Response  // returns in order
    calls     []provider.RequestParams  // records what was sent
    idx       int
}

func NewMockProvider(responses ...model.Response) *MockProvider
func (m *MockProvider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error)
func (m *MockProvider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error)
func (m *MockProvider) Calls() []provider.RequestParams  // what was sent
```

`Stream` converts `model.Response` into `StreamChunk` sequence (TextDelta chunks + Done).

### 6.3 Scenario Testing (the key for complex cases)

Scenario files describe multi-turn agentic interactions:

`testdata/scenarios/multi_tool_turn.yaml`:
```yaml
name: "Multi-tool turn with permission denial"
model: "mock"
turns:
  - user: "List files and read README.md"
    provider_response:
      stop_reason: tool_use
      content:
        - type: tool_call
          name: Bash
          input: {"command": "ls"}
        - type: tool_call
          name: Read
          input: {"file_path": "README.md"}
    tool_results:
      - name: Bash
        output: "file1.go\nfile2.go\nREADME.md"
      - name: Read
        output: "# Project\nSome content"
    expect_events:
      - kind: ToolBatchStarted
        concurrent_count: 2
      - kind: ToolPermissionChecked
        tool_name: Bash
        decision: allow
      - kind: ToolPermissionChecked
        tool_name: Read
        decision: allow
      - kind: ToolBatchCompleted

  - provider_response:
      stop_reason: end_turn
      content:
        - type: text
          text: "Here are the files..."
    expect_events:
      - kind: TurnCompleteEvent
        stop_reason: end_turn
```

```
type Scenario struct {
    Name   string       `yaml:"name"`
    Model  string       `yaml:"model"`
    Turns  []ScenarioTurn `yaml:"turns"`
}

type ScenarioTurn struct {
    User             string                 `yaml:"user,omitempty"`
    ProviderResponse model.Response         `yaml:"provider_response"`
    ToolResults      map[string]string      `yaml:"tool_results,omitempty"`
    ExpectEvents     []EventMatcher         `yaml:"expect_events"`
    ExpectNoEvents   []string               `yaml:"expect_no_events,omitempty"`
}

type EventMatcher struct {
    Kind   string            `yaml:"kind"`
    Fields map[string]string `yaml:"fields,omitempty"` // field name → expected value
}

func RunScenario(t *testing.T, path string)
func RunAllScenarios(t *testing.T, dir string)
```

### 6.4 What Scenario Tests Catch

| Scenario | Tests | Pragma Bug It Would Catch |
|---|---|---|
| `permission_denial_enforced.yaml` | Deny tool → verify `WasExecuted=false` | Permission bypass (user says No, tool runs) |
| `mcp_timeout.yaml` | MCP tool hangs → verify timeout event after N seconds | 16-hour MCP hang with no output |
| `concurrent_tool_error.yaml` | One tool in batch fails → others get cancelled | Silent tool failures in concurrent batches |
| `compaction_trigger.yaml` | Token budget exceeded → compaction fires → tokens reduced | Invisible token explosion |
| `api_retry_backoff.yaml` | Rate limit → retry with backoff → success | Retry storms |
| `subagent_permission_scope.yaml` | Sub-agent tries parent-only tool → denied | Sub-agent permission escalation |
| `session_save_restore.yaml` | Save mid-conversation → restore → continue | Session corruption |
| `token_cost_tracking.yaml` | Multi-turn → verify cost accumulates correctly | Invisible cost explosion |

### 6.5 Replay-Based Regression Testing

When a bug is found in production:
1. User shares replay file (`gogent share-replay` → uploads events + outputs)
2. Replay file becomes a test case in `testdata/replays/`
3. CI runs all replay files deterministically
4. If system behavior changes, test fails
5. Bug can never regress

```
func TestReplays(t *testing.T) {
    replays, _ := filepath.Glob("testdata/replays/*/events.jsonl")
    for _, replay := range replays {
        t.Run(filepath.Base(filepath.Dir(replay)), func(t *testing.T) {
            engine := observe.LoadReplay(filepath.Dir(replay))
            RunDeterministicReplay(t, engine)
        })
    }
}
```

---

## Part 7: Health Monitoring

### 7.1 MCP Watchdog

`internal/observe/watchdog.go`

```
type MCPWatchdog struct {
    bus         *EventBus
    timeouts    map[string]time.Time  // server → last activity
    maxSilence  time.Duration         // default 30s
}

func NewMCPWatchdog(bus *EventBus, maxSilence time.Duration) *MCPWatchdog
func (w *MCPWatchdog) Start(ctx context.Context)
```

- Subscribes to MCP events on the bus
- If no activity from a server for `maxSilence`, emits `MCPHealthCheck` event with `Status: "unresponsive"`
- If process is dead, emits `MCPServerDisconnected` with `Reason: "process_exited"`
- Prevents the 16-hour zombie process problem

### 7.2 Token Budget Monitor

```
type TokenMonitor struct {
    bus        *EventBus
    budget     int
    current    int
    thresholds []float64  // e.g. [0.5, 0.8, 0.95]
    notified   map[float64]bool
}

func NewTokenMonitor(bus *EventBus, budget int, thresholds []float64) *TokenMonitor
```

Emits warnings when token usage crosses thresholds. TUI shows live token count.

---

## Part 8: CLI Integration

```
gogent                         # normal mode (Info level, compact format)
gogent --verbose               # Debug level, text format
gogent --debug                 # Trace level, JSON format to file
gogent --debug=tool,api        # Trace level, only tool+api topics
gogent --record                # enable replay recording
gogent replay <dir>            # replay a recorded session
gogent replay --deterministic  # replay for regression testing
gogent audit <session>         # show permission audit trail
gogent metrics <session>       # show metrics summary from replay file
```

---

## Part 9: Wiring (how it connects to existing architecture)

```
main.go:
  bus := observe.NewEventBus(10000)
  bus.Subscribe(observe.NewLogger(logWriter, level, format, topics))
  bus.Subscribe(observe.NewMetrics())
  bus.Subscribe(observe.NewAuditor())
  if recordMode { bus.Subscribe(observe.NewRecorder(replayDir)) }

  provider := anthropic.New(apiKey, bus)     // provider emits API events
  registry := tool.NewRegistry(bus)          // registry emits tool events
  orchestrator := tool.NewOrchestrator(registry, perms, bus)  // orchestrator emits batch events
  engine := query.NewEngine(provider, registry, orchestrator, store, config, bus)  // engine emits turn events

  // Every component receives *EventBus via constructor
  // Every component emits events at its boundaries
  // No ad-hoc logging — everything goes through events
```

---

## Part 10: Persistence of This Plan

On approval:
1. Append to `/SPEC.md` as new Part (Observability)
2. Update `.pragma/AGENT.md` with observability conventions
3. Create agile tasks for observe/ package implementation
4. Update memory with observability design reference

---

## Verification

- `go test ./internal/observe/` — EventBus delivers to all subscribers
- `go test ./internal/observe/ -run TestLogger` — level/topic filtering works
- `go test ./internal/observe/ -run TestRecorder` — events round-trip through JSONL
- `go test ./internal/observe/ -run TestAuditor` — permission violations detected
- `go test ./internal/observe/ -run TestMetrics` — token/cost accumulation correct
- Scenario test: `go test ./internal/testing/ -run TestScenarios` — all YAML scenarios pass
- Replay test: `go test ./internal/testing/ -run TestReplays` — all recorded sessions replay deterministically
- Watchdog test: mock MCP server goes silent → watchdog emits unresponsive event within maxSilence
- Integration: run `gogent --record -p "list files"` → replay file exists → `gogent replay --deterministic` passes
# gogent — Spec Enforcement: How Code Cannot Drift from Plan

## Context

A markdown spec is advisory. Nothing stops code from ignoring it. We need **enforcement mechanisms** that make violations either uncompilable or test failures. Two layers: Go compiler (interfaces/types) + architecture tests (scan for violations the compiler can't catch).

---

## Layer 1: Go Compiler Enforcement (violations won't compile)

### 1.1 Tool Interface — compiler forces every tool to have exactly these methods

The `Tool[I,O]` interface is defined in `internal/tool/tool.go`. Any struct registered as a tool MUST implement `Name()`, `Description()`, `Run()` or it won't compile. The `Register[I,O]()` generic function ensures `I` and `O` are concrete types — you can't register a tool with `interface{}` inputs.

### 1.2 Provider Interface — compiler forces every provider to translate correctly

`provider.Provider` requires `Stream()` and `Complete()` that accept `provider.RequestParams` (internal types) and return `provider.StreamChunk` / `model.Response` (internal types). A provider adapter that returns Anthropic wire types won't compile.

### 1.3 Sealed Interfaces — compiler prevents adding variants in wrong packages

`ContentPart`, `Event`, `LoopEvent` have unexported marker methods. External packages cannot add new variants — they must be defined in the package that owns the interface. Exhaustive `switch` catches missing cases.

### 1.4 EventBus as constructor parameter — compiler forces observability wiring

If `NewEngine()` requires `*observe.EventBus` as a parameter, you can't create an Engine without one. The compiler forces observability to be wired.

```
func NewEngine(provider Provider, registry *Registry, bus *observe.EventBus, ...) *Engine
func NewOrchestrator(registry *Registry, perms *Checker, bus *observe.EventBus) *Orchestrator
func NewRegistry(bus *observe.EventBus) *Registry
```

No EventBus → won't compile → can't skip observability.

### 1.5 Type safety on events — compiler prevents untyped logging

Events are typed structs. You can't emit a free-form string through the EventBus — it only accepts `observe.Event` implementations. This prevents `bus.Emit("some random debug string")`.

---

## Layer 2: Architecture Tests (violations fail CI)

`internal/archtest/` — tests that scan the codebase for structural violations.

### 2.1 Import Rules Test

```
TestNoDirectProviderImports:
  Scan all .go files outside internal/provider/
  FAIL if any file imports:
    - "internal/provider/anthropic"
    - "internal/provider/openai"
    - "internal/provider/google"
  ALLOWED: "internal/provider" (the interface package)
  Exception: cmd/gogent/main.go (wiring), internal/cli/ (factory)
```

This enforces the LLM-generic boundary. If `internal/query/engine.go` imports `internal/provider/anthropic`, the test fails.

### 2.2 No Ad-Hoc Logging Test

```
TestNoAdHocLogging:
  Scan all .go files in internal/ (excluding internal/observe/)
  FAIL if any file contains:
    - fmt.Print / fmt.Println / fmt.Printf (to stdout)
    - log.Print / log.Println / log.Printf
    - os.Stderr.Write (direct stderr)
  ALLOWED:
    - internal/observe/ (the logging package itself)
    - internal/tui/ (TUI output is not logging)
    - _test.go files (test output via t.Log is fine)
```

This enforces: all observability goes through EventBus.

### 2.3 Permission Audit Completeness Test

```
TestPermissionAuditCompleteness:
  Scan all .go files in internal/tool/ and internal/tools/
  For every call to CheckPerm or Evaluate:
    FAIL if no corresponding bus.Emit(ToolPermissionChecked{...}) within 5 lines
  For every permission denial:
    FAIL if no bus.Emit(PermissionDenialEnforced{...}) within 10 lines
```

This enforces: every permission check produces an audit event. The pragma bug (permission ignored with no audit trail) becomes structurally impossible.

### 2.4 Tool Registration Completeness Test

```
TestAllToolsRegistered:
  Scan all packages under internal/tools/
  For each package that defines a struct implementing Tool[I,O]:
    FAIL if that tool is not registered in the expected registration list
  Cross-reference against the registration calls in cmd/gogent/main.go or cli/wire.go
```

### 2.5 Package Dependency DAG Test

```
TestPackageDependencyDAG:
  Parse all imports in the codebase
  Build dependency graph

  FAIL if any of these illegal edges exist:
    internal/model/ → any internal/ package (model has no internal deps)
    internal/permission/ → any internal/ package except model/
    internal/tool/ → internal/query/ (tool cannot depend on engine)
    internal/tool/ → internal/tui/ (tool cannot depend on UI)
    internal/tools/* → internal/tools/* (tools cannot depend on each other)
    internal/query/ → internal/tui/ (engine cannot depend on UI)
    internal/provider/ → anything except internal/model/

  PASS if graph matches the DAG defined in AGENT.md
```

### 2.6 Struct Tag Discipline Test

```
TestToolInputStructTags:
  For every struct type used as a Tool input (type parameter I):
    For every exported field:
      FAIL if missing `json` tag
      FAIL if missing `desc` tag (every input field needs a description)
      WARN if missing `schema:"required"` on non-optional fields
```

This enforces: tool input structs have the tags that reflection needs for JSON Schema generation. A field without tags won't generate a proper schema.

### 2.7 Event Emission at Boundaries Test

```
TestEventEmissionAtBoundaries:
  For Provider.Stream calls:
    FAIL if no APIRequestStarted event emitted before call
    FAIL if no APIRequestCompleted/APIRequestFailed event emitted after call

  For Orchestrator.Execute calls:
    FAIL if no ToolBatchStarted event emitted before execution
    FAIL if no ToolBatchCompleted event emitted after execution

  For each tool execution:
    FAIL if no ToolExecutionStarted before Run()
    FAIL if no ToolExecutionCompleted/ToolExecutionFailed after Run()
```

### 2.8 No Dual Path Test

```
TestNoDualMessageTypes:
  Scan all .go files
  FAIL if any file defines a type with "Message" in the name
    outside of internal/model/message.go
  Exception: TUI message types (tea.Msg implementations in internal/tui/)

TestNoDualToolTypes:
  FAIL if any file defines a "Tool" interface outside internal/tool/tool.go

TestNoDualContentTypes:
  FAIL if any file defines ContentPart variants outside internal/model/content.go
```

---

## Layer 3: How Architecture Tests Run

### 3.1 Implementation Approach

Architecture tests use Go's `go/parser` and `go/ast` packages to scan source files. No external tools needed. Pure Go tests that read `.go` files and check patterns.

```
internal/archtest/
  imports_test.go         ← TestNoDirectProviderImports, TestPackageDependencyDAG
  logging_test.go         ← TestNoAdHocLogging
  audit_test.go           ← TestPermissionAuditCompleteness
  registration_test.go    ← TestAllToolsRegistered
  tags_test.go            ← TestToolInputStructTags
  boundaries_test.go      ← TestEventEmissionAtBoundaries
  singlesource_test.go    ← TestNoDualMessageTypes, TestNoDualToolTypes
```

### 3.2 When They Run

- `go test ./internal/archtest/` — manually
- Every commit (via git pre-commit hook if user enables it)
- CI (when CI is set up)
- Part of the phase gate checklist (before merging to main)

### 3.3 Helper Functions

```
func scanGoFiles(root string) []string
func parseImports(filePath string) []string
func findFunctionCalls(filePath string, funcName string) []Location
func findStringLiterals(filePath string, pattern string) []Location
func findStructsImplementing(root string, interfaceName string) []StructInfo
func findStructTags(structType *ast.StructType) map[string][]TagInfo
```

---

## Summary: What Prevents Drift

| Risk | Enforcement | Layer |
|---|---|---|
| Tool missing required methods | `Tool[I,O]` interface — won't compile | Compiler |
| Provider leaking wire types | `Provider` interface returns `model.*` only — won't compile | Compiler |
| New ContentPart variant in wrong package | Sealed interface (unexported marker) — won't compile | Compiler |
| Engine created without EventBus | Constructor requires `*EventBus` — won't compile | Compiler |
| Untyped log message through EventBus | `Emit(Event)` only accepts typed events — won't compile | Compiler |
| Direct import of anthropic/ from query/ | `TestNoDirectProviderImports` — test fails | Arch test |
| `fmt.Println` debug logging | `TestNoAdHocLogging` — test fails | Arch test |
| Permission check without audit event | `TestPermissionAuditCompleteness` — test fails | Arch test |
| Tool not registered | `TestAllToolsRegistered` — test fails | Arch test |
| Circular package dependency | `TestPackageDependencyDAG` — test fails | Arch test |
| Tool input missing schema tags | `TestToolInputStructTags` — test fails | Arch test |
| Boundary crossing without event emission | `TestEventEmissionAtBoundaries` — test fails | Arch test |
| Duplicate Message/Tool/Content types | `TestNoDual*Types` — test fails | Arch test |

**13 enforcement rules. 5 caught by compiler. 8 caught by architecture tests. Zero drift.**

---

## Persistence

On approval:
1. Append to `SPEC.md` as Part 3 (Enforcement)
2. Add `internal/archtest/` package directory
3. Update `AGENT.md` with enforcement conventions
4. Update agile tasks for archtest implementation

---

# gogent — Observability, Debugging, Replayability, Testing

## Context

Pragma has ~45,800 GitHub issues. Common pain points:
- MCP servers hang 16+ hours with zero log output and 70 zombie processes
- Permission denial is literally ignored — user says "No", tool executes anyway, no audit trail
- 70% quota consumed in an hour with zero mid-session visibility
- `.pragma/settings.json` corruption from race conditions reported 8+ times, all closed without fix
- OTel broken for 6 consecutive versions and nobody noticed
- Debug mode is all-or-nothing — no granular control
- Sessions can't be replayed to reproduce bugs

**The root cause**: Ad-hoc logging scattered across the codebase. No single event backbone. No structured audit trail. No replay mechanism. No regression testing from real sessions.

**Our solution**: **Event Sourcing as the observability backbone.** Every significant action produces a typed Event. Events are simultaneously logged, persisted (for replay), aggregated (for metrics), and audited (for permission trails). One mechanism, four purposes.

---

## Part 1: The Event System

### 1.1 Core Concept

Every boundary crossing in the system emits a typed Event:
- API call starts/ends
- Tool permission checked
- Tool execution starts/ends
- Message appended to conversation
- Compaction triggered
- MCP server connected/disconnected
- Sub-agent spawned/completed
- Error occurred
- Session saved/restored

Events flow through an **EventBus**. Subscribers process them for different purposes:
- **Logger** → structured log output (file, stderr, TUI)
- **Recorder** → session replay file (deterministic reproduction)
- **Metrics** → live counters (token usage, latency, cost)
- **Auditor** → permission audit trail
- **TUI** → real-time display updates

### 1.2 Event Types (sealed interface)

`internal/observe/event.go`

```
type Event interface {
    eventSealed()
    EventKind() string
    Timestamp() time.Time
    TraceID() string        // correlates events across a single user turn
    SpanID() string         // correlates events within a single operation
    ParentSpanID() string   // for nested operations (sub-agent, tool-in-tool)
}

type EventHeader struct {
    Kind         string    `json:"kind"`
    Time         time.Time `json:"time"`
    Trace        string    `json:"trace_id"`
    Span         string    `json:"span_id"`
    ParentSpan   string    `json:"parent_span_id,omitempty"`
    AgentID      string    `json:"agent_id,omitempty"`
}
```

All events embed `EventHeader` and implement the sealed interface.

### 1.3 Event Catalog (every event type, exhaustive)

**Conversation Events:**

| Event | Key Fields | When |
|---|---|---|
| `ConversationStarted` | `ConversationID, Model, Provider, WorkDir` | New conversation created |
| `MessageAppended` | `MessageID, Role, ContentTypes[]string, TokenEstimate int` | Any message added |
| `ConversationForked` | `ParentConvID, ChildConvID, AgentName` | Sub-agent fork |

**API Events:**

| Event | Key Fields | When |
|---|---|---|
| `APIRequestStarted` | `Model, MessageCount, ToolCount, TokenEstimate` | Before provider.Stream() |
| `APIStreamChunk` | `ChunkType string, BytesDelta int` | Each StreamChunk (throttled: 1/sec max) |
| `APIRequestCompleted` | `StopReason, Usage, DurationMs, Model` | Stream fully consumed |
| `APIRequestFailed` | `ErrorType, ErrorMessage, Retryable bool, Attempt int` | API error |
| `APIRetryScheduled` | `Attempt, DelayMs, Reason` | Before retry sleep |

**Tool Events:**

| Event | Key Fields | When |
|---|---|---|
| `ToolCallReceived` | `ToolCallID, ToolName, InputSizeBytes` | Tool use block parsed from response |
| `ToolPermissionChecked` | `ToolCallID, ToolName, Decision string, Rule string, Source string` | Permission evaluated |
| `ToolPermissionPrompted` | `ToolCallID, ToolName, UserDecision string, DurationMs` | User prompted, response recorded |
| `ToolExecutionStarted` | `ToolCallID, ToolName, Concurrent bool` | Execution begins |
| `ToolExecutionCompleted` | `ToolCallID, ToolName, DurationMs, OutputSizeBytes, IsError bool` | Execution ends |
| `ToolExecutionFailed` | `ToolCallID, ToolName, ErrorType, ErrorMessage` | Tool threw error |
| `ToolBatchStarted` | `ConcurrentCount, SerialCount, TotalCount` | Orchestrator begins batch |
| `ToolBatchCompleted` | `TotalDurationMs, ConcurrentDurationMs, SerialDurationMs` | Orchestrator ends batch |

**Compaction Events:**

| Event | Key Fields | When |
|---|---|---|
| `CompactionStarted` | `PreTokenCount, BudgetTokens, MessageCount` | Token budget exceeded |
| `CompactionCompleted` | `PostTokenCount, SummarizedCount, DurationMs` | Compaction done |
| `CompactionFailed` | `ErrorType, ErrorMessage` | Compaction error |

**MCP Events:**

| Event | Key Fields | When |
|---|---|---|
| `MCPServerConnecting` | `ServerName, Transport string` | Connection attempt |
| `MCPServerConnected` | `ServerName, ToolCount, DurationMs` | Connected, tools loaded |
| `MCPServerDisconnected` | `ServerName, Reason string` | Connection lost/closed |
| `MCPServerFailed` | `ServerName, ErrorType, ErrorMessage` | Connection failed |
| `MCPToolCallStarted` | `ServerName, ToolName, InputSizeBytes` | MCP tool invoked |
| `MCPToolCallCompleted` | `ServerName, ToolName, DurationMs, OutputSizeBytes` | MCP tool done |
| `MCPHealthCheck` | `ServerName, Status string, PID int, MemoryMB float64` | Periodic health |

**Session Events:**

| Event | Key Fields | When |
|---|---|---|
| `SessionStarted` | `SessionID, ResumedFrom string` | Session begins |
| `SessionSaved` | `SessionID, MessageCount, FileSizeBytes` | Checkpoint saved |
| `SessionEnded` | `SessionID, DurationMs, TurnCount, TotalCostUSD` | Session ends |

**Agent Events:**

| Event | Key Fields | When |
|---|---|---|
| `SubAgentSpawned` | `AgentID, AgentName, Model, Provider, ParentAgentID` | Sub-agent created |
| `SubAgentCompleted` | `AgentID, DurationMs, TurnCount, Usage` | Sub-agent done |
| `SubAgentFailed` | `AgentID, ErrorType, ErrorMessage` | Sub-agent error |

**Error Events:**

| Event | Key Fields | When |
|---|---|---|
| `ErrorOccurred` | `Severity string, Component string, ErrorType, ErrorMessage, Stack string` | Any error |

**Permission Audit Events (the audit trail that pragma lacks):**

| Event | Key Fields | When |
|---|---|---|
| `PermissionRuleMatched` | `ToolName, Pattern, Source, Decision` | A rule matched |
| `PermissionEscalated` | `ToolName, FromDecision, ToDecision, Reason` | Decision overridden |
| `PermissionDenialEnforced` | `ToolCallID, ToolName, WasExecuted bool` | Denial actually stopped execution |

The `WasExecuted` field on `PermissionDenialEnforced` is the key — it catches the pragma bug where denial is ignored. If `Decision == Deny` but `WasExecuted == true`, the audit log screams.

---

## Part 2: EventBus

`internal/observe/bus.go`

```
type EventBus struct {
    subscribers []Subscriber
    mu          sync.RWMutex
    buffer      chan Event    // buffered channel, non-blocking emit
}

type Subscriber interface {
    HandleEvent(event Event)
}

func NewEventBus(bufferSize int) *EventBus
func (b *EventBus) Emit(event Event)
func (b *EventBus) Subscribe(sub Subscriber) func()
func (b *EventBus) Drain()   // flush buffer, call on shutdown
```

`Emit` is non-blocking — writes to buffered channel. A goroutine reads from the channel and fans out to subscribers. Events are never dropped (buffer sized generously, e.g. 10,000). If buffer fills, Emit blocks (backpressure — better than silent drop).

**Every component that does significant work receives `*EventBus` via constructor injection.** Not a global. Not a singleton.

---

## Part 3: Subscribers

### 3.1 Logger

`internal/observe/logger.go`

```
type Logger struct {
    writer   io.Writer       // file, stderr, or both
    level    Level           // minimum level to output
    topics   map[string]bool // topic filter (nil = all)
    format   Format          // json | text | compact
}

type Level int
const (
    LevelTrace Level = iota   // every StreamChunk, every byte
    LevelDebug                // API calls, tool executions, decisions
    LevelInfo                 // turns, tool results, costs
    LevelWarn                 // retries, slow operations, anomalies
    LevelError                // failures
)

type Format int
const (
    FormatJSON Format = iota  // structured JSONL (for machine parsing)
    FormatText                // human-readable with colors
    FormatCompact             // one-line summaries
)

func NewLogger(writer io.Writer, level Level, format Format, topics map[string]bool) *Logger
func (l *Logger) HandleEvent(event Event)
```

**Topic filtering**: `--debug=tool,api,mcp` shows only those event categories. Solves the all-or-nothing problem.

**Level mapping** (Event → Level):
- `APIStreamChunk` → Trace
- `ToolExecutionStarted`, `APIRequestStarted` → Debug
- `ToolExecutionCompleted`, `APIRequestCompleted`, `MessageAppended` → Info
- `APIRetryScheduled`, `CompactionStarted` → Warn
- `*Failed`, `ErrorOccurred` → Error

### 3.2 Recorder (for replay)

`internal/observe/recorder.go`

```
type Recorder struct {
    file     *os.File
    encoder  *json.Encoder
    mu       sync.Mutex
}

func NewRecorder(path string) (*Recorder, error)
func (r *Recorder) HandleEvent(event Event)
func (r *Recorder) Close() error
```

Writes every event as JSONL to a replay file. The file contains the complete sequence of events for deterministic replay.

**Replay file format** (`.gogent-replay.jsonl`):
```jsonl
{"kind":"ConversationStarted","time":"...","trace_id":"...","conversation_id":"...","model":"claude-sonnet-4-20250514"}
{"kind":"MessageAppended","time":"...","role":"user","content_types":["text"],"token_estimate":42}
{"kind":"APIRequestStarted","time":"...","model":"claude-sonnet-4-20250514","message_count":2,"tool_count":15}
{"kind":"APIStreamChunk","time":"...","chunk_type":"text_delta","bytes_delta":23}
{"kind":"APIRequestCompleted","time":"...","stop_reason":"tool_use","usage":{"input_tokens":1500,"output_tokens":200}}
{"kind":"ToolCallReceived","time":"...","tool_call_id":"uuid-1","tool_name":"Bash","input_size_bytes":45}
{"kind":"ToolPermissionChecked","time":"...","tool_call_id":"uuid-1","tool_name":"Bash","decision":"allow","rule":"allow:Bash(git *)","source":"settings"}
{"kind":"ToolExecutionStarted","time":"...","tool_call_id":"uuid-1","tool_name":"Bash"}
{"kind":"ToolExecutionCompleted","time":"...","tool_call_id":"uuid-1","tool_name":"Bash","duration_ms":234,"output_size_bytes":1200,"is_error":false}
```

### 3.3 Metrics Collector

`internal/observe/metrics.go`

```
type Metrics struct {
    mu               sync.RWMutex
    tokenUsage       model.TokenUsage   // accumulated
    totalCostUSD     float64
    turnCount        int
    toolCalls        map[string]int     // tool name → count
    toolDurations    map[string]int64   // tool name → total ms
    toolErrors       map[string]int     // tool name → error count
    apiCalls         int
    apiErrors        int
    apiTotalLatencyMs int64
    compactions      int
}

func NewMetrics() *Metrics
func (m *Metrics) HandleEvent(event Event)
func (m *Metrics) Snapshot() MetricsSnapshot
```

`Snapshot()` returns a value copy — displayed in TUI status bar. **Live token/cost visibility** that pragma lacks.

```
type MetricsSnapshot struct {
    TokenUsage       model.TokenUsage
    TotalCostUSD     float64
    TurnCount        int
    ToolCallCount    int
    ToolErrorCount   int
    APICallCount     int
    APIErrorCount    int
    AvgAPILatencyMs  int64
    Compactions      int
    SessionDurationMs int64
}
```

### 3.4 Auditor (permission trail)

`internal/observe/auditor.go`

```
type Auditor struct {
    mu      sync.Mutex
    entries []AuditEntry
}

type AuditEntry struct {
    Timestamp   time.Time `json:"timestamp"`
    ToolCallID  string    `json:"tool_call_id"`
    ToolName    string    `json:"tool_name"`
    Decision    string    `json:"decision"`      // allow|deny|ask
    UserResponse string   `json:"user_response"` // allow|deny (if asked)
    RuleMatched string    `json:"rule_matched"`
    RuleSource  string    `json:"rule_source"`
    WasExecuted bool      `json:"was_executed"`   // THE KEY FIELD
}

func NewAuditor() *Auditor
func (a *Auditor) HandleEvent(event Event)
func (a *Auditor) Trail() []AuditEntry    // full audit trail
func (a *Auditor) Violations() []AuditEntry  // entries where Decision==deny && WasExecuted==true
```

`Violations()` catches the exact bug pragma has — permission denial ignored.

---

## Part 4: Trace Correlation

Every user turn generates a **TraceID** (UUID). Every operation within that turn generates a **SpanID**. Nested operations (sub-agent spawns, tool-in-tool) set **ParentSpanID**.

```
TraceID: abc-123 (one per user turn)
├── SpanID: def-456 (API request)
│   └── ParentSpanID: (none, root span)
├── SpanID: ghi-789 (tool execution: Bash)
│   └── ParentSpanID: (none, root span)
├── SpanID: jkl-012 (tool execution: Agent)
│   └── ParentSpanID: (none, root span)
│   ├── SpanID: mno-345 (sub-agent API request)
│   │   └── ParentSpanID: jkl-012
│   └── SpanID: pqr-678 (sub-agent tool: FileRead)
│       └── ParentSpanID: jkl-012
```

This enables:
- "Show me everything that happened in turn 3" (filter by TraceID)
- "Show me why this tool call was slow" (follow SpanID chain)
- "Show me what the sub-agent did" (filter by AgentID)

---

## Part 5: Replay System

### 5.1 Recording

Recording is automatic when a Recorder subscriber is attached to the EventBus. CLI flag `--record` or env `GOGENT_RECORD=1` enables it.

But events alone aren't enough for replay — we also need the **tool outputs** (which are non-deterministic). The Recorder captures:

1. All events (including ToolExecutionCompleted with output hash)
2. Tool output snapshots (actual output content, stored separately to keep event log lean)

```
type RecordedToolOutput struct {
    ToolCallID string `json:"tool_call_id"`
    Output     string `json:"output"`
    IsError    bool   `json:"is_error"`
}
```

Replay file structure:
```
session-abc/
  events.jsonl              # all events
  tool-outputs/
    uuid-1.json             # Bash output
    uuid-2.json             # FileRead output
    ...
  api-responses/
    turn-1.json             # raw API response (for provider-level replay)
    turn-2.json
    ...
```

### 5.2 Replay Engine

`internal/observe/replay.go`

```
type ReplayEngine struct {
    events      []Event
    toolOutputs map[string]RecordedToolOutput
    apiResponses map[int]model.Response  // turn number → response
}

func LoadReplay(dir string) (*ReplayEngine, error)
func (r *ReplayEngine) Events() []Event
func (r *ReplayEngine) ToolOutput(toolCallID string) (string, bool)
func (r *ReplayEngine) APIResponse(turn int) (model.Response, bool)
```

### 5.3 Replay Modes

**Event replay** (fastest, for debugging):
- Walk through events, display in TUI or log format
- No actual execution — just view what happened
- `gogent replay --events session-abc/`

**Deterministic replay** (for regression testing):
- Mock provider returns recorded API responses
- Mock tools return recorded outputs
- Verify the system produces the same events
- `gogent replay --deterministic session-abc/`

**Partial replay** (for reproducing bugs):
- Replay up to turn N, then switch to live execution
- Use recorded conversation state as starting point
- `gogent replay --until-turn=5 --then-live session-abc/`

---

## Part 6: Testing Strategy

### 6.1 Test Harness

`internal/testing/` (not `internal/test/` which is a Go keyword)

```
type Harness struct {
    provider     *MockProvider
    bus          *observe.EventBus
    recorder     *observe.Recorder
    registry     *tool.Registry
    store        *app.StateStore[app.AppState]
    events       []observe.Event  // captured for assertions
}

func NewHarness() *Harness
func (h *Harness) WithTool(name string, handler func(json.RawMessage) (string, error)) *Harness
func (h *Harness) WithProviderResponse(responses ...model.Response) *Harness
func (h *Harness) Run(ctx context.Context, userMessage string) ([]observe.Event, error)
func (h *Harness) AssertEvent(kind string, matcher func(Event) bool) error
func (h *Harness) AssertNoEvent(kind string) error
func (h *Harness) AssertEventSequence(kinds ...string) error
```

### 6.2 Mock Provider

```
type MockProvider struct {
    responses []model.Response  // returns in order
    calls     []provider.RequestParams  // records what was sent
    idx       int
}

func NewMockProvider(responses ...model.Response) *MockProvider
func (m *MockProvider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error)
func (m *MockProvider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error)
func (m *MockProvider) Calls() []provider.RequestParams  // what was sent
```

`Stream` converts `model.Response` into `StreamChunk` sequence (TextDelta chunks + Done).

### 6.3 Scenario Testing (the key for complex cases)

Scenario files describe multi-turn agentic interactions:

`testdata/scenarios/multi_tool_turn.yaml`:
```yaml
name: "Multi-tool turn with permission denial"
model: "mock"
turns:
  - user: "List files and read README.md"
    provider_response:
      stop_reason: tool_use
      content:
        - type: tool_call
          name: Bash
          input: {"command": "ls"}
        - type: tool_call
          name: Read
          input: {"file_path": "README.md"}
    tool_results:
      - name: Bash
        output: "file1.go\nfile2.go\nREADME.md"
      - name: Read
        output: "# Project\nSome content"
    expect_events:
      - kind: ToolBatchStarted
        concurrent_count: 2
      - kind: ToolPermissionChecked
        tool_name: Bash
        decision: allow
      - kind: ToolPermissionChecked
        tool_name: Read
        decision: allow
      - kind: ToolBatchCompleted

  - provider_response:
      stop_reason: end_turn
      content:
        - type: text
          text: "Here are the files..."
    expect_events:
      - kind: TurnCompleteEvent
        stop_reason: end_turn
```

```
type Scenario struct {
    Name   string       `yaml:"name"`
    Model  string       `yaml:"model"`
    Turns  []ScenarioTurn `yaml:"turns"`
}

type ScenarioTurn struct {
    User             string                 `yaml:"user,omitempty"`
    ProviderResponse model.Response         `yaml:"provider_response"`
    ToolResults      map[string]string      `yaml:"tool_results,omitempty"`
    ExpectEvents     []EventMatcher         `yaml:"expect_events"`
    ExpectNoEvents   []string               `yaml:"expect_no_events,omitempty"`
}

type EventMatcher struct {
    Kind   string            `yaml:"kind"`
    Fields map[string]string `yaml:"fields,omitempty"` // field name → expected value
}

func RunScenario(t *testing.T, path string)
func RunAllScenarios(t *testing.T, dir string)
```

### 6.4 What Scenario Tests Catch

| Scenario | Tests | Pragma Bug It Would Catch |
|---|---|---|
| `permission_denial_enforced.yaml` | Deny tool → verify `WasExecuted=false` | Permission bypass (user says No, tool runs) |
| `mcp_timeout.yaml` | MCP tool hangs → verify timeout event after N seconds | 16-hour MCP hang with no output |
| `concurrent_tool_error.yaml` | One tool in batch fails → others get cancelled | Silent tool failures in concurrent batches |
| `compaction_trigger.yaml` | Token budget exceeded → compaction fires → tokens reduced | Invisible token explosion |
| `api_retry_backoff.yaml` | Rate limit → retry with backoff → success | Retry storms |
| `subagent_permission_scope.yaml` | Sub-agent tries parent-only tool → denied | Sub-agent permission escalation |
| `session_save_restore.yaml` | Save mid-conversation → restore → continue | Session corruption |
| `token_cost_tracking.yaml` | Multi-turn → verify cost accumulates correctly | Invisible cost explosion |

### 6.5 Replay-Based Regression Testing

When a bug is found in production:
1. User shares replay file (`gogent share-replay` → uploads events + outputs)
2. Replay file becomes a test case in `testdata/replays/`
3. CI runs all replay files deterministically
4. If system behavior changes, test fails
5. Bug can never regress

```
func TestReplays(t *testing.T) {
    replays, _ := filepath.Glob("testdata/replays/*/events.jsonl")
    for _, replay := range replays {
        t.Run(filepath.Base(filepath.Dir(replay)), func(t *testing.T) {
            engine := observe.LoadReplay(filepath.Dir(replay))
            RunDeterministicReplay(t, engine)
        })
    }
}
```

---

## Part 7: Health Monitoring

### 7.1 MCP Watchdog

`internal/observe/watchdog.go`

```
type MCPWatchdog struct {
    bus         *EventBus
    timeouts    map[string]time.Time  // server → last activity
    maxSilence  time.Duration         // default 30s
}

func NewMCPWatchdog(bus *EventBus, maxSilence time.Duration) *MCPWatchdog
func (w *MCPWatchdog) Start(ctx context.Context)
```

- Subscribes to MCP events on the bus
- If no activity from a server for `maxSilence`, emits `MCPHealthCheck` event with `Status: "unresponsive"`
- If process is dead, emits `MCPServerDisconnected` with `Reason: "process_exited"`
- Prevents the 16-hour zombie process problem

### 7.2 Token Budget Monitor

```
type TokenMonitor struct {
    bus        *EventBus
    budget     int
    current    int
    thresholds []float64  // e.g. [0.5, 0.8, 0.95]
    notified   map[float64]bool
}

func NewTokenMonitor(bus *EventBus, budget int, thresholds []float64) *TokenMonitor
```

Emits warnings when token usage crosses thresholds. TUI shows live token count.

---

## Part 8: CLI Integration

```
gogent                         # normal mode (Info level, compact format)
gogent --verbose               # Debug level, text format
gogent --debug                 # Trace level, JSON format to file
gogent --debug=tool,api        # Trace level, only tool+api topics
gogent --record                # enable replay recording
gogent replay <dir>            # replay a recorded session
gogent replay --deterministic  # replay for regression testing
gogent audit <session>         # show permission audit trail
gogent metrics <session>       # show metrics summary from replay file
```

---

## Part 9: Wiring (how it connects to existing architecture)

```
main.go:
  bus := observe.NewEventBus(10000)
  bus.Subscribe(observe.NewLogger(logWriter, level, format, topics))
  bus.Subscribe(observe.NewMetrics())
  bus.Subscribe(observe.NewAuditor())
  if recordMode { bus.Subscribe(observe.NewRecorder(replayDir)) }

  provider := anthropic.New(apiKey, bus)     // provider emits API events
  registry := tool.NewRegistry(bus)          // registry emits tool events
  orchestrator := tool.NewOrchestrator(registry, perms, bus)  // orchestrator emits batch events
  engine := query.NewEngine(provider, registry, orchestrator, store, config, bus)  // engine emits turn events

  // Every component receives *EventBus via constructor
  // Every component emits events at its boundaries
  // No ad-hoc logging — everything goes through events
```

---

## Part 10: Persistence of This Plan

On approval:
1. Append to `/SPEC.md` as new Part (Observability)
2. Update `.pragma/AGENT.md` with observability conventions
3. Create agile tasks for observe/ package implementation
4. Update memory with observability design reference

---

## Verification

- `go test ./internal/observe/` — EventBus delivers to all subscribers
- `go test ./internal/observe/ -run TestLogger` — level/topic filtering works
- `go test ./internal/observe/ -run TestRecorder` — events round-trip through JSONL
- `go test ./internal/observe/ -run TestAuditor` — permission violations detected
- `go test ./internal/observe/ -run TestMetrics` — token/cost accumulation correct
- Scenario test: `go test ./internal/testing/ -run TestScenarios` — all YAML scenarios pass
- Replay test: `go test ./internal/testing/ -run TestReplays` — all recorded sessions replay deterministically
- Watchdog test: mock MCP server goes silent → watchdog emits unresponsive event within maxSilence
- Integration: run `gogent --record -p "list files"` → replay file exists → `gogent replay --deterministic` passes
