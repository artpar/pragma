package toolset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/pragma/internal/mcp"
)

func TestLoadMergesJSONAndYAMLByScope(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	workDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(workDir, ".pragma"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "home", ".pragma"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(dir, "home", ".pragma", "toolsets.json"), `{
		"toolsets": {
			"idea": {
				"mcpServers": ["global-*"],
				"tools": ["global.*"]
			}
		}
	}`)
	writeFile(t, filepath.Join(workDir, ".pragma", "toolsets.yaml"), `
toolsets:
  idea:
    mcpServers:
      - project-*
    tools:
      - com.intellij.*
  docs:
    includeBuiltinTools: true
    tools:
      - Read
`)
	writeFile(t, filepath.Join(workDir, ".pragma", "toolsets.local.json"), `{
		"toolsets": {
			"docs": {
				"includeBuiltinTools": true,
				"tools": ["Read", "Grep"]
			}
		}
	}`)

	defs, err := Load(workDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := defs["idea"].MCPServers; len(got) != 1 || got[0] != "project-*" {
		t.Fatalf("idea MCPServers = %v, want project override", got)
	}
	if got := defs["docs"].Tools; len(got) != 2 || got[1] != "Grep" {
		t.Fatalf("docs Tools = %v, want local override", got)
	}
}

func TestResolveUnknownToolsetReportsAvailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	workDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(workDir, ".pragma"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workDir, ".pragma", "toolsets.json"), `{
		"toolsets": {
			"idea": {
				"mcpServers": ["jetbrains-*"]
			}
		}
	}`)

	_, err := Resolve(workDir, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != `toolset "missing" not found; available toolsets: idea` {
		t.Fatalf("error = %q", got)
	}
}

func TestCompiledFiltersMCPServersAndTools(t *testing.T) {
	ts := &Compiled{
		Name: "idea",
		Definition: Definition{
			MCPServerSources: []string{"jetbrains"},
			Tools:            []string{"com.intellij.*"},
		},
	}

	servers := map[string]mcp.ServerConfig{
		"jetbrains-custom": {DiscoverySource: "jetbrains"},
		"github":           {DiscoverySource: "config"},
	}
	filtered := ts.FilterMCPServers(servers)
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}
	if _, ok := filtered["jetbrains-custom"]; !ok {
		t.Fatal("expected jetbrains-custom to remain")
	}
	if !ts.AllowMCPTool("jetbrains-custom", "com.intellij.openapi.application.ApplicationInfo.getInstance") {
		t.Fatal("expected IntelliJ tool to be allowed")
	}
	if ts.AllowMCPTool("jetbrains-custom", "other.tool") {
		t.Fatal("expected non-matching tool to be rejected")
	}
}

func TestCompiledBuiltinAndMCPResourceExposure(t *testing.T) {
	ts := &Compiled{
		Name: "idea",
		Definition: Definition{
			MCPServers:              []string{"jetbrains-*"},
			Tools:                   []string{"com.intellij.*"},
			IncludeBuiltinTools:     false,
			IncludeMCPResourceTools: false,
		},
	}
	if ts.AllowBuiltinTool("Bash") {
		t.Fatal("Bash should be hidden")
	}
	if ts.AllowBuiltinTool("ListMcpResourcesTool") {
		t.Fatal("MCP resource tool should be hidden")
	}

	mixed := &Compiled{
		Name: "mixed",
		Definition: Definition{
			Tools:                   []string{"Read", "ListMcpResourcesTool"},
			IncludeBuiltinTools:     true,
			IncludeMCPResourceTools: true,
		},
	}
	if !mixed.AllowBuiltinTool("Read") {
		t.Fatal("Read should be allowed")
	}
	if !mixed.AllowBuiltinTool("ListMcpResourcesTool") {
		t.Fatal("ListMcpResourcesTool should be allowed")
	}
	if mixed.AllowBuiltinTool("Bash") {
		t.Fatal("Bash should be rejected because it does not match tools")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
