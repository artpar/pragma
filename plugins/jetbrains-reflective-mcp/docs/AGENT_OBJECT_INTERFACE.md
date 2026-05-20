# Agent Object Interface

The agent object interface is the stable, harness-independent layer for software engineering work through IntelliJ IDEA. It lives entirely in the plugin and is exposed as ordinary MCP tools, so Pragma, Codex, Claude Desktop, or any other MCP client can use the same contract.

The interface is intentionally object-oriented. Top-level tools create or discover live IDE objects. Each object response includes a method catalog, and follow-up work uses `ide.object.call` with the returned object alias.

## Why This Exists

The raw reflective tools expose a large IntelliJ API surface, but a model has to know too much about IntelliJ threading, descriptors, PSI quality, VFS objects, and handle lifetime before it can do basic engineering work.

The object interface keeps the large reflective escape hatch, but gives agents a smaller productive path:

- Observe the current IDE scene.
- Open or resolve a file through the IDE.
- Receive `file1`, `doc1`, `editor1`, `symbol1`, `search1`, `plugin1`, `runConfig1`, `runType1`, or `execution1`.
- Inspect the object's method catalog.
- Call methods on that object.
- Release objects when done.

## Top-Level Tools

These tools are the preferred entry point for agentic development:

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

The `ide.*` tools are not Pragma prompt helpers. They are plugin-side MCP tools returned by `tools/list`.

## Result Envelope

Every object-interface response uses the same shape:

```json
{
  "ok": true,
  "code": "ok",
  "summary": "Opened /repo/src/Main.kt.",
  "data": {},
  "affordances": [],
  "limitations": [],
  "nextBestActions": []
}
```

When an operation cannot run, the plugin returns `ok: false` with a specific `code`. Important codes include:

- `object_not_found`: the alias is not in the live object registry.
- `file_not_found`: the IDE could not resolve the requested file.
- `native_psi_unavailable`: the IDE has no semantic PSI for that file.
- `unsupported_method`: the object exists, but does not expose the requested method.
- `restart_required`: IntelliJ cannot apply the requested plugin operation dynamically.
- `breakpoint_type_not_found`: no IntelliJ line breakpoint type can be placed at the requested file and line.
- `breakpoint_not_found`: removal was requested for a line with no matching breakpoint.
- `run_config_not_found`: the requested run configuration name, id, or object alias was not found.
- `run_config_factory_not_found`: the requested configuration type or factory is not available in the host IDE.
- `run_config_invalid`: IntelliJ rejected the run configuration before execution.
- `executor_not_found`: the host IDE did not expose the requested run/debug executor.
- `invalid_query`: a text search query was empty or otherwise invalid.

## Object Metadata

Objects are server-assigned aliases. Clients must not invent aliases.

```json
{
  "alias": "file1",
  "ref": "file1",
  "type": "File",
  "className": "com.intellij.openapi.vfs.newvfs.impl.VirtualFileImpl",
  "metadata": {
    "path": "/repo/src/Main.kt",
    "name": "Main.kt",
    "directory": false,
    "valid": true,
    "fileType": "Kotlin"
  },
  "methods": [
    {
      "name": "read",
      "signature": "read(): DocumentText",
      "readOnly": true,
      "destructive": false,
      "available": true,
      "unavailableReason": null
    }
  ]
}
```

The `methods` array is the agent's immediate affordance list. A client should show or preserve it in model context after object creation.

## Object Types

### Project

Created by `ide.observe`.

Methods:

- `observe()`
- `capabilities()`
- `openFile(filePath)`
- `createFile(filePath, text?, overwrite?, openEditor?)`
- `search(query, limit?)`
- `runConfigurations(query?, includeTemporary?, limit?)`
- `runConfigurationTypes(query?, limit?)`
- `createRunConfiguration(name, typeId, factoryId?, temporary?, patch?)`

### File

Created by `ide.file.open`, `ide.file.resolve`, `ide.file.create`, directory navigation, or search results.

Methods:

- `read()`
- `openEditor()`
- `search(query, limit?)`
- `replace(startLine, startColumn, endLine, endColumn, text, clamp?)`
- `replaceOffsets(startOffset, endOffset, text)`
- `append(text)`
- `delete()`
- `diagnostics()`
- `symbolAt(line, column)`
- `breakpoints()`
- `setBreakpoint(line, typeId?, enabled?, temporary?)`
- `removeBreakpoint(line, typeId?)`
- `close()`

`symbolAt` is available only when IntelliJ reports semantic PSI. TextMate or plain text files return `native_psi_unavailable`.

### Directory

Created by `ide.file.resolve` or directory navigation.

Methods:

- `children()`
- `find(name)`
- `search(query, limit?)`
- `createFile(name, text?, overwrite?, openEditor?)`
- `createDirectory(name)`

### Document

Created by `file.read()`.

Methods:

- `text()`
- `replace(startLine, startColumn, endLine, endColumn, text)`
- `append(text)`
- `save()`
- `lineInfo(line)`

Document mutations run through IntelliJ write commands so undo and PSI state remain IDE-native.

### Editor

Created by `ide.file.open` or `file.openEditor()`.

Methods:

