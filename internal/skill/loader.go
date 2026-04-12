package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

// Loader discovers and loads skills from disk.
// Skills are loaded from two scopes: user (~/.gogent/skills/) and
// project (<workDir>/.gogent/skills/). Project skills override user skills
// on name collision.
type Loader struct {
	workDir string
}

// NewLoader creates a Loader for the given working directory.
func NewLoader(workDir string) *Loader {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Loader{workDir: workDir}")
	return &Loader{workDir: workDir}
}

// LoadAll discovers skills from all scopes and returns them.
// Project skills override user skills with the same name.
func (l *Loader) LoadAll() ([]Skill, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	byName := make(map[string]Skill)

	home, err := config.GogentHome()
	if err == nil {
		observe.GlobalTrace("if: err == nil")
		userDir := filepath.Join(home, "skills")
		skills, _ := loadSkillsFromDir(userDir, "user")
		for _, s := range skills {
			observe.GlobalTrace("range skills")
			byName[s.Name] = s
		}
	}

	projDir := filepath.Join(l.workDir, ".gogent", "skills")
	skills, _ := loadSkillsFromDir(projDir, "project")
	for _, s := range skills {
		observe.GlobalTrace("range skills")
		byName[s.Name] = s
	}

	result := make([]Skill, 0, len(byName))
	for _, s := range byName {
		observe.GlobalTrace("range byName")
		result = append(result, s)
	}
	observe.GlobalTrace("return: result, nil")
	return result, nil
}

// Load finds and loads a single skill by name. Case-insensitive exact match.
func (l *Loader) Load(name string) (Skill, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	name = strings.ToLower(strings.TrimSpace(name))

	projDir := filepath.Join(l.workDir, ".gogent", "skills")
	if s, err := loadSkillFromDir(projDir, name, "project"); err == nil {
		observe.GlobalTrace("if: err == nil")
		observe.GlobalTrace("return: s, nil")
		return s, nil
	}

	home, err := config.GogentHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Skill{}, fmt.Errorf(\"unknown skill: %s\", name)")
		return Skill{}, fmt.Errorf("unknown skill: %s", name)
	}
	userDir := filepath.Join(home, "skills")
	if s, err := loadSkillFromDir(userDir, name, "user"); err == nil {
		observe.GlobalTrace("if: err == nil")
		observe.GlobalTrace("return: s, nil")
		return s, nil
	}
	observe.GlobalTrace("return: Skill{}, fmt.Errorf(\"unknown skill: %s\", name)")

	return Skill{}, fmt.Errorf("unknown skill: %s", name)
}

// loadSkillsFromDir scans a skills directory for SKILL.md files.
func loadSkillsFromDir(dir, source string) ([]Skill, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	entries, err := os.ReadDir(dir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		observe.GlobalTrace("range entries")
		if !entry.IsDir() {
			observe.GlobalTrace("if: !entry.IsDir()")
			continue
		}
		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		raw, err := os.ReadFile(skillFile)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		meta, body, err := ParseFrontmatter(string(raw))
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		s := skillFromMeta(entry.Name(), filepath.Join(dir, entry.Name()), source, meta, body)
		skills = append(skills, s)
	}
	observe.GlobalTrace("return: skills, nil")
	return skills, nil
}

// loadSkillFromDir loads a single skill by name from a skills directory.
func loadSkillFromDir(dir, name, source string) (Skill, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	entries, err := os.ReadDir(dir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Skill{}, err")
		return Skill{}, err
	}

	for _, entry := range entries {
		observe.GlobalTrace("range entries")
		if !entry.IsDir() {
			observe.GlobalTrace("if: !entry.IsDir()")
			continue
		}
		if strings.ToLower(entry.Name()) != name {
			observe.GlobalTrace("if: strings.ToLower(entry.Name()) != name")
			continue
		}
		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		raw, err := os.ReadFile(skillFile)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: Skill{}, err")
			return Skill{}, err
		}
		meta, body, err := ParseFrontmatter(string(raw))
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: Skill{}, err")
			return Skill{}, err
		}
		observe.GlobalTrace("return: skillFromMeta(entry.Name(), filepath.Join(dir, entry.Name()), source, meta, b...")
		return skillFromMeta(entry.Name(), filepath.Join(dir, entry.Name()), source, meta, body), nil
	}
	observe.GlobalTrace("return: Skill{}, fmt.Errorf(\"skill %q not found in %s\", name, dir)")

	return Skill{}, fmt.Errorf("skill %q not found in %s", name, dir)
}

// SubstituteArgs performs argument substitution in skill content.
//
// Supported patterns:
//   - $ARGUMENTS → full raw args string
//   - $1, $2, ... → positional (shell-style split)
//   - $name → named arg (positional mapped to argNames[i])
//   - ${GOGENT_SKILL_DIR} → skill's BaseDir
//
// Unmatched $N placeholders are left as-is.
func SubstituteArgs(content, rawArgs string, argNames []string, baseDir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	content = strings.ReplaceAll(content, "${GOGENT_SKILL_DIR}", baseDir)

	content = strings.ReplaceAll(content, "$ARGUMENTS", rawArgs)

	positional := splitArgs(rawArgs)

	for i, arg := range positional {
		observe.GlobalTrace("range positional")
		placeholder := fmt.Sprintf("$%d", i+1)
		content = strings.ReplaceAll(content, placeholder, arg)
	}

	for i, name := range argNames {
		observe.GlobalTrace("range argNames")
		if i >= len(positional) {
			observe.GlobalTrace("if: i >= len(positional)")
			break
		}
		content = strings.ReplaceAll(content, "$"+name, positional[i])
	}
	observe.GlobalTrace("return: content")

	return content
}

// splitArgs splits a raw argument string respecting double quotes.
// "hello world" → ["hello world"], hello world → ["hello", "world"]
var splitArgsRe = regexp.MustCompile(`"([^"]*)"|\S+`)

func splitArgs(raw string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		observe.GlobalTrace("if: raw == \"\"")
		observe.GlobalTrace("return: nil")
		return nil
	}

	matches := splitArgsRe.FindAllStringSubmatch(raw, -1)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		observe.GlobalTrace("range matches")
		if m[1] != "" {
			observe.GlobalTrace("if: m[1] != \"\"")

			result = append(result, m[1])
		} else {
			observe.GlobalTrace("else: m[1] != \"\"")
			result = append(result, m[0])
		}
	}
	observe.GlobalTrace("return: result")
	return result
}
