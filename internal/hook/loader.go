package hook

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/artpar/gogent/internal/config"
)

// LoadHooks reads hook config from all 3 settings scopes and merges them.
// Returns map[Event][]Entry with entries from all scopes concatenated.
// Global entries come first, local entries last (all execute, no override).
func LoadHooks(workDir string) map[Event][]Entry {
	result := make(map[Event][]Entry)

	globalPath, err := config.GlobalSettingsPath()
	if err == nil {
		loadFromFile(globalPath, result)
	}
	loadFromFile(config.ProjectSettingsPath(workDir), result)
	loadFromFile(config.LocalSettingsPath(workDir), result)

	return result
}

// loadFromFile reads a settings.json file and appends its hooks to the result.
func loadFromFile(path string, result map[Event][]Entry) {
	data, err := readSettingsHooks(path)
	if err != nil || data == nil {
		return
	}
	for event, entries := range data {
		result[Event(event)] = append(result[Event(event)], entries...)
	}
}

// readSettingsHooks reads the "hooks" field from a settings JSON file.
func readSettingsHooks(path string) (map[string][]Entry, error) {
	var raw struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	data, err := readJSONFile(path)
	if err != nil || data == nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Hooks == nil {
		return nil, nil
	}

	result := make(map[string][]Entry, len(raw.Hooks))
	for event, rawEntries := range raw.Hooks {
		var entries []Entry
		if err := json.Unmarshal(rawEntries, &entries); err != nil {
			continue // skip malformed entries
		}
		result[event] = entries
	}
	return result, nil
}

// readJSONFile reads a file and returns its bytes, or nil if not found.
func readJSONFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// MatchCommands returns all commands from entries whose matcher matches the given value.
// Empty matcher = match all. Pipe-separated = OR. /regex/ = regex match.
func MatchCommands(entries []Entry, value string) []Command {
	var result []Command
	for _, entry := range entries {
		if matchesPattern(entry.Matcher, value) {
			result = append(result, entry.Hooks...)
		}
	}
	return result
}

// matchesPattern implements the TS matchesPattern() logic:
// - empty pattern → match all
// - exact match
// - pipe-separated list (OR)
// - /regex/ → regex match
func matchesPattern(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	if pattern == value {
		return true
	}
	// Pipe-separated OR
	if strings.Contains(pattern, "|") {
		for _, p := range strings.Split(pattern, "|") {
			if strings.TrimSpace(p) == value {
				return true
			}
		}
		return false
	}
	// Regex match
	if len(pattern) >= 2 && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
		re, err := regexp.Compile(pattern[1 : len(pattern)-1])
		if err != nil {
			return false
		}
		return re.MatchString(value)
	}
	return false
}
