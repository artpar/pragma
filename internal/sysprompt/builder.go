package sysprompt

import (
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// Builder composes a system prompt from static blocks, AGENT.md files, and environment.
type Builder struct {
	workDir string
	model   string
	bus     *observe.EventBus
}

// New creates a Builder. workDir is the project root. bus may be nil.
func New(workDir, modelID string, bus *observe.EventBus) *Builder {
	return &Builder{
		workDir: workDir,
		model:   modelID,
		bus:     bus,
	}
}

// Build composes the full system prompt.
// Pipeline: static blocks → AGENT.md block → env block.
// Never fails — missing files are skipped, env detection always returns values.
func (b *Builder) Build() model.SystemPrompt {
	// Static blocks (cacheable)
	blocks := staticBlocks()

	// AGENT.md block (not cacheable)
	sources := LoadAgentMD(b.workDir, b.bus)
	if len(sources) > 0 {
		blocks = append(blocks, agentMDBlock(sources))
	}

	// Environment block (not cacheable)
	env := DetectEnv(b.workDir, b.model)
	blocks = append(blocks, envBlock(env))

	// Emit build event
	totalBytes := 0
	for _, blk := range blocks {
		totalBytes += len(blk.Text)
	}
	if b.bus != nil {
		b.bus.Emit(observe.SystemPromptBuilt{
			EventHeader: observe.NewEventHeader("SystemPromptBuilt", "", observe.NewSpanID(), ""),
			BlockCount:  len(blocks),
			TotalBytes:  totalBytes,
		})
	}

	return model.SystemPrompt{Blocks: blocks}
}
