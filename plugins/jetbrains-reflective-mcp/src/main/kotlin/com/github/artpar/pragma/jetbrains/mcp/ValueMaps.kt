package com.github.artpar.pragma.jetbrains.mcp

fun Any?.toBoundaryValue(): Any? {
    return when (this) {
        null -> null
        is String, is Number, is Boolean -> this
        is ProjectInfo -> mapOf(
            "projectName" to projectName,
            "projectPath" to projectPath,
            "isIndexing" to isIndexing,
            "ide" to ide.toBoundaryValue(),
            "application" to application.toBoundaryValue(),
        )
        is IdeInfo -> mapOf("fullVersion" to fullVersion, "build" to build, "productName" to productName)
        is ApplicationState -> mapOf(
            "isDispatchThread" to isDispatchThread,
            "isReadAccessAllowed" to isReadAccessAllowed,
            "isWriteAccessAllowed" to isWriteAccessAllowed,
        )
        is ActionInfo -> mapOf(
            "actionId" to actionId,
            "found" to found,
            "actionClass" to actionClass,
            "text" to text,
            "description" to description,
        )
        is EditorContext -> mapOf(
            "available" to available,
            "filePath" to filePath,
            "projectRelativePath" to projectRelativePath,
            "caret" to caret.toBoundaryValue(),
            "selection" to selection.toBoundaryValue(),
            "message" to message,
        )
        is CaretInfo -> mapOf("offset" to offset, "line" to line, "column" to column)
        is SelectionInfo -> mapOf(
            "hasSelection" to hasSelection,
            "startOffset" to startOffset,
            "endOffset" to endOffset,
            "text" to text,
        )
        is PsiElementInfo -> mapOf(
            "elementClass" to elementClass,
            "elementType" to elementType,
            "name" to name,
            "text" to text,
            "filePath" to filePath,
            "projectRelativePath" to projectRelativePath,
            "line" to line,
            "column" to column,
            "language" to language,
        )
        is PsiElementAtResult -> mapOf(
            "offset" to offset,
            "found" to found,
            "element" to element.toBoundaryValue(),
            "parents" to parents.map { it.toBoundaryValue() },
        )
        is PsiResolveResult -> mapOf(
            "offset" to offset,
            "referenceClass" to referenceClass,
            "canonicalText" to canonicalText,
            "resolved" to resolved,
            "element" to element.toBoundaryValue(),
        )
        is ReferenceInfo -> mapOf(
            "referenceClass" to referenceClass,
            "canonicalText" to canonicalText,
            "filePath" to filePath,
            "projectRelativePath" to projectRelativePath,
            "line" to line,
            "column" to column,
            "text" to text,
        )
        is ReferencesResult -> mapOf(
            "target" to target.toBoundaryValue(),
            "limit" to limit,
            "count" to count,
            "references" to references.map { it.toBoundaryValue() },
        )
        is VirtualFileInfo -> mapOf(
            "filePath" to filePath,
            "projectRelativePath" to projectRelativePath,
            "fileType" to fileType,
            "psiLanguage" to psiLanguage,
            "documentAvailable" to documentAvailable,
        )
        is RefreshFileResult -> mapOf(
            "filePath" to filePath,
            "found" to found,
            "projectRelativePath" to projectRelativePath,
            "fileType" to fileType,
            "psiLanguage" to psiLanguage,
            "documentAvailable" to documentAvailable,
        )
        is ActionExecutionResult -> mapOf(
            "actionId" to actionId,
            "scheduled" to scheduled,
            "context" to context,
            "now" to now,
            "message" to message,
        )
        is UnsupportedResult -> mapOf("supported" to supported, "reason" to reason, "requires" to requires)
        is Map<*, *> -> entries.associate { it.key.toString() to it.value.toBoundaryValue() }
        is Iterable<*> -> map { it.toBoundaryValue() }
        else -> toString()
    }
}
