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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Builder{\n\tworkDir:\tworkDir,\n\tmodel:\t\tmodelID,\n\tbus:\t\tbus,\n}")
	observe.GlobalTrace("return: &Builder{\n\tworkDir:\tworkDir,\n\tmodel:\t\tmodelID,\n\tbus:\t\tbus,\n}")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	blocks := staticBlocks()

	sources := LoadAgentMD(b.workDir, b.bus)
	if len(sources) > 0 {
		observe.GlobalTrace("if: len(sources) > 0")
		blocks = append(blocks, agentMDBlock(sources))
	}

	env := DetectEnv(b.workDir, b.model)
	blocks = append(blocks, envBlock(env))

	totalBytes := 0
	for _, blk := range blocks {
		observe.GlobalTrace("range blocks")
		totalBytes += len(blk.Text)
	}
	if b.bus != nil {
		observe.GlobalTrace("if: b.bus != nil")
		b.bus.Emit(observe.SystemPromptBuilt{
			EventHeader: observe.NewEventHeader("SystemPromptBuilt", "", observe.NewSpanID(), ""),
			BlockCount:  len(blocks),
			TotalBytes:  totalBytes,
		})
	}
	observe.GlobalTrace("return: model.SystemPrompt{Blocks: blocks}")
	observe.GlobalTrace("return: model.SystemPrompt{Blocks: blocks}")

	return model.SystemPrompt{Blocks: blocks}
}
