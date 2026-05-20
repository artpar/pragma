# Pragma JetBrains Reflective MCP

This plugin runs inside JetBrains IDEs and exposes a local MCP server backed by IntelliJ Platform APIs. It is intended to replace external LSP and filesystem tooling with direct IDE state, PSI, VFS, editor, inspection, refactoring, plugin management, run configuration, and action-system access.

The bridge has two layers:

- `ide.*`: a stable agent object interface for everyday engineering work. It returns live objects such as `file1`, `doc1`, `editor1`, `symbol1`, `search1`, and `plugin1`, each with an immediately visible method catalog.
- Reflective IntelliJ API tools: a lower-level escape hatch for the wider platform surface.

The reflective layer deliberately keeps tool names as stable references to the IntelliJ Platform API surface they call or to the generic reflective bridge used to access that surface.

The plugin depends only on `com.intellij.modules.platform`, so it can load in non-Java IDEs such as WebStorm. Product-specific surfaces such as run configurations, inspections, duplicate detection, and refactoring are exposed only when the host IDE provides the corresponding platform classes.

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

The plugin also records a per-project port hint under `~/.pragma/jetbrains-mcp/ports/` and tries to bind that same port on the next startup. If the remembered port is unavailable, it falls back to a free loopback port and updates discovery metadata.

## Boundaries

The plugin is organized so most behavior can be tested without starting IntelliJ.

| Boundary | Files | Responsibility |
| --- | --- | --- |
| Values | `Domain.kt`, `ValueMaps.kt`, `ReflectiveRuntime.kt` | Serializable values, reflective handles, and JSON boundary values returned through MCP. |
| Ports | `Ports.kt` | Small interfaces for fixed IntelliJ capabilities plus the reflective runtime port. |
| Tool descriptors | `ToolDescriptor.kt`, `ToolDescriptors.kt`, `Schema.kt`, `JsonArgs.kt` | Reflective tool catalog, schemas, argument decoding, and execution. No IntelliJ imports. |
| MCP protocol | `McpProtocol.kt`, `McpHttpHandler.kt` | JSON-RPC/MCP request handling and HTTP adaptation. |
| IntelliJ adapter | `IntelliJIdePorts.kt`, `AgentIdeRuntime.kt`, `ReflectiveRuntime.kt` | The layers that call IntelliJ Platform APIs directly. |
| Startup/service | `McpStartupActivity.kt`, `ReflectiveMcpProjectService.kt`, `plugin.xml` | Project startup, server lifecycle, and discovery file writing. |

This makes the test boundary explicit: unit tests use fake ports and real descriptors, while target API coverage tests verify the actual IntelliJ runtime classes and methods the reflective bridge is expected to expose.

## Agent Object Tools

The recommended tools for agents are:

```text
ide.observe
ide.capabilities
ide.object.list
ide.object.describe
ide.object.call
ide.object.release
ide.file.open
ide.file.resolve
ide.file.create
ide.search.text
ide.plugin.list
ide.plugin.resolve
ide.plugin.enable
ide.plugin.disable
ide.plugin.load
ide.plugin.unload
ide.plugin.install
ide.plugin.uninstall
ide.plugin.self.update
ide.debug.breakpoints
ide.debug.breakpoint.set
ide.debug.breakpoint.remove
ide.run.config.types
ide.run.config.list
ide.run.config.resolve
ide.run.config.create
ide.run.config.update
ide.run.config.delete
ide.run.executions
```

The typical flow is:

1. Call `ide.observe`.
2. Open or resolve something with `ide.file.*`, `ide.search.text`, `ide.plugin.*`, or `ide.run.*`.
3. Use the returned object's `methods` catalog.
4. Call methods through `ide.object.call`.
5. Release stale or no-longer-needed objects with `ide.object.release`.

See `docs/AGENT_OBJECT_INTERFACE.md` for the object model and method catalogs.
See `docs/PLUGIN_MANAGEMENT.md` for plugin list, enable, disable, load, unload, install, and uninstall semantics.
See `docs/DEBUGGING.md` for breakpoint management.

Run, build, test, and debug workflows should use `ide.run.*`, not `ActionManager.tryToExecute`. The run tools read and mutate IntelliJ `RunManager` configurations directly, discover host IDE configuration types and factories at runtime, and return `RunConfiguration` and `Execution` objects with method catalogs. This lets GoLand, WebStorm, IDEA, and other JetBrains IDEs expose the profiles contributed by their installed plugins without hard dependencies in this bridge.

The first install is a harness or IDE responsibility because the MCP server does not exist until the plugin is loaded. Once running, the plugin exposes self-management through `ide.plugin.self.update` plus staged self disable, unload, and uninstall operations that return a restart/reconnect contract for the harness.

## Reflective Tools

The fixed catalog exposes these IntelliJ-shaped MCP tool names:

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

`ActionManager.tryToExecute` is a fallback for UI actions. It returns `scheduledOnly: true` because the IntelliJ action system does not provide a stable completion result through this call.

The generic reflective catalog adds:

```text
com.github.artpar.pragma.jetbrains.reflect.Protocol.describe
com.github.artpar.pragma.jetbrains.reflect.Roots.list
java.lang.Class.forName
java.lang.Class.describe
java.lang.Class.getConstructors
java.lang.reflect.Field.get
java.lang.reflect.Constructor.newInstance
java.lang.reflect.Method.invoke
com.intellij.openapi.application.Application.runReadAction
com.intellij.openapi.command.WriteCommandAction.runWriteCommandAction
com.github.artpar.pragma.jetbrains.reflect.ObjectStore.list
com.github.artpar.pragma.jetbrains.reflect.ObjectStore.get
com.github.artpar.pragma.jetbrains.reflect.ObjectStore.release
```

Read-only and destructive behavior is expressed in MCP tool annotations. Generic method invocation, constructor invocation, write-command invocation, object release, and IDE action execution are marked destructive because they can mutate IDE or project state.

See `docs/REFLECTIVE_PROTOCOL.md` for the JSON value format and handle lifetime rules.
See `docs/TARGET_API_COVERAGE.md` for the tested IntelliJ API matrix.

## Development

Run unit tests:

```bash
gradle --no-daemon -p plugins/jetbrains-reflective-mcp test
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
gradle --no-daemon -p plugins/jetbrains-reflective-mcp buildPlugin
```

## Test Coverage

Tests cover the protocol, descriptor layer, reflective catalog, and target IntelliJ API matrix:

- MCP `initialize`, `notifications/initialized`, `ping`, `tools/list`, and `tools/call`.
- Missing tool behavior.
- Stable unique tool names.
- Tool schema presence.
- Descriptor execution through fake ports.
- Destructive metadata for action execution and generic reflective mutation tools.
- Target IntelliJ classes and method signatures for project, editor, VFS, PSI, search, actions, intentions, diagnostics, inspections, duplication, refactoring, run/build/test APIs, execution APIs, and process control.
- Agent object tools for files, documents, editors, search results, plugin descriptors, plugin management operations, debugger breakpoints, run configuration types, run configurations, and executions.
- Object-layer diagnostics, rename preview/apply, and search-result batch replacement backed by IntelliJ daemon highlights, refactoring, and document APIs.
- Reflective root handles are backed by declared target API coverage.

The coverage rule is strict: if a target API is documented as reachable for agentic development, it must be present in `TargetApiMatrix.kt` and verified by `TargetApiCoverageTest`.
