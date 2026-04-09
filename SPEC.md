# gogent — Technical Specification

## Context

Porting Pragma TypeScript CLI (57 tools, ~1,900 files) to Go. This document specifies the **exact type system, method signatures, design patterns, and reflection strategy** — everything down to parameter names and return types. No code, but a complete contract.

**Core philosophy**: Struct tags are the single source of truth. Reflection at startup generates all boilerplate (JSON schemas, permission closures, tool descriptors). Zero reflection at runtime — all hot paths use pre-built closures and cached data.

---

## Part 0: Persistence Strategy (How This Plan Survives Sessions)

This plan file (`/Users/artpar/.pragma/plans/...`) is ephemeral. On exit, persist to **4 durable locations**:

### 0.1 In-Repo Spec: `SPEC.md` (root of repo)

Copy this entire technical specification into `/Users/artpar/workspace/code/gogent/SPEC.md` and commit it. This is the **authoritative contract** — any future session reads this first. It contains:
- All struct definitions with exact fields and tags
- All method signatures with exact params and return types
- Design patterns mapped to components
- Reflection strategy
- Library choices

Update `SPEC.md` whenever a type or signature changes during implementation. It stays in sync with the code.

### 0.2 AGENT.md (already exists — add reference)

Add to `.pragma/AGENT.md`:
```
## Authoritative Spec
- Full type system, method signatures, reflection strategy: see `/SPEC.md`
- Any future session MUST read SPEC.md before writing code
- If code diverges from SPEC.md, update SPEC.md (spec follows code, not the other way around)
```

### 0.3 Memory Files (cross-session recall)

Update memory to point to SPEC.md:
- `architecture_decisions.md` — add: "Full spec in /SPEC.md, ADRs in .pragma/AGENT.md"
- `project_gogent_port.md` — add: "Read SPEC.md before coding. All types, methods, patterns defined there."

### 0.4 Agile Task Descriptions (already done)

Each task in the GOGENT kanban already references TS source files. Add: "See SPEC.md Part 3.X for Go type definitions" to Phase 1-5 task descriptions.

### Session Start Protocol (Updated)

Every future session:
1. `Read /Users/artpar/workspace/code/gogent/SPEC.md` — the contract
2. `Read /Users/artpar/workspace/code/gogent/.pragma/AGENT.md` — ADRs + conventions
3. `mcp__agile__task_query GOGENT filter:{status:"in_progress"}` — incomplete work
4. Start coding

### When SPEC.md Changes

If during implementation a type or method needs to change:
1. Change the code
2. Update SPEC.md to match
3. Commit both together: `GOGENT-XX: update Foo type + spec`

SPEC.md is a **living document**, not a frozen contract. Code is truth, spec documents truth.

---

## Part 1: Struct Tag Vocabulary

Every struct tag used in the project. This is the tag language.

| Tag | On | Example | Purpose |
|---|---|---|---|
| `json:"name,omitempty"` | All fields | `json:"pattern"` | JSON serialization (encoding/json) |
| `schema:"required"` | Tool input fields | `schema:"required"` | JSON Schema: marks field as required |
| `desc:"..."` | Tool input fields | `desc:"The glob pattern to match"` | JSON Schema: description property |
| `enum:"a,b,c"` | Tool input fields | `enum:"content,files_with_matches,count"` | JSON Schema: enum constraint |
| `default:"value"` | Tool input fields | `default:"."` | JSON Schema: default value |
| `merge:"strategy"` | Settings fields | `merge:"append"` | Config merge: replace\|append\|deep |
| `min:"N"` | Tool input fields | `min:"0"` | JSON Schema: minimum |
| `max:"N"` | Tool input fields | `max:"600000"` | JSON Schema: maximum |

---

## Part 2: Libraries (Final)

