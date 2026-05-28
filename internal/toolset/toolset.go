package toolset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
)

const (
	resourceListTool = "ListMcpResourcesTool"
)

// File is the top-level reusable toolset configuration file.
type File struct {
	Toolsets map[string]Definition `json:"toolsets" yaml:"toolsets"`
}

// Definition selects which MCP servers and tools are exposed to the model.
type Definition struct {
	MCPServers              []string `json:"mcpServers" yaml:"mcpServers"`
	MCPServerSources        []string `json:"mcpServerSources" yaml:"mcpServerSources"`
	Tools                   []string `json:"tools" yaml:"tools"`
	IncludeBuiltinTools     bool     `json:"includeBuiltinTools" yaml:"includeBuiltinTools"`
	IncludeMCPResourceTools bool     `json:"includeMCPResourceTools" yaml:"includeMCPResourceTools"`
}

// Compiled is a validated, ready-to-apply toolset.
type Compiled struct {
	Name string
	Definition
}

// Load reads reusable toolset definitions from user, project, and local scopes.
func Load(workDir string) (map[string]Definition, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	paths, err := configToolsetPaths(workDir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	out := make(map[string]Definition)
	for _, path := range paths {
		observe.GlobalTrace("range paths")
		file, err := readFile(path)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			if errors.Is(err, os.ErrNotExist) {
				observe.GlobalTrace("if: errors.Is(err, os.ErrNotExist)")
				continue
			}
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		for name, def := range file.Toolsets {
			observe.GlobalTrace("range file.Toolsets")
			out[name] = def
		}
	}
	observe.GlobalTrace("return: out, nil")
	return out, nil
}

// Resolve returns the named compiled toolset from loaded config.
func Resolve(workDir, name string) (*Compiled, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	defs, err := Load(workDir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	def, ok := defs[name]
	if !ok {
		observe.GlobalTrace("if: !ok")
		available := make([]string, 0, len(defs))
		for n := range defs {
			observe.GlobalTrace("range defs")
			available = append(available, n)
		}
		sort.Strings(available)
		if len(available) == 0 {
			observe.GlobalTrace("if: len(available) == 0")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"toolset %q not found; no toolsets are configured\", name)")
			return nil, fmt.Errorf("toolset %q not found; no toolsets are configured", name)
		}
		observe.GlobalTrace("return: nil, fmt.Errorf(\"toolset %q not found; available toolsets: %s\", name, strings...")
		return nil, fmt.Errorf("toolset %q not found; available toolsets: %s", name, strings.Join(available, ", "))
	}
	compiled := &Compiled{Name: name, Definition: def}
	if err := compiled.validate(); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"toolset %q: %w\", name, err)")
		return nil, fmt.Errorf("toolset %q: %w", name, err)
	}
	observe.GlobalTrace("return: compiled, nil")
	return compiled, nil
}

func (c *Compiled) validate() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.MCPServers) == 0 && len(c.MCPServerSources) == 0 && len(c.Tools) == 0 && !c.IncludeBuiltinTools && !c.IncludeMCPResourceTools {
		observe.GlobalTrace("if: len(c.MCPServers) == 0 && len(c.MCPServerSources) == 0 && len(c.Tools) == 0 &...")
		observe.GlobalTrace("return: fmt.Errorf(\"definition has no selectors\")")
		return fmt.Errorf("definition has no selectors")
	}
	for _, pat := range append(append([]string{}, c.MCPServers...), c.Tools...) {
		observe.GlobalTrace("range append(append([]string{}, c.MCPServers...), c.Tools...)")
		if _, err := filepath.Match(pat, "probe"); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"invalid glob %q: %w\", pat, err)")
			return fmt.Errorf("invalid glob %q: %w", pat, err)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// FilterMCPServers removes unselected MCP servers before connection.
func (c *Compiled) FilterMCPServers(servers map[string]mcp.ServerConfig) map[string]mcp.ServerConfig {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: servers")
		return servers
	}
	filtered := make(map[string]mcp.ServerConfig)
	for name, cfg := range servers {
		observe.GlobalTrace("range servers")
		if c.AllowMCPServer(name, cfg) {
			observe.GlobalTrace("if: c.AllowMCPServer(name, cfg)")
			filtered[name] = cfg
		}
	}
	observe.GlobalTrace("return: filtered")
	return filtered
}

