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
)

const (
	resourceListTool = "ListMcpResourcesTool"
	resourceReadTool = "ReadMcpResourceTool"
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
	paths, err := configToolsetPaths(workDir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Definition)
	for _, path := range paths {
		file, err := readFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for name, def := range file.Toolsets {
			out[name] = def
		}
	}
	return out, nil
}

// Resolve returns the named compiled toolset from loaded config.
func Resolve(workDir, name string) (*Compiled, error) {
	defs, err := Load(workDir)
	if err != nil {
		return nil, err
	}
	def, ok := defs[name]
	if !ok {
		available := make([]string, 0, len(defs))
		for n := range defs {
			available = append(available, n)
		}
		sort.Strings(available)
		if len(available) == 0 {
			return nil, fmt.Errorf("toolset %q not found; no toolsets are configured", name)
		}
		return nil, fmt.Errorf("toolset %q not found; available toolsets: %s", name, strings.Join(available, ", "))
	}
	compiled := &Compiled{Name: name, Definition: def}
	if err := compiled.validate(); err != nil {
		return nil, fmt.Errorf("toolset %q: %w", name, err)
	}
	return compiled, nil
}

func (c *Compiled) validate() error {
	if len(c.MCPServers) == 0 && len(c.MCPServerSources) == 0 && len(c.Tools) == 0 && !c.IncludeBuiltinTools && !c.IncludeMCPResourceTools {
		return fmt.Errorf("definition has no selectors")
	}
	for _, pat := range append(append([]string{}, c.MCPServers...), c.Tools...) {
		if _, err := filepath.Match(pat, "probe"); err != nil {
			return fmt.Errorf("invalid glob %q: %w", pat, err)
		}
	}
	return nil
}

// FilterMCPServers removes unselected MCP servers before connection.
func (c *Compiled) FilterMCPServers(servers map[string]mcp.ServerConfig) map[string]mcp.ServerConfig {
	if c == nil {
		return servers
	}
	filtered := make(map[string]mcp.ServerConfig)
	for name, cfg := range servers {
		if c.AllowMCPServer(name, cfg) {
			filtered[name] = cfg
		}
	}
	return filtered
}

// AllowMCPServer reports whether the toolset selects the MCP server.
func (c *Compiled) AllowMCPServer(name string, cfg mcp.ServerConfig) bool {
	if c == nil {
		return true
	}
	if len(c.MCPServers) == 0 && len(c.MCPServerSources) == 0 {
		return true
	}
	for _, source := range c.MCPServerSources {
		if source == cfg.DiscoverySource {
			return true
		}
	}
	for _, pat := range c.MCPServers {
		if matchGlob(pat, name) {
			return true
		}
	}
	return false
}

// AllowMCPTool reports whether an MCP tool from a selected server is exposed.
func (c *Compiled) AllowMCPTool(serverName string, toolName string) bool {
	if c == nil {
		return true
	}
	if len(c.Tools) == 0 {
		return true
	}
	fullName := mcp.BuildToolName(serverName, toolName)
	for _, pat := range c.Tools {
		if matchGlob(pat, toolName) || matchGlob(pat, fullName) {
			return true
		}
	}
	return false
}

// AllowBuiltinTool reports whether a built-in Pragma tool should be registered.
func (c *Compiled) AllowBuiltinTool(name string) bool {
	if c == nil {
		return true
	}
	if isMCPResourceTool(name) {
		return c.IncludeMCPResourceTools && c.matchesTool(name)
	}
	return c.IncludeBuiltinTools && c.matchesTool(name)
}

// SelectsMCP reports whether the toolset depends on MCP tools.
func (c *Compiled) SelectsMCP() bool {
	if c == nil {
		return false
	}
	return len(c.MCPServers) > 0 || len(c.MCPServerSources) > 0
}

func (c *Compiled) matchesTool(name string) bool {
	if len(c.Tools) == 0 {
		return true
	}
	for _, pat := range c.Tools {
		if matchGlob(pat, name) {
			return true
		}
	}
	return false
}

func isMCPResourceTool(name string) bool {
	return name == resourceListTool || name == resourceReadTool
}

func matchGlob(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

func readFile(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var file File
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &file); err != nil {
			return File{}, fmt.Errorf("parse %s: %w", path, err)
		}
	default:
		if err := json.Unmarshal(data, &file); err != nil {
			return File{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	return file, nil
}

func configToolsetPaths(workDir string) ([]string, error) {
	globalJSON, err := config.GlobalToolsetsPath("json")
	if err != nil {
		return nil, fmt.Errorf("resolve global toolsets path: %w", err)
	}
	globalYAML, err := config.GlobalToolsetsPath("yaml")
	if err != nil {
		return nil, fmt.Errorf("resolve global toolsets path: %w", err)
	}
	return []string{
		globalJSON,
		globalYAML,
		config.ProjectToolsetsPath(workDir, "json"),
		config.ProjectToolsetsPath(workDir, "yaml"),
		config.LocalToolsetsPath(workDir, "json"),
		config.LocalToolsetsPath(workDir, "yaml"),
	}, nil
}