| Library | Import Path | Purpose | Reflection Role |
|---|---|---|---|
| cobra | `github.com/spf13/cobra` | CLI framework | None |
| anthropic-sdk-go | `github.com/anthropics/anthropic-sdk-go` | Claude API | None |
| bubbletea | `github.com/charmbracelet/bubbletea` | TUI | None |
| lipgloss | `github.com/charmbracelet/lipgloss` | Styling | None |
| bubbles | `github.com/charmbracelet/bubbles` | TUI components (textarea, viewport, spinner) | None |
| mcp-go | `github.com/mark3labs/mcp-go` | MCP protocol | None |
| doublestar | `github.com/bmatcuk/doublestar/v4` | Glob `**` patterns | None |
| websocket | `nhooyr.io/websocket` | WebSocket | None |
| errgroup | `golang.org/x/sync/errgroup` | Concurrent tool batches | None |
| uuid | `github.com/google/uuid` | Task IDs | None |
| slog | `log/slog` (stdlib) | Structured logging | None |
| reflect | `reflect` (stdlib) | Schema gen, tool registration, config merge | **Core** |

**No other libraries.** Everything else is stdlib.

---

## Part 3: Package Architecture (with exact types)

### 3.1 `internal/tool/` — Tool System (Reflection Core)

#### Struct Tag → JSON Schema Generator

```
func GenerateSchema[T any]() json.RawMessage
```
- Reflects on struct type `T` at registration time
- Reads `json`, `schema`, `desc`, `enum`, `default`, `min`, `max` tags
- Produces JSON Schema as `json.RawMessage` — cached, never regenerated
- Panics if tags are malformed (fail-fast at startup)

Internal helper (unexported):
```
func parseStructTags(t reflect.Type) []SchemaField
```

```
type SchemaField struct {
    Name        string      // from json tag (first component)
    GoName      string      // Go field name (for error messages)
    Type        string      // JSON Schema type: "string"|"number"|"boolean"|"integer"|"array"|"object"
    ItemType    string      // for arrays: item type
    Description string      // from desc tag
    Required    bool        // from schema:"required"
    Enum        []string    // from enum tag, split on ","
    Default     any         // from default tag, parsed to match Type
    Minimum     *float64    // from min tag
    Maximum     *float64    // from max tag
    Omitempty   bool        // from json:",omitempty"
}
```

#### Tool Interface (Minimal — 3 required methods)

```
type Tool[I any, O any] interface {
    Name() string
    Description() string
    Run(ctx context.Context, input I, snap StateSnapshot) (O, error)
}
```

- `I` = input struct (tags generate JSON Schema)
- `O` = output struct (auto-rendered to ContentBlock)
- `StateSnapshot` = `app.AppState` value copy (read-only, crosses boundaries)

**That's it.** Everything else is discovered via optional interfaces.

#### Optional Interfaces (type-asserted in Register, not reflected)

```
type ReadonlyTool interface {
    IsReadonly() bool
}

type ConcurrentTool interface {
    IsConcurrent() bool
}

type DestructiveTool interface {
    IsDestructive() bool
}

type PathTool[I any] interface {
    ExtractPath(input I) string
}

type CustomPermission[I any] interface {
    CheckPermission(ctx context.Context, input I, perm *permission.Checker) (permission.Decision, error)
}

type CustomValidator[I any] interface {
    Validate(input I, snap StateSnapshot) error
}

type CustomRenderer[O any] interface {
    RenderResult(output O) string
}

type SearchClassifier[I any] interface {
    ClassifySearch(input I) (isSearch bool, isRead bool, isList bool)
}

type ActivityDescriber[I any] interface {
    ActivityDescription(input I) string
}

type SummaryProvider[I any] interface {
    Summary(input I) string
}

type PermissionMatcher[I any] interface {
    MatchPermissionPattern(input I, pattern string) bool
}

type BackgroundCapable interface {
    SupportsBackground() bool
}

type Aliased interface {
    Aliases() []string
}

type Searchable interface {
    SearchHint() string
}

type Deferrable interface {
    ShouldDefer() bool
}
```

#### Descriptor (type-erased, stored in Registry)

```
type Descriptor struct {
    Name           string
    Aliases        []string
    Description    string
    SearchHint     string
    InputSchema    json.RawMessage
    MaxResultChars int
    Readonly       bool
    Concurrent     bool
    Destructive    bool
    IsAgent        bool
    ShouldDefer    bool
    Invoke         func(ctx context.Context, raw json.RawMessage, snap StateSnapshot) (string, error)
    CheckPerm      func(ctx context.Context, raw json.RawMessage, perm *permission.Checker) (permission.Decision, error)
    GetActivity    func(raw json.RawMessage) string
    GetSummary     func(raw json.RawMessage) string
    ClassifySearch func(raw json.RawMessage) (bool, bool, bool)
    MatchPerm      func(raw json.RawMessage, pattern string) bool
}
```

