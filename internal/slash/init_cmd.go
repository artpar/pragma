package slash

import (
	"context"

	"github.com/artpar/pragma/internal/observe"
)

const initPrompt = `Please analyze this codebase and create an AGENT.md file in the project root, which will be given to future instances of pragma to operate in this repository.

What to add:
1. Commands that will be commonly used, such as how to build, lint, and run tests. Include the necessary commands to develop in this codebase, such as how to run a single test.
2. High-level code architecture and structure so that future instances can be productive more quickly. Focus on the "big picture" architecture that requires reading multiple files to understand.

Usage notes:
- If there's already an AGENT.md, suggest improvements to it.
- When you make the initial AGENT.md, do not repeat yourself and do not include obvious instructions like "Provide helpful error messages to users", "Write unit tests for all new utilities", "Never include sensitive information (API keys, tokens) in code or commits".
- Avoid listing every component or file structure that can be easily discovered.
- Don't include generic development practices.
- If there are Cursor rules (in .cursor/rules/ or .cursorrules), Copilot rules (in .github/copilot-instructions.md), or an existing AGENT.md, make sure to include the important parts.
- If there is a README.md, make sure to include the important parts.
- Do not make up information such as "Common Development Tasks", "Tips for Development", "Support and Documentation" unless this is expressly included in other files that you read.
- Also create the .pragma/ directory if it doesn't exist.
- Be sure to prefix the file with the following text:

` + "```" + `
# AGENT.md

This file provides guidance to pragma when working with code in this repository.
` + "```"

func handleInit(_ context.Context, _ string, _ Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: Result{InjectPrompt: initPrompt}, nil")
	return Result{InjectPrompt: initPrompt}, nil
}
