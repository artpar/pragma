package slash

import (
	"github.com/artpar/pragma/internal/observe"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "\n") {
		observe.GlobalTrace("if: !strings.HasPrefix(value, \"/\") || strings.Contains(value, \"\\n\")")
		observe.GlobalTrace("return: nil")
		return nil
	}
	cmdToken, hasArgs := firstSlashToken(value)
	if !hasArgs {
		observe.GlobalTrace("if: !hasArgs")
		observe.GlobalTrace("return: commandCompletionItems(strings.TrimPrefix(cmdToken, \"/\"), commands)")
		return commandCompletionItems(strings.TrimPrefix(cmdToken, "/"), commands)
	}
	name := strings.TrimPrefix(cmdToken, "/")
	if name != "orchestrate" && name != "fsm" {
		observe.GlobalTrace("if: name != \"orchestrate\" && name != \"fsm\"")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: orchestrateCompletionItems(value, cwd)")
	return orchestrateCompletionItems(value, cwd)
}

func commandCompletionItems(prefix string, commands []Command) []CompletionItem {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	seen := make(map[string]bool)
	var scored []scoredCompletionItem
	for _, cmd := range commands {
		observe.GlobalTrace("range commands")
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
			observe.GlobalTrace("range cmd.Aliases")
			add(alias, "alias for /"+cmd.Name)
		}
	}
	observe.GlobalTrace("return: sortedCompletionItems(scored)")
	return sortedCompletionItems(scored)
}