- `caret()`
- `selection()`
- `select(startLine, startColumn, endLine, endColumn, clamp?)`
- `selectOffsets(startOffset, endOffset)`
- `insert(text)`
- `replaceSelection(text)`
- `symbolAt()`
- `setBreakpoint(line?, typeId?, enabled?, temporary?)`
- `removeBreakpoint(line?, typeId?)`
- `close()`

When `line` is omitted on an editor breakpoint call, the current caret line is used. Line/column selections reject out-of-range coordinates by default; pass `clamp: true` only when clamping is intentional. Offset-based methods are preferred for exact edits.

### Symbol

Created by `file.symbolAt(...)` or `editor.symbolAt()`.

Methods:

- `definition()`
- `references(limit?)`
- `renamePreview(newName)`
- `renameApply(newName)`

Rename uses IntelliJ `RenameProcessor`. Preview returns the target and discovered usages without mutating the project. Apply runs the refactoring through an IntelliJ write command so undo and PSI state remain IDE-native.

### SearchResult

Created by `ide.search.text`, `project.search`, `file.search`, or `directory.search`.

Methods:

- `items()`
- `openItem(index)`
- `replaceAllPreview(text)`
- `replaceAllApply(text)`

`openItem(index)` uses zero-based indexing to match the returned `items` array.

Batch replacement previews and applies replacements over the current search result set. Apply verifies each hit still matches the original query at its recorded offset before editing, then writes through IntelliJ documents and saves the affected files.

### Plugin

Created by `ide.plugin.list`, `ide.plugin.resolve`, or successful install.

Methods:

- `info()`
- `dependencies()`
- `enable()`
- `disable()`
- `load()`
- `unload()`
- `uninstall()`

See `PLUGIN_MANAGEMENT.md` for plugin operation semantics.

### Breakpoint

Created by `ide.debug.breakpoint.set`, `ide.debug.breakpoints`, `file.setBreakpoint(...)`, or `file.breakpoints()`.

Methods:

- `info()`
- `enable()`
- `disable()`
- `remove()`

Breakpoint creation uses IntelliJ `XDebuggerManager` and registered `XLineBreakpointType` implementations. If `typeId` is omitted, the plugin picks the first registered line breakpoint type that reports `canPutAt(file, line, project)`.

### RunConfigurationType

Created by `ide.run.config.types`.

Methods:

- `info()`
- `factories()`
- `template(factoryId?)`

Types and factories come from the host IDE at runtime. A GoLand project exposes Go-related factories if the Go plugin contributes them; a WebStorm project exposes web-related factories if those plugins contribute them. The bridge does not hard-code product-specific run configuration classes.

### RunConfiguration

Created by `ide.run.config.list`, `ide.run.config.resolve`, `ide.run.config.create`, type templates, or clone operations.

Methods:

- `info()`
- `schema()`
- `update(patch)`
- `select()`
- `run()`
- `debug()`
- `clone(name?)`
- `delete()`

`schema()` reports stable settings fields plus primitive/string public setters discovered on the concrete configuration class. `update(patch)` applies supported fields and returns `applied` and `rejected` lists so agents can see exactly what changed.

`run()` and `debug()` call IntelliJ execution APIs directly. They return an `Execution` object and `scheduledOnly: true`; use `ide.run.executions` or `execution.status()` to inspect process state after the IDE starts the profile.

### Execution

Created by `runConfig.run()`, `runConfig.debug()`, or `ide.run.executions`.

Methods:

- `status()`
- `console(limit?, offset?)`
- `stop()`

`ide.run.executions` returns live IntelliJ process handlers when they are running. `console()` returns text captured after the MCP bridge attaches to that process; output emitted before attachment may not be present.

## Example Flow

Open, inspect, edit, and save a file:

```json
{"name":"ide.file.open","arguments":{"filePath":"src/main/kotlin/Main.kt"}}
```

The response returns `file1` and usually `editor1`.

```json
{"name":"ide.object.call","arguments":{"object":"file1","method":"read"}}
```

The response returns `doc1`.

```json
{
  "name": "ide.object.call",
  "arguments": {
    "object": "doc1",
    "method": "replace",
    "arguments": {
      "startLine": 10,
      "startColumn": 1,
      "endLine": 10,
      "endColumn": 20,
      "text": "val next = current"
    }
  }
}
```

```json
{"name":"ide.object.call","arguments":{"object":"doc1","method":"save"}}
```

Set a breakpoint through a File object:

```json
{
  "name": "ide.object.call",
  "arguments": {
    "object": "file1",
    "method": "setBreakpoint",
    "arguments": {
      "line": 42
    }
  }
}
```

Or use the top-level debug tool:

```json
{"name":"ide.debug.breakpoint.set","arguments":{"filePath":"src/main/kotlin/Main.kt","line":42}}
```

## Object Lifetime

Aliases live inside the project MCP service. They are valid until:

- `ide.object.release` is called.
- The IDE project closes.
- The plugin is unloaded or restarted.

Objects can become stale if the underlying IntelliJ object is invalidated. Calls then return `stale_object` where the plugin can detect that condition.

## Strict IDE Boundary

The object interface does not use shell commands, external LSPs, or direct filesystem edits. When a task is not covered, the plugin returns an explicit limitation and points to the reflective escape hatch where appropriate.
