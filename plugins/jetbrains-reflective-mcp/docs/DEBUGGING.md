# Debugging

The agent object interface exposes IntelliJ line breakpoints through the IDE debugger model. This is not a filesystem marker or editor-only annotation; it uses IntelliJ `XDebuggerManager`, `XBreakpointManager`, and registered `XLineBreakpointType` implementations.

## Tools

```text
ide.debug.breakpoints
ide.debug.breakpoint.set
ide.debug.breakpoint.remove
```

`ide.debug.breakpoints` is read-only. Setting and removing breakpoints are marked destructive because they mutate the IDE debugger state.

## Setting A Breakpoint

Use a project-relative or absolute file path and a one-based line number:

```json
{
  "name": "ide.debug.breakpoint.set",
  "arguments": {
    "filePath": "src/main/kotlin/Main.kt",
    "line": 42
  }
}
```

The plugin resolves the file through IntelliJ VFS, asks registered line breakpoint types whether they can be placed at the target line, and creates or updates the first compatible breakpoint.

To force a specific line breakpoint type:

```json
{
  "name": "ide.debug.breakpoint.set",
  "arguments": {
    "filePath": "src/main/kotlin/Main.kt",
    "line": 42,
    "typeId": "java-line"
  }
}
```

`enabled` defaults to true. `temporary` defaults to false.

## File And Editor Methods

After opening a file:

```json
{"name":"ide.file.open","arguments":{"filePath":"src/main/kotlin/Main.kt"}}
```

Call methods on the returned object:

```json
{"name":"ide.object.call","arguments":{"object":"file1","method":"setBreakpoint","arguments":{"line":42}}}
```

Editors can set or remove a breakpoint at the caret line when `line` is omitted:

```json
{"name":"ide.object.call","arguments":{"object":"editor1","method":"setBreakpoint","arguments":{}}}
```

## Listing

List all breakpoints:

```json
{"name":"ide.debug.breakpoints","arguments":{}}
```

List breakpoints for a file:

```json
{"name":"ide.object.call","arguments":{"object":"file1","method":"breakpoints","arguments":{}}}
```

Both return `Breakpoint` objects with method catalogs.

## Breakpoint Objects

Breakpoint methods:

- `info()`
- `enable()`
- `disable()`
- `remove()`

The info record includes:

- `typeId`
- `typeTitle`
- `enabled`
- `suspendPolicy`
- `fileUrl`
- `filePath`
- `line`
- `temporary`
- `condition`
- `logMessage`
- `logStack`
- `timestamp`

## Failure Modes

- `file_not_found`: IntelliJ VFS could not resolve the file.
- `breakpoint_type_not_found`: no registered line breakpoint type accepts the file and line.
- `breakpoint_not_found`: remove was requested, but no matching breakpoint exists.

The plugin does not guess a breakpoint type by language name. It asks IntelliJ's registered debugger breakpoint types, which lets Java, Kotlin, JavaScript, and plugin-provided debuggers decide whether a line is valid.

## Coverage

The target API matrix verifies the debugger classes and methods used for this feature:

- `XDebuggerManager`
- `XBreakpointManager`
- `XBreakpointType`
- `XLineBreakpointType`
- `XBreakpoint`
- `XLineBreakpoint`

