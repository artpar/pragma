# Target API Coverage

This project uses target API coverage, not only local code coverage. The goal is to prove that the IntelliJ APIs promised to agents are actually present in the tested IDEA runtime.

## Coverage Rule

Every API surface we claim for agentic development must be represented in `TargetApiMatrix.kt` and checked by `TargetApiCoverageTest`.

The test matrix must include:

- The target category.
- The fully qualified class name.
- Exact method signatures where overloads matter.
- Expected static/non-static shape where that affects invocation.

The test fails when:

- A declared class is not available on the IntelliJ test runtime classpath.
- A declared method is absent.
- A declared static/non-static shape changes.
- A reflective root points at a class that is not covered by the target matrix.
- A required reflective bridge tool is missing from `tools/list`.

## Current Categories

- `application-threading`: application instance, read actions, write actions, dispatch-thread execution.
- `project-model`: project metadata, dumb/indexing state, roots and SDK metadata.
- `editor-documents`: editors, open files, documents, text replacement, saves.
- `vfs`: local filesystem and virtual file read/write primitives.
- `psi-navigation`: PSI file lookup, elements, references, text ranges, document mapping.
- `search-indexes`: file indexes, global scopes, word search, reference search.
- `actions`: action discovery, metadata, and execution.
- `plugin-management`: installed plugin descriptors, enable/disable, dynamic load/unload, local or Marketplace install, and uninstall scheduling.
- `intentions-quickfixes`: intention discovery, availability, invocation, quick-fix conversion.
- `diagnostics-inspections`: daemon analyzer, inspection profiles, global inspection context, cleanup.
- `duplicates`: IntelliJ duplicate-code profile and duplicate fragment APIs.
- `debug-breakpoints`: XDebugger manager, breakpoint manager, registered breakpoint types, line breakpoint mutation.
- `refactoring`: rename, safe delete, extract method/class/interface, change signature, introduce variable/field, move support.
- `run-build-test`: run manager, execution manager, program runner utilities.

## Extending Coverage

When adding a new capability area:

1. Add the IntelliJ classes and methods to `TargetApiMatrix.kt`.
2. Prefer exact parameter types for overloaded methods.
3. Add or update reflective roots only when a root object is generally useful to agents.
4. Run `gradle --no-daemon -p plugins/jetbrains-reflective-mcp test`.
5. Update this document if the new category changes the agent-facing capability contract.

Do not remove a matrix row to make a test pass unless the capability is intentionally no longer part of the agent-facing IDE surface.
