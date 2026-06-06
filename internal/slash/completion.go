package slash

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CompletionItem struct {
	Label       string `json:"label"`
	Detail      string `json:"detail,omitempty"`
	Replacement string `json:"replacement"`
}

type scoredCompletionItem struct {
	item  CompletionItem
	score int
}

func CompletionItems(value string, commands []Command, cwd string) []CompletionItem {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "\n") {
		return nil
	}
	cmdToken, hasArgs := firstSlashToken(value)
	if !hasArgs {
		return commandCompletionItems(strings.TrimPrefix(cmdToken, "/"), commands)
	}
	name := strings.TrimPrefix(cmdToken, "/")
	if name != "orchestrate" && name != "fsm" {
		return nil
	}
	return orchestrateCompletionItems(value, cwd)
}

func commandCompletionItems(prefix string, commands []Command) []CompletionItem {
	seen := make(map[string]bool)
	var scored []scoredCompletionItem
	for _, cmd := range commands {
		add := func(name, detail string) {
			score, ok := fuzzyCompletionScore(name, prefix)
			if name == "" || seen[name] || !ok {
				return
			}
			seen[name] = true
			scored = append(scored, scoredCompletionItem{
				item: CompletionItem{
					Label:       "/" + name,
					Detail:      detail,
					Replacement: "/" + name + " ",
				},
				score: score,
			})
		}
		add(cmd.Name, cmd.Description)
		for _, alias := range cmd.Aliases {
			add(alias, "alias for /"+cmd.Name)
		}
	}
	return sortedCompletionItems(scored)
}

func orchestrateCompletionItems(value, cwd string) []CompletionItem {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	endsSpace := endsWithSpace(value)
	args := fields[1:]
	current := ""
	previous := fields[len(fields)-1]
	currentArgIndex := -1
	if !endsSpace {
		current = fields[len(fields)-1]
		if len(fields) > 1 {
			currentArgIndex = len(fields) - 2
			previous = fields[len(fields)-2]
		}
	}

	state := ParseOrchestrateCompletionState(args, currentArgIndex)
	switch {
	case state.CurrentRole == "definition":
		return pathCompletionItems(value, current, cwd, pathCompletionYAML)
	case state.CurrentRole == "persona":
		return pathCompletionItems(value, current, cwd, pathCompletionDir)
	case state.CurrentRole == "flag":
		if strings.HasPrefix(current, "--persona-dir=") {
			prefix := strings.TrimPrefix(current, "--persona-dir=")
			items := pathCompletionItems(value, prefix, cwd, pathCompletionDir)
			for i := range items {
				items[i].Replacement = replaceCurrentToken(value, "--persona-dir="+items[i].Label+" ")
			}
			return items
		}
		return flagCompletionItems(value, current, state.FlagCandidates())
	case endsSpace && previous != "--prompt":
		switch {
		case previous == "--persona-dir":
			return pathCompletionItems(value, "", cwd, pathCompletionDir)
		case !state.HasDefinition:
			return pathCompletionItems(value, "", cwd, pathCompletionYAML)
		default:
			return flagCompletionItems(value, "", state.FlagCandidates())
		}
	}
	return nil
}

func flagCompletionItems(value, prefix string, flags []string) []CompletionItem {
	var scored []scoredCompletionItem
	for _, flag := range flags {
		score, ok := fuzzyCompletionScore(flag, prefix)
		if ok {
			scored = append(scored, scoredCompletionItem{
				item: CompletionItem{
					Label:       flag,
					Detail:      OrchestrateFlagDetail(flag),
					Replacement: replaceCurrentToken(value, flag+" "),
				},
				score: score,
			})
		}
	}
	return sortedCompletionItems(scored)
}

type pathCompletionKind int

const (
	pathCompletionYAML pathCompletionKind = iota
	pathCompletionDir
)

func pathCompletionItems(value, prefix, cwd string, kind pathCompletionKind) []CompletionItem {
	if cwd == "" {
		cwd = "."
	}
	dirPart, basePart := filepath.Split(prefix)
	readDir := dirPart
	if readDir == "" {
		readDir = "."
	}
	if !filepath.IsAbs(readDir) {
		readDir = filepath.Join(cwd, readDir)
	}
	entries, err := os.ReadDir(readDir)
	if err != nil {
		return nil
	}
	var scored []scoredCompletionItem
	for _, entry := range entries {
		name := entry.Name()
		score, ok := fuzzyCompletionScore(name, basePart)
		if !ok {
			continue
		}
		isDir := entry.IsDir()
		if kind == pathCompletionDir && !isDir {
			continue
		}
		if kind == pathCompletionYAML && !isDir && !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		candidate := filepath.ToSlash(filepath.Join(dirPart, name))
		if isDir {
			candidate += "/"
		}
		detail := "directory"
		if !isDir {
			detail = "orchestration yaml"
		}
		replacement := candidate
		if kind == pathCompletionDir || !isDir {
			replacement += " "
		}
		scored = append(scored, scoredCompletionItem{
			item: CompletionItem{
				Label:       candidate,
				Detail:      detail,
				Replacement: replaceCurrentToken(value, replacement),
			},
			score: score,
		})
	}
	return sortedCompletionItems(scored)
}

func sortedCompletionItems(scored []scoredCompletionItem) []CompletionItem {
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].item.Label < scored[j].item.Label
	})
	items := make([]CompletionItem, 0, len(scored))
	for _, entry := range scored {
		items = append(items, entry.item)
	}
	return items
}

func fuzzyCompletionScore(candidate, query string) (int, bool) {
	query = strings.ToLower(query)
	candidate = strings.ToLower(candidate)
	if query == "" {
		return 0, true
	}
	if strings.HasPrefix(candidate, query) {
		return 100000 - len(candidate), true
	}

	score := 50000 - len(candidate)
	lastMatch := -1
	searchFrom := 0
	for _, want := range query {
		match := -1
		for i, have := range candidate[searchFrom:] {
			if have == want {
				match = searchFrom + i
				break
			}
		}
		if match < 0 {
			return 0, false
		}
		score += 100
		if isCompletionBoundary(candidate, match) {
			score += 25
		}
		if lastMatch >= 0 {
			gap := match - lastMatch - 1
			if gap == 0 {
				score += 50
			} else {
				score -= gap
			}
		}
		lastMatch = match
		searchFrom = match + 1
	}
	return score, true
}

func isCompletionBoundary(value string, index int) bool {
	if index == 0 || index >= len(value) {
		return true
	}
	switch value[index-1] {
	case '-', '_', '.', '/', ' ':
		return true
	default:
		return false
	}
}

func replaceCurrentToken(value, replacement string) string {
	if endsWithSpace(value) {
		return value + replacement
	}
	start := strings.LastIndexAny(value, " \t")
	if start < 0 {
		return replacement
	}
	return value[:start+1] + replacement
}

func firstSlashToken(value string) (string, bool) {
	rest := strings.TrimPrefix(value, "/")
	i := strings.IndexAny(rest, " \t")
	if i < 0 {
		return value, false
	}
	return "/" + rest[:i], true
}

func endsWithSpace(value string) bool {
	return value != "" && (value[len(value)-1] == ' ' || value[len(value)-1] == '\t')
}