Every `func(... json.RawMessage ...)` closure is built by `Register` — it deserializes JSON into the concrete `I` type internally.

#### Registry

```
type Registry struct {
    tools   map[string]*Descriptor
    aliases map[string]string        // alias → canonical name
    mu      sync.RWMutex
}

func NewRegistry() *Registry
func Register[I any, O any](r *Registry, t Tool[I, O])
func (r *Registry) Get(name string) (*Descriptor, bool)
func (r *Registry) All() []*Descriptor
func (r *Registry) APIToolParams() []api.ToolParam
func (r *Registry) DeferredToolParams() []api.ToolParam
func (r *Registry) EagerToolParams() []api.ToolParam
```

**What `Register` does (reflection at startup)**:
1. `GenerateSchema[I]()` → produces `InputSchema json.RawMessage`
2. Type-assert `t` against every optional interface → set flags
3. Build `Invoke` closure: `json.Unmarshal(raw, &input)` → optional `Validate` → `Run` → optional `RenderResult` or default `json.Marshal(output)`
4. Build `CheckPerm` closure: if `CustomPermission` → use it; elif `PathTool` → extract path + default filesystem check; else → allow
5. Build `GetActivity`, `GetSummary`, `ClassifySearch`, `MatchPerm` closures similarly
6. Store `Descriptor` in map keyed by `Name()` and all `Aliases()`

#### Orchestrator

```
type Orchestrator struct {
    registry *Registry
    perms    *permission.Checker
}

func NewOrchestrator(reg *Registry, perms *permission.Checker) *Orchestrator
func (o *Orchestrator) Execute(
    ctx context.Context,
    calls []api.ToolUseBlock,
    snap StateSnapshot,
) ([]api.ToolResultBlock, error)
```

`Execute` partitions calls:
- Calls where `Descriptor.Concurrent == true` → run in parallel via `errgroup.Group`
- Calls where `Descriptor.Concurrent == false` → run serially after concurrent batch
- For each call: `CheckPerm` → if denied, return error result → else `Invoke` → wrap in `ToolResultBlock`

---

### 3.2 `internal/api/` — Claude API Types

#### Content Blocks (sealed)

```
type ContentBlock interface {
    contentBlockSealed()
    BlockType() string
}

type TextBlock struct {
    Type string `json:"type"`       // "text"
    Text string `json:"text"`
}
func (TextBlock) contentBlockSealed() {}
func (TextBlock) BlockType() string   { return "text" }

type ToolUseBlock struct {
    Type  string          `json:"type"`  // "tool_use"
    ID    string          `json:"id"`
    Name  string          `json:"name"`
    Input json.RawMessage `json:"input"`
}
func (ToolUseBlock) contentBlockSealed() {}
func (ToolUseBlock) BlockType() string   { return "tool_use" }

type ToolResultBlock struct {
    Type      string `json:"type"`        // "tool_result"
    ToolUseID string `json:"tool_use_id"`
    Content   string `json:"content"`
    IsError   bool   `json:"is_error,omitempty"`
}
func (ToolResultBlock) contentBlockSealed() {}
func (ToolResultBlock) BlockType() string   { return "tool_result" }

type ThinkingBlock struct {
    Type     string `json:"type"`     // "thinking"
    Thinking string `json:"thinking"`
}
func (ThinkingBlock) contentBlockSealed() {}
func (ThinkingBlock) BlockType() string   { return "thinking" }

type ImageBlock struct {
    Type   string      `json:"type"`   // "image"
    Source ImageSource `json:"source"`
}
type ImageSource struct {
    Type      string `json:"type"`       // "base64"
    MediaType string `json:"media_type"`
    Data      string `json:"data"`
}
func (ImageBlock) contentBlockSealed() {}
func (ImageBlock) BlockType() string   { return "image" }
```

Custom JSON marshal/unmarshal on `ContentBlock` dispatches on `"type"` field.

#### Messages

```
type Message struct {
    Role    string         `json:"role"`    // "user"|"assistant"
    Content []ContentBlock `json:"content"`
}
```

No sealed interface for messages — just a struct with `Role` discriminator. Simpler, and the API only has 2 roles.