// AllowMCPServer reports whether the toolset selects the MCP server.
func (c *Compiled) AllowMCPServer(name string, cfg mcp.ServerConfig) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: true")
		return true
	}
	if len(c.MCPServers) == 0 && len(c.MCPServerSources) == 0 {
		observe.GlobalTrace("if: len(c.MCPServers) == 0 && len(c.MCPServerSources) == 0")
		observe.GlobalTrace("return: true")
		return true
	}
	for _, source := range c.MCPServerSources {
		observe.GlobalTrace("range c.MCPServerSources")
		if source == cfg.DiscoverySource {
			observe.GlobalTrace("if: source == cfg.DiscoverySource")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	for _, pat := range c.MCPServers {
		observe.GlobalTrace("range c.MCPServers")
		if matchGlob(pat, name) {
			observe.GlobalTrace("if: matchGlob(pat, name)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

// AllowMCPTool reports whether an MCP tool from a selected server is exposed.
func (c *Compiled) AllowMCPTool(serverName string, toolName string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: true")
		return true
	}
	if len(c.Tools) == 0 {
		observe.GlobalTrace("if: len(c.Tools) == 0")
		observe.GlobalTrace("return: true")
		return true
	}
	fullName := mcp.BuildToolName(serverName, toolName)
	for _, pat := range c.Tools {
		observe.GlobalTrace("range c.Tools")
		if matchGlob(pat, toolName) || matchGlob(pat, fullName) {
			observe.GlobalTrace("if: matchGlob(pat, toolName) || matchGlob(pat, fullName)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

// AllowBuiltinTool reports whether a built-in Pragma tool should be registered.
func (c *Compiled) AllowBuiltinTool(name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: true")
		return true
	}
	if isMCPResourceTool(name) {
		observe.GlobalTrace("if: isMCPResourceTool(name)")
		observe.GlobalTrace("return: c.IncludeMCPResourceTools && c.matchesTool(name)")
		return c.IncludeMCPResourceTools && c.matchesTool(name)
	}
	observe.GlobalTrace("return: c.IncludeBuiltinTools && c.matchesTool(name)")
	return c.IncludeBuiltinTools && c.matchesTool(name)
}

// SelectsMCP reports whether the toolset depends on MCP tools.
func (c *Compiled) SelectsMCP() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: len(c.MCPServers) > 0 || len(c.MCPServerSources) > 0")
	return len(c.MCPServers) > 0 || len(c.MCPServerSources) > 0
}

func (c *Compiled) matchesTool(name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.Tools) == 0 {
		observe.GlobalTrace("if: len(c.Tools) == 0")
		observe.GlobalTrace("return: true")
		return true
	}
	for _, pat := range c.Tools {
		observe.GlobalTrace("range c.Tools")
		if matchGlob(pat, name) {
			observe.GlobalTrace("if: matchGlob(pat, name)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func isMCPResourceTool(name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: name == resourceListTool")
	return name == resourceListTool
}

func matchGlob(pattern, name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ok, err := filepath.Match(pattern, name)
	observe.GlobalTrace("return: err == nil && ok")
	return err == nil && ok
}

func readFile(path string) (File, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: File{}, err")
		return File{}, err
	}
	var file File
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		observe.GlobalTrace("case: \".yaml\", \".yml\"")
		if err := yaml.Unmarshal(data, &file); err != nil {
			observe.GlobalTrace("return: File{}, fmt.Errorf(\"parse %s: %w\", path, err)")
			return File{}, fmt.Errorf("parse %s: %w", path, err)
		}
	default:
		observe.GlobalTrace("default")
		if err := json.Unmarshal(data, &file); err != nil {
			observe.GlobalTrace("return: File{}, fmt.Errorf(\"parse %s: %w\", path, err)")
			return File{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	observe.GlobalTrace("return: file, nil")
	return file, nil
}

func configToolsetPaths(workDir string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	globalJSON, err := config.GlobalToolsetsPath("json")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve global toolsets path: %w\", err)")
		return nil, fmt.Errorf("resolve global toolsets path: %w", err)
	}
	globalYAML, err := config.GlobalToolsetsPath("yaml")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve global toolsets path: %w\", err)")
		return nil, fmt.Errorf("resolve global toolsets path: %w", err)
	}
	observe.GlobalTrace("return: []string{\n\tglobalJSON,\n\tglobalYAML,\n\tconfig.ProjectToolsetsPath(workDir, \"jso...")
	return []string{
		globalJSON,
		globalYAML,
		config.ProjectToolsetsPath(workDir, "json"),
		config.ProjectToolsetsPath(workDir, "yaml"),
		config.LocalToolsetsPath(workDir, "json"),
		config.LocalToolsetsPath(workDir, "yaml"),
	}, nil
}
