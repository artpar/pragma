# Reflective MCP Protocol

The reflective bridge exposes a small MCP tool set that can reach a large IntelliJ API surface. Agents should discover tools through `tools/list`, call `Protocol.describe`, then work from `Roots.list` and stored object handles.

For normal engineering work, prefer the higher-level `ide.*` object interface documented in `AGENT_OBJECT_INTERFACE.md`. Use this reflective protocol when the object interface does not yet expose a needed IntelliJ API.

## Value Format

Arguments are JSON values with these special object forms:

```json
{ "$ref": "root:project" }
```

References a stored plugin-side object handle.

```json
{ "$class": "com.intellij.openapi.application.ApplicationManager" }
```

References a JVM class object.

```json
{ "$enum": { "className": "fully.qualified.EnumClass", "name": "CONSTANT" } }
```

References an enum constant.

```json
{ "$array": [1, 2, 3], "componentType": "int" }
```

Builds a Java array. `componentType` is optional when the target method parameter is already an array.

Plain JSON strings, booleans, and numbers are coerced to the target JVM parameter type.

## Object Handles

Non-primitive return values can be stored as handles when `storeResult` is true. Handles live in the project MCP service until:

- The agent calls `ObjectStore.release`.
- The IDE project closes and the MCP service stops.
- The plugin is unloaded or restarted.

Root handles use `root:*` names and cannot be released. Current roots include project, application, action manager, file editor manager, file document manager, PSI managers, local filesystem, dumb service, project root manager, run manager, daemon code analyzer, inspection profile manager, and refactoring factory.

## Threading Tools

Use direct `Method.invoke` only for APIs that are safe to call directly.

Use `Application.runReadAction` for PSI, index, VFS metadata, inspections, diagnostics reads, and any API that requires IntelliJ read access.

Use `WriteCommandAction.runWriteCommandAction` for document/PSI mutations and refactoring operations so IntelliJ undo, PSI, and VFS state remain consistent.

Set `dispatchThread=true` when an API requires the IDE event dispatch thread.

## Example Flow

1. Call `Roots.list`.
2. Call `Class.describe` for the target class or root handle.
3. Use `Method.invoke` or `Application.runReadAction` with explicit `parameterTypes` when overloads exist.
4. Store returned objects as handles for follow-up calls.
5. Release non-root handles once the agent no longer needs them.