#### Usage

```
type Usage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}
```

#### Tool Param (for API request)

```
type ToolParam struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"input_schema"`
}
```

#### Stream Events (sealed)

```
type StreamEvent interface {
    streamEventSealed()
    EventType() string
}

type MessageStartEvent struct {
    Type    string  `json:"type"`    // "message_start"
    Message Message `json:"message"` // partial, has model/id
}

type ContentBlockStartEvent struct {
    Type         string       `json:"type"`          // "content_block_start"
    Index        int          `json:"index"`
    ContentBlock ContentBlock `json:"content_block"`
}

type ContentBlockDeltaEvent struct {
    Type  string     `json:"type"`  // "content_block_delta"
    Index int        `json:"index"`
    Delta BlockDelta `json:"delta"`
}

type BlockDelta struct {
    Type        string `json:"type"`                   // "text_delta"|"input_json_delta"|"thinking_delta"
    Text        string `json:"text,omitempty"`
    PartialJSON string `json:"partial_json,omitempty"`
    Thinking    string `json:"thinking,omitempty"`
}

type ContentBlockStopEvent struct {
    Type  string `json:"type"`  // "content_block_stop"
    Index int    `json:"index"`
}

type MessageDeltaEvent struct {
    Type  string       `json:"type"`  // "message_delta"
    Delta MessageDelta `json:"delta"`
    Usage Usage        `json:"usage"`
}

type MessageDelta struct {
    StopReason string `json:"stop_reason"`
}

type MessageStopEvent struct {
    Type string `json:"type"` // "message_stop"
}
```

Each implements `streamEventSealed()` and `EventType()`.

#### Client

```
type Client struct {
    apiKey  string
    model   string
    baseURL string
    http    *http.Client
}

type ClientOption func(*Client)

func NewClient(apiKey string, model string, opts ...ClientOption) *Client
func WithBaseURL(url string) ClientOption
func WithHTTPClient(c *http.Client) ClientOption

func (c *Client) Stream(ctx context.Context, params MessageParams) (<-chan StreamEvent, error)

type MessageParams struct {
    Model       string         `json:"model"`
    MaxTokens   int            `json:"max_tokens"`
    Messages    []Message      `json:"messages"`
    System      []SystemBlock  `json:"system,omitempty"`
    Tools       []ToolParam    `json:"tools,omitempty"`
    Temperature *float64       `json:"temperature,omitempty"`
    Metadata    *Metadata      `json:"metadata,omitempty"`
}

