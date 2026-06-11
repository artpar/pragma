package sysprompt

import (
	_ "embed"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

//go:embed codex_default.md
var codexDefaultPrompt string

// staticBlocks returns the static Codex-compatible system prompt block.
func staticBlocks() []model.SystemBlock {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: []model.SystemBlock{codexDefaultPrompt}")
	observe.GlobalTrace("return: []model.SystemBlock{\n\t{Text: codexDefaultPrompt, Cacheable: false},\n}")
	return []model.SystemBlock{
		{Text: codexDefaultPrompt, Cacheable: false},
	}
}