func orchestrateCompletionItems(value, cwd string) []CompletionItem {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(value)
	if len(fields) == 0 {
		observe.GlobalTrace("if: len(fields) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	endsSpace := endsWithSpace(value)
	args := fields[1:]
	current := ""
	previous := fields[len(fields)-1]
	currentArgIndex := -1
	if !endsSpace {
		observe.GlobalTrace("if: !endsSpace")
		current = fields[len(fields)-1]
		if len(fields) > 1 {
			observe.GlobalTrace("if: len(fields) > 1")
			currentArgIndex = len(fields) - 2
			previous = fields[len(fields)-2]
		}
	}

	state := ParseOrchestrateCompletionState(args, currentArgIndex)
	switch {
	case state.CurrentRole == "definition":
		observe.GlobalTrace("case: state.CurrentRole == \"definition\"")
		return pathCompletionItems(value, current, cwd, pathCompletionYAML)
	case state.CurrentRole == "persona":
		observe.GlobalTrace("case: state.CurrentRole == \"persona\"")
		return pathCompletionItems(value, current, cwd, pathCompletionDir)
	case state.CurrentRole == "flag":
		observe.GlobalTrace("case: state.CurrentRole == \"flag\"")
		if strings.HasPrefix(current, "--persona-dir=") {
			prefix := strings.TrimPrefix(current, "--persona-dir=")
			items := pathCompletionItems(value, prefix, cwd, pathCompletionDir)
			for i := range items {
				observe.GlobalTrace("range items")
				items[i].Replacement = replaceCurrentToken(value, "--persona-dir="+items[i].Label+" ")
			}
			observe.GlobalTrace("return: items")
			return items
		}
		return flagCompletionItems(value, current, state.FlagCandidates())
	case endsSpace && previous != "--prompt":
		observe.GlobalTrace("case: endsSpace && previous != \"--prompt\"")
		switch {
		case previous == "--persona-dir":
			return pathCompletionItems(value, "", cwd, pathCompletionDir)
		case !state.HasDefinition:
			return pathCompletionItems(value, "", cwd, pathCompletionYAML)
		default:
			return flagCompletionItems(value, "", state.FlagCandidates())
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func flagCompletionItems(value, prefix string, flags []string) []CompletionItem {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var scored []scoredCompletionItem
	for _, flag := range flags {
		observe.GlobalTrace("range flags")
		score, ok := fuzzyCompletionScore(flag, prefix)
		if ok {
			observe.GlobalTrace("if: ok")
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
	observe.GlobalTrace("return: sortedCompletionItems(scored)")
	return sortedCompletionItems(scored)
}

type pathCompletionKind int

const (
	pathCompletionYAML pathCompletionKind = iota
	pathCompletionDir
)

func pathCompletionItems(value, prefix, cwd string, kind pathCompletionKind) []CompletionItem {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cwd == "" {
		observe.GlobalTrace("if: cwd == \"\"")
		cwd = "."
	}
	dirPart, basePart := filepath.Split(prefix)
	readDir := dirPart
	if readDir == "" {
		observe.GlobalTrace("if: readDir == \"\"")
		readDir = "."
	}
	if !filepath.IsAbs(readDir) {
		observe.GlobalTrace("if: !filepath.IsAbs(readDir)")
		readDir = filepath.Join(cwd, readDir)
	}
	entries, err := os.ReadDir(readDir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	var scored []scoredCompletionItem
	for _, entry := range entries {
		observe.GlobalTrace("range entries")
		name := entry.Name()
		score, ok := fuzzyCompletionScore(name, basePart)
		if !ok {
			observe.GlobalTrace("if: !ok")
			continue
		}
		isDir := entry.IsDir()
		if kind == pathCompletionDir && !isDir {
			observe.GlobalTrace("if: kind == pathCompletionDir && !isDir")
			continue
		}
		if kind == pathCompletionYAML && !isDir && !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			observe.GlobalTrace("if: kind == pathCompletionYAML && !isDir && !strings.HasSuffix(name, \".yaml\") && ...")
			continue
		}
		candidate := filepath.ToSlash(filepath.Join(dirPart, name))
		if isDir {
			observe.GlobalTrace("if: isDir")
			candidate += "/"
		}
		detail := "directory"
		if !isDir {
			observe.GlobalTrace("if: !isDir")
			detail = "orchestration yaml"
		}
		replacement := candidate
		if kind == pathCompletionDir || !isDir {
			observe.GlobalTrace("if: kind == pathCompletionDir || !isDir")
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
	observe.GlobalTrace("return: sortedCompletionItems(scored)")
	return sortedCompletionItems(scored)
}

func sortedCompletionItems(scored []scoredCompletionItem) []CompletionItem {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].item.Label < scored[j].item.Label
	})
	items := make([]CompletionItem, 0, len(scored))
	for _, entry := range scored {
		observe.GlobalTrace("range scored")
		items = append(items, entry.item)
	}
	observe.GlobalTrace("return: items")
	return items
}

func fuzzyCompletionScore(candidate, query string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	query = strings.ToLower(query)
	candidate = strings.ToLower(candidate)
	if query == "" {
		observe.GlobalTrace("if: query == \"\"")
		observe.GlobalTrace("return: 0, true")
		return 0, true
	}
	if strings.HasPrefix(candidate, query) {
		observe.GlobalTrace("if: strings.HasPrefix(candidate, query)")
		observe.GlobalTrace("return: 100000 - len(candidate), true")
		return 100000 - len(candidate), true
	}

	score := 50000 - len(candidate)
	lastMatch := -1
	searchFrom := 0
	for _, want := range query {
		observe.GlobalTrace("range query")
		match := -1
		for i, have := range candidate[searchFrom:] {
			observe.GlobalTrace("range candidate[searchFrom:]")
			if have == want {
				observe.GlobalTrace("if: have == want")
				match = searchFrom + i
				break
			}
		}
		if match < 0 {
			observe.GlobalTrace("if: match < 0")
			observe.GlobalTrace("return: 0, false")
			return 0, false
		}
		score += 100
		if isCompletionBoundary(candidate, match) {
			observe.GlobalTrace("if: isCompletionBoundary(candidate, match)")
			score += 25
		}
		if lastMatch >= 0 {
			observe.GlobalTrace("if: lastMatch >= 0")
			gap := match - lastMatch - 1
			if gap == 0 {
				observe.GlobalTrace("if: gap == 0")
				score += 50
			} else {
				observe.GlobalTrace("else: gap == 0")
				score -= gap
			}
		}
		lastMatch = match
		searchFrom = match + 1
	}
	observe.GlobalTrace("return: score, true")
	return score, true
}

func isCompletionBoundary(value string, index int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if index == 0 || index >= len(value) {
		observe.GlobalTrace("if: index == 0 || index >= len(value)")
		observe.GlobalTrace("return: true")
		return true
	}
	switch value[index-1] {
	case '-', '_', '.', '/', ' ':
		observe.GlobalTrace("case: '-', '_', '.', '/', ' '")
		return true
	default:
		observe.GlobalTrace("default")
		return false
	}
}

func replaceCurrentToken(value, replacement string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if endsWithSpace(value) {
		observe.GlobalTrace("if: endsWithSpace(value)")
		observe.GlobalTrace("return: value + replacement")
		return value + replacement
	}
	start := strings.LastIndexAny(value, " \t")
	if start < 0 {
		observe.GlobalTrace("if: start < 0")
		observe.GlobalTrace("return: replacement")
		return replacement
	}
	observe.GlobalTrace("return: value[:start+1] + replacement")
	return value[:start+1] + replacement
}

func firstSlashToken(value string) (string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rest := strings.TrimPrefix(value, "/")
	i := strings.IndexAny(rest, " \t")
	if i < 0 {
		observe.GlobalTrace("if: i < 0")
		observe.GlobalTrace("return: value, false")
		return value, false
	}
	observe.GlobalTrace("return: \"/\" + rest[:i], true")
	return "/" + rest[:i], true
}

func endsWithSpace(value string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: value != \"\" && (value[len(value)-1] == ' ' || value[len(value)-1] == '\\t')")
	return value != "" && (value[len(value)-1] == ' ' || value[len(value)-1] == '\t')
}