type SystemBlock struct {
    Type         string        `json:"type"`          // "text"
    Text         string        `json:"text"`
    CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type CacheControl struct {
    Type string `json:"type"` // "ephemeral"
}

type Metadata struct {
    UserID string `json:"user_id,omitempty"`
}
```

#### Errors

```
var (
    ErrRateLimited   = errors.New("rate limited")
    ErrOverloaded    = errors.New("overloaded")
    ErrPromptTooLong = errors.New("prompt too long")
    ErrAuthFailed    = errors.New("authentication failed")
    ErrRetryable     = errors.New("retryable error")
)

type APIError struct {
    StatusCode int
    Type       string `json:"type"`
    Message    string `json:"message"`
}
func (e *APIError) Error() string
func (e *APIError) Unwrap() error  // returns appropriate sentinel
```

---

### 3.3 `internal/app/` — State

#### StateStore (generic, ADR-002)

```
type StateStore[T any] struct {
    mu    sync.RWMutex
    state T
    subs  []func(T)
}

func NewStateStore[T any](initial T) *StateStore[T]
func (s *StateStore[T]) Snapshot() T
func (s *StateStore[T]) Update(fn func(*T))
func (s *StateStore[T]) Subscribe(fn func(T)) func()
```

#### AppState

```
type AppState struct {
    Settings       config.Settings
    Verbose        bool
    Model          string
    SessionModel   string            // override for this session
    WorkingDir     string
    SessionID      string
    IsInteractive  bool
    IsPlanMode     bool
    IsBriefOnly    bool

    Permission     PermissionState
    Tasks          map[string]TaskEntry
    MCP            MCPState
    Plugins        PluginState
    Agents         AgentState
    Notifications  NotificationState
    Hooks          HookState
    TokenUsage     TokenUsage

    ThinkingEnabled bool
}

type PermissionState struct {
    Mode                   string                     // "default"|"bypass"|"plan"
    AllowRules             map[string][]PermissionRule // source → rules
    DenyRules              map[string][]PermissionRule
    AdditionalWorkingDirs  map[string]string           // path → source
}

type PermissionRule struct {
    Tool    string `json:"tool"`
    Pattern string `json:"pattern"`
}

type TaskEntry struct {
    ID             string
    Type           string     // "bash"|"agent"|"remote"|"teammate"|"workflow"|"monitor"|"dream"
    Status         string     // "pending"|"running"|"completed"|"failed"|"killed"
    Description    string
    ToolUseID      string
    OutputFile     string
    OutputOffset   int64
    StartTime      int64
    EndTime        int64
    IsBackgrounded bool
    Cancel         context.CancelFunc `json:"-"`
}

type MCPState struct {
    Clients        []MCPConnection
    ToolNames      []string
    Resources      map[string][]MCPResource
}

type MCPConnection struct {
    Name   string
    Status string // "connected"|"failed"|"pending"|"needs_auth"|"disabled"
}

type MCPResource struct {
    URI      string `json:"uri"`
    Name     string `json:"name"`
    MimeType string `json:"mimeType,omitempty"`
    Server   string `json:"server"`
}

type PluginState struct {
    Enabled  []string
    Disabled []string
    Errors   []string
}

type AgentState struct {
    Definitions  map[string]AgentDef
    Colors       map[string]string
    NameRegistry map[string]string // name → agent ID
}

type AgentDef struct {
    Name        string
    Description string
    Model       string
    Tools       []string
    SystemPrompt string
}

type NotificationState struct {
    Current *Notification
    Queue   []Notification
}

type Notification struct {
    Title   string
    Message string
    Type    string // "info"|"warning"|"error"
}

type HookState struct {
    SessionStarted bool
    Errors         []string
}

type TokenUsage struct {
    InputTokens              int
    OutputTokens             int
    CacheCreationInputTokens int
    CacheReadInputTokens     int
    TotalCostUSD             float64
}
```

`StateSnapshot` is an alias:
```
type StateSnapshot = AppState
```

---

### 3.4 `internal/permission/` — Permission System

```
type Decision int
const (
    Allow Decision = iota
    Deny
    Ask
)

type Category int
const (
    Read Category = iota
    Write
    Execute
    Network
)

type Checker struct {
    state   *app.StateStore[app.AppState]
    askUser func(toolName string, detail string) (bool, error)
}

func NewChecker(state *app.StateStore[app.AppState], askUser func(string, string) (bool, error)) *Checker
func (c *Checker) Evaluate(ctx context.Context, cat Category, toolName string, path string) (Decision, error)
func (c *Checker) AddAllowRule(source string, rule app.PermissionRule)
func (c *Checker) AddDenyRule(source string, rule app.PermissionRule)
func MatchWildcard(pattern string, value string) bool
```

---

### 3.5 `internal/query/` — Agentic Loop

#### Loop Events (sealed)

```
type LoopEvent interface {
    loopEventSealed()
}

type TextEvent struct{ Text string }
type ThinkingEvent struct{ Text string }
type ToolCallEvent struct{ Name string; ID string; Input json.RawMessage }
type ToolResultEvent struct{ ID string; Content string; IsError bool }
type TurnCompleteEvent struct{ StopReason string; Usage api.Usage }
type ErrorEvent struct{ Err error }

// Each implements loopEventSealed()
```

#### Config

```
type Config struct {
    Model         string
    MaxTokens     int
    MaxTurns      int      // 0 = unlimited
    SystemPrompt  string
    Tools         []api.ToolParam
    Temperature   *float64
}
```

#### Engine

```
type Engine struct {
    client       *api.Client
    registry     *tool.Registry
    orchestrator *tool.Orchestrator
    store        *app.StateStore[app.AppState]
    config       Config
}

func NewEngine(
    client *api.Client,
    registry *tool.Registry,
    orchestrator *tool.Orchestrator,
    store *app.StateStore[app.AppState],
    config Config,
) *Engine

func (e *Engine) Run(ctx context.Context, userMessage string) <-chan LoopEvent
func (e *Engine) RunWithMessages(ctx context.Context, messages []api.Message) <-chan LoopEvent
```

`Run` internals (goroutine writes to channel, ADR-004):
1. Build `[]api.Message` with system prompt + user message
2. Loop:
   a. `client.Stream(ctx, params)` → read `StreamEvent` channel
   b. Forward `TextEvent`/`ThinkingEvent` as delta arrives
   c. Accumulate `ToolUseBlock`s from stream
   d. If `stop_reason == "end_turn"` → send `TurnCompleteEvent` → close channel
   e. If `stop_reason == "tool_use"` → `orchestrator.Execute(calls)` → send `ToolCallEvent`/`ToolResultEvent` → append to messages → loop
   f. Decrement `MaxTurns` if set → close channel if exhausted

---

### 3.6 `internal/config/` — Settings with Merge Tags

```
type Settings struct {
    Model              string            `json:"model"              merge:"replace"`
    MaxTokens          int               `json:"maxTokens"          merge:"replace"`
    CustomInstructions string            `json:"customInstructions" merge:"replace"`
    Temperature        *float64          `json:"temperature"        merge:"replace"`
    ApiKey             string            `json:"apiKey"             merge:"replace"`
    BaseURL            string            `json:"baseURL"            merge:"replace"`
    AllowedTools       []string          `json:"allowedTools"       merge:"append"`
    DeniedTools        []string          `json:"deniedTools"        merge:"append"`
    AllowedPaths       []string          `json:"allowedPaths"       merge:"append"`
    DeniedPaths        []string          `json:"deniedPaths"        merge:"append"`
    Hooks              map[string][]Hook `json:"hooks"              merge:"deep"`
    Permissions        PermSettings      `json:"permissions"        merge:"deep"`
    MCPServers         map[string]MCPServerConfig `json:"mcpServers" merge:"deep"`
}

type Hook struct {
    Command string `json:"command"`
    Pattern string `json:"pattern,omitempty"`
    Timeout int    `json:"timeout,omitempty"`
}

type PermSettings struct {
    DefaultMode string   `json:"defaultMode,omitempty"`
    Allow       []string `json:"allow,omitempty"   merge:"append"`
    Deny        []string `json:"deny,omitempty"    merge:"append"`
}

type MCPServerConfig struct {
    Type    string            `json:"type"`              // "stdio"|"sse"|"http"
    Command string            `json:"command,omitempty"` // stdio
    Args    []string          `json:"args,omitempty"`    // stdio
    URL     string            `json:"url,omitempty"`     // sse/http
    Env     map[string]string `json:"env,omitempty"`
    Headers map[string]string `json:"headers,omitempty"`
}

func LoadSettings(globalPath string, projectPath string, localPath string) (Settings, error)
func MergeSettings(base Settings, overlay Settings) Settings
func mergeByTag(base reflect.Value, overlay reflect.Value)
```

`MergeSettings` reflects on each field:
- `merge:"replace"` → if overlay field is non-zero, use it
- `merge:"append"` → append overlay slice to base slice
- `merge:"deep"` → recursively merge maps / nested structs

---

### 3.7 `internal/tui/` — Bubbletea TUI

```
type Model struct {
    store        *app.StateStore[app.AppState]
    eventCh      <-chan query.LoopEvent
    input        textarea.Model
    viewport     viewport.Model
    spinner      spinner.Model
    blocks       []RenderedBlock
    width        int
    height       int
    mode         Mode
    permPrompt   *PermissionPrompt
}

type Mode int
const (
    ModeInput Mode = iota
    ModeScrolling
    ModePermission
)

type RenderedBlock struct {
    Role    string // "user"|"assistant"|"tool"|"error"
    Content string
    Style   lipgloss.Style
}

type PermissionPrompt struct {
    ToolName string
    Detail   string
    Respond  chan<- bool
}

func NewModel(store *app.StateStore[app.AppState]) Model
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string
```

Tea messages bridging query loop → TUI:
```
type StreamTextMsg struct{ Text string }
type StreamThinkingMsg struct{ Text string }
type ToolCallMsg struct{ Name string; ID string }
type ToolResultMsg struct{ ID string; Content string; IsError bool }
type TurnDoneMsg struct{ StopReason string }
type ErrorMsg struct{ Err error }
type PermissionAskMsg struct{ ToolName string; Detail string; Respond chan<- bool }
```

---

### 3.8 `internal/cli/` — CLI Wiring

```
func RootCmd() *cobra.Command
func RunInteractive(ctx context.Context, store *app.StateStore[app.AppState], engine *query.Engine) error
func RunOnce(ctx context.Context, store *app.StateStore[app.AppState], engine *query.Engine, prompt string) error
```

---

### 3.9 `internal/context/` — System Prompt Construction

```
type Builder struct {
    settings config.Settings
    workDir  string
    claudeMD string
    gitInfo  string
    platform string
}

func NewBuilder(settings config.Settings, workDir string) *Builder
func (b *Builder) Build() []api.SystemBlock
func LoadClaudeMD(dir string) (string, error)
func GetGitInfo(dir string) (string, error)
func GetPlatformInfo() string
```

---

### 3.10 `internal/mcp/` — MCP Client + Tool Adapter (ADR-005)

```
type Transport interface {
    Send(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error)
    Close() error
}

type StdioTransport struct { cmd *exec.Cmd; stdin io.Writer; stdout *bufio.Scanner }
type SSETransport struct { url string; client *http.Client }

func NewStdioTransport(command string, args []string, env []string) (*StdioTransport, error)
func NewSSETransport(url string) (*SSETransport, error)

type Client struct {
    transport Transport
    name      string
}

func NewClient(name string, transport Transport) *Client
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error)
func (c *Client) CallTool(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)
func (c *Client) ListResources(ctx context.Context) ([]Resource, error)
func (c *Client) ReadResource(ctx context.Context, uri string) (json.RawMessage, error)
func (c *Client) Close() error

type ToolDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"inputSchema"`
}

type Resource struct {
    URI      string `json:"uri"`
    Name     string `json:"name"`
    MimeType string `json:"mimeType,omitempty"`
}
```

**MCP Tool Adapter** — wraps MCP tools as `tool.Descriptor` (single path, ADR-005):
```
func RegisterMCPTools(ctx context.Context, reg *tool.Registry, client *Client, prefix string) error
```

This calls `client.ListTools()`, creates a `tool.Descriptor` for each, and registers it. The `Invoke` closure calls `client.CallTool()`. After registration, MCP tools are indistinguishable from built-in tools.

---

### 3.11 `internal/session/` — Persistence

```
type Session struct {
    ID        string        `json:"id"`
    CreatedAt time.Time     `json:"created_at"`
    UpdatedAt time.Time     `json:"updated_at"`
    Messages  []api.Message `json:"messages"`
    Model     string        `json:"model"`
    WorkDir   string        `json:"work_dir"`
    Summary   string        `json:"summary"`
}

type Store struct {
    dir string
}

func NewStore(dir string) *Store
func (s *Store) Save(session Session) error
func (s *Store) Load(id string) (Session, error)
func (s *Store) List() ([]Session, error)
```

---

### 3.12 `internal/task/` — Background Tasks

