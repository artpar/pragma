package hook

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

// LoadHooks reads hook config from all 3 settings scopes and merges them.
// Returns map[Event][]Entry with entries from all scopes concatenated.
// Global entries come first, local entries last (all execute, no override).
func LoadHooks(workDir string) map[Event][]Entry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result := make(map[Event][]Entry)

	globalPath, err := config.GlobalSettingsPath()
	if err == nil {
		observe.GlobalTrace("if: err == nil")
		loadFromFile(globalPath, result)
	}
	loadFromFile(config.ProjectSettingsPath(workDir), result)
	loadFromFile(config.LocalSettingsPath(workDir), result)
	observe.GlobalTrace("return: result")

	return result
}

// loadFromFile reads a settings.json file and appends its hooks to the result.
func loadFromFile(path string, result map[Event][]Entry) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := readSettingsHooks(path)
	if err != nil || data == nil {
		observe.GlobalTrace("if: err != nil || data == nil")
		return
	}
	for event, entries := range data {
		observe.GlobalTrace("range data")
		result[Event(event)] = append(result[Event(event)], entries...)
	}
}

// readSettingsHooks reads the "hooks" field from a settings JSON file.
func readSettingsHooks(path string) (map[string][]Entry, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var raw struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	data, err := readJSONFile(path)
	if err != nil || data == nil {
		observe.GlobalTrace("if: err != nil || data == nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	if raw.Hooks == nil {
		observe.GlobalTrace("if: raw.Hooks == nil")
		observe.GlobalTrace("return: nil, nil")
		return nil, nil
	}

	result := make(map[string][]Entry, len(raw.Hooks))
	for event, rawEntries := range raw.Hooks {
		observe.GlobalTrace("range raw.Hooks")
		var entries []Entry
		if err := json.Unmarshal(rawEntries, &entries); err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		result[event] = entries
	}
	observe.GlobalTrace("return: result, nil")
	return result, nil
}

// readJSONFile reads a file and returns its bytes, or nil if not found.
func readJSONFile(path string) ([]byte, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if errors.Is(err, os.ErrNotExist) {
			observe.GlobalTrace("if: errors.Is(err, os.ErrNotExist)")
			observe.GlobalTrace("return: nil, nil")
			return nil, nil
		}
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	observe.GlobalTrace("return: data, nil")
	return data, nil
}

// MatchCommands returns all commands from entries whose matcher matches the given value.
// Empty matcher = match all. Pipe-separated = OR. /regex/ = regex match.
func MatchCommands(entries []Entry, value string) []Command {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var result []Command
	for _, entry := range entries {
		observe.GlobalTrace("range entries")
		if matchesPattern(entry.Matcher, value) {
			observe.GlobalTrace("if: matchesPattern(entry.Matcher, value)")
			result = append(result, entry.Hooks...)
		}
	}
	observe.GlobalTrace("return: result")
	return result
}

// matchesPattern implements the TS matchesPattern() logic:
// - empty pattern → match all
// - exact match
// - pipe-separated list (OR)
// - /regex/ → regex match
func matchesPattern(pattern, value string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if pattern == "" {
		observe.GlobalTrace("if: pattern == \"\"")
		observe.GlobalTrace("return: true")
		return true
	}
	if pattern == value {
		observe.GlobalTrace("if: pattern == value")
		observe.GlobalTrace("return: true")
		return true
	}

	if strings.Contains(pattern, "|") {
		observe.GlobalTrace("if: strings.Contains(pattern, \"|\")")
		for _, p := range strings.Split(pattern, "|") {
			observe.GlobalTrace("range strings.Split(pattern, \"|\")")
			if strings.TrimSpace(p) == value {
				observe.GlobalTrace("if: strings.TrimSpace(p) == value")
				observe.GlobalTrace("return: true")
				return true
			}
		}
		observe.GlobalTrace("return: false")
		return false
	}

	if len(pattern) >= 2 && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
		observe.GlobalTrace("if: len(pattern) >= 2 && pattern[0] == '/' && pattern[len(pattern)-1] == '/'")
		re, err := regexp.Compile(pattern[1 : len(pattern)-1])
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: false")
			return false
		}
		observe.GlobalTrace("return: re.MatchString(value)")
		return re.MatchString(value)
	}
	observe.GlobalTrace("return: false")
	return false
}
