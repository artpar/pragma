# Pragma JetBrains Reflective MCP

This plugin runs inside IntelliJ Platform IDEs and exposes a local MCP server backed by IntelliJ APIs. It is intended to replace Pragma's external LSP dependency with direct IDE state, PSI, VFS, editor, and action-system access.

The bridge deliberately does not invent product-level tool names such as `ide_search`. MCP tool names are stable references to the IntelliJ Platform API surface they call.

## Runtime Contract

On project startup the plugin starts a loopback HTTP MCP server and writes discovery metadata to:

```text
~/.pragma/jetbrains-mcp/latest.json
```

The file includes the project path, IDE version, selected port, HTTP URL, and a ready-to-use MCP config fragment:

```json
{
  "mcpServers": {
    "jetbrains-<project-hash>": {
      "type": "http",
      "url": "http://127.0.0.1:<port>"
    }
  }
}
```

Pragma should consume this file, register the HTTP MCP server, and preserve the tool names as reported by `tools/list`.

## Boundaries

The plugin is organized so most behavior can be tested without starting IntelliJ.

| Boundary | Files | Responsibility |
| --- | --- | --- |
| Values | `Domain.kt`, `ValueMaps.kt` | Serializable domain values returned through MCP. |
| Ports | `Ports.kt` | Small interfaces for IntelliJ capabilities used by tool descriptors. |
| Tool descriptors | `ToolDescriptor.kt`, `ToolDescriptors.kt`, `Schema.kt`, `JsonArgs.kt` | Reflective tool catalog, schemas, argument decoding, and execution. No IntelliJ imports. |
| MCP protocol | `McpProtocol.kt`, `McpHttpHandler.kt` | JSON-RPC/MCP request handling and HTTP adaptation. |
| IntelliJ adapter | `IntelliJIdePorts.kt` | The only layer that calls IntelliJ Platform APIs directly. |
| Startup/service | `McpStartupActivity.kt`, `ReflectiveMcpProjectService.kt`, `plugin.xml` | Project startup, server lifecycle, and discovery file writing. |

This makes the test seam explicit: unit tests use fake ports and real descriptors; IntelliJ fixture tests should cover the adapter layer.

## Current Tools

The initial catalog exposes these IntelliJ-shaped MCP tool names:

```text
com.intellij.openapi.application.ApplicationInfo.getInstance
com.intellij.openapi.actionSystem.ActionManager.getActionIdList
com.intellij.openapi.actionSystem.ActionManager.getAction
com.intellij.openapi.fileEditor.FileEditorManager.getSelectedTextEditor
com.intellij.psi.PsiFile.findElementAt
com.intellij.psi.PsiReference.resolve
com.intellij.psi.search.searches.ReferencesSearch.search
com.intellij.psi.search.FilenameIndex.getVirtualFilesByName
com.intellij.openapi.vfs.LocalFileSystem.refreshAndFindFileByPath
com.intellij.openapi.actionSystem.ActionManager.tryToExecute
```

Read-only and destructive behavior is expressed in tool annotations. `ActionManager.tryToExecute` is marked destructive because actions may mutate IDE or project state.

## Development

Run unit tests:

```bash
gradle --no-daemon test
```

Run the plugin in an IntelliJ sandbox against this repository:

```bash
gradle --no-daemon runIde --args /Users/artpar/workspace/code/pragma
```

The sandbox launch passes `-Didea.trust.all.projects=true` so automated and tmux-based tests do not block on IntelliJ's "Trust this project" dialog.

After the IDE starts, inspect the discovery file:

```bash
cat ~/.pragma/jetbrains-mcp/latest.json
```

Smoke-test the live MCP endpoint:

```bash
URL=$(jq -r '.url' ~/.pragma/jetbrains-mcp/latest.json)

curl -s "$URL" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"curl","version":"test"}}}' | jq .

curl -s "$URL" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | jq -r '.result.tools[].name'

curl -s "$URL" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"com.intellij.openapi.application.ApplicationInfo.getInstance","arguments":{}}}' | jq .
```

Build an installable plugin ZIP:

```bash
gradle --no-daemon buildPlugin
```

## Test Coverage

Existing tests cover the pure protocol and descriptor layers:

- MCP `initialize`, `notifications/initialized`, `ping`, `tools/list`, and `tools/call`.
- Missing tool behavior.
- Stable unique tool names.
- Tool schema presence.
- Descriptor execution through fake ports.
- Destructive metadata for action execution.

The next coverage layer should use IntelliJ fixtures for editor, PSI, references, filename index, VFS refresh, and action-system behavior.