```
type State int
const (
    Pending State = iota
    Running
    Completed
    Failed
    Killed
)

type Manager struct {
    store *app.StateStore[app.AppState]
}

func NewManager(store *app.StateStore[app.AppState]) *Manager
func (m *Manager) Create(id string, taskType string, desc string, toolUseID string) app.TaskEntry
func (m *Manager) Start(ctx context.Context, id string, run func(ctx context.Context, outputFile string) error) error
func (m *Manager) Kill(id string) error
func (m *Manager) Get(id string) (app.TaskEntry, bool)
func (m *Manager) List(filter *string) []app.TaskEntry
func (m *Manager) ReadOutput(id string, offset int64, block bool, timeout time.Duration) (string, int64, error)
func GenerateID(taskType string) string
```

---

## Part 4: Design Patterns → Components

| Pattern | Component | How It Works |
|---|---|---|
| **Struct tags → JSON Schema** | `tool.GenerateSchema[T]()` | Reflect on `T`'s fields, read json/schema/desc/enum/default/min/max tags, produce JSON Schema bytes |
| **Optional interface discovery** | `tool.Register[I,O]()` | Type-assert tool against 14 optional interfaces, set Descriptor flags, build closures |
| **Type-erased closures** | `tool.Descriptor.Invoke` | Closure captures `I`/`O` types, deserializes JSON → `I`, calls `Run`, serializes `O` → string |
| **Sealed interfaces** | `api.ContentBlock`, `api.StreamEvent`, `query.LoopEvent` | Unexported marker method prevents external implementation |
| **Channel generators** | `query.Engine.Run()`, `api.Client.Stream()` | Goroutine writes to chan, caller reads with `for range`, producer closes (ADR-004) |
| **Functional state updates** | `app.StateStore[T].Update(func(*T))` | Write lock → mutate in place → unlock → notify subscribers (ADR-002) |
| **Values as boundaries** | `StateSnapshot`, `json.RawMessage`, `ToolResultBlock` | Structs cross goroutine/package boundaries as values, never pointers (ADR-003) |
| **Config merge via reflection** | `config.MergeSettings()` | Read `merge` tag per field, apply replace/append/deep strategy |
| **Constructor DI** | `query.NewEngine(...)` | All deps passed as params, no globals, no init() side effects (ADR-006) |
| **Sentinel errors** | `api.ErrRateLimited` etc. | `errors.New` + `fmt.Errorf("%w")` + `errors.Is` (ADR-007) |
| **Adapter pattern** | `mcp.RegisterMCPTools()` | MCP tools → `tool.Descriptor`, single path, no `isMcp` checks (ADR-005) |
| **Elm architecture** | `tui.Model` | bubbletea `Init/Update/View`, events from query loop arrive as `tea.Msg` |

---

## Part 5: Reflection Usage Summary

**3 places, all at startup, all cached:**

1. **`tool.Register[I,O]()`** — reflects on input struct `I` to generate JSON Schema from tags. Type-asserts tool against optional interfaces. Builds type-erased closures.

2. **`tool.GenerateSchema[T]()`** — called by Register. Walks struct fields via `reflect.TypeOf`, reads tags, builds JSON Schema object, marshals to `json.RawMessage`.

3. **`config.MergeSettings()`** — reflects on `Settings` struct fields, reads `merge` tags, applies strategy per field.

**Zero reflection at runtime.** All hot paths (tool invocation, permission checking, streaming, rendering) use pre-built closures and cached data.

---

## Part 6: What You Write Per Tool (Minimal Code)

For a typical tool like GlobTool:

```
// 1. Input struct with tags (SINGLE SOURCE OF TRUTH for schema)
type Input struct {
    Pattern string `json:"pattern" schema:"required" desc:"The glob pattern to match files against"`
    Path    string `json:"path,omitempty"             desc:"The directory to search in"`
}

// 2. Output struct
type Output struct {
    Files     []string `json:"filenames"`
    Count     int      `json:"numFiles"`
    Truncated bool     `json:"truncated"`
    DurationMs int64   `json:"durationMs"`
}

// 3. Tool struct (zero fields for simple tools)
type GlobTool struct{}

// 4. Three required methods
func (*GlobTool) Name() string { ... }
func (*GlobTool) Description() string { ... }
func (*GlobTool) Run(ctx context.Context, input Input, snap StateSnapshot) (Output, error) { ... }

// 5. Optional one-liner interfaces
func (*GlobTool) IsReadonly() bool { ... }     // true
func (*GlobTool) IsConcurrent() bool { ... }   // true
func (*GlobTool) ExtractPath(input Input) string { ... }
func (*GlobTool) Summary(input Input) string { ... }
```

**That's it.** No JSON Schema definition. No permission boilerplate. No result mapping. No registration code. Reflection handles all of that from the struct tags.

Compare to TS where GlobTool is ~200 lines with Zod schema, mapToolResultToToolResultBlockParam, renderToolResultMessage, renderToolUseMessage, checkPermissions, etc.

---

## Part 7: Verification

After implementation:
- `go build ./...` — compiles
- `go test ./internal/tool/ -run TestGenerateSchema` — struct tags → correct JSON Schema
- `go test ./internal/tool/ -run TestRegister` — Register builds correct Descriptor
- `go test ./internal/config/ -run TestMergeSettings` — merge tags work correctly
- `gogent -p "what is 2+2"` — end-to-end non-interactive
- `gogent -p "list files in ."` — tool use works
- `gogent` — interactive REPL launches
