package com.github.artpar.pragma.jetbrains.mcp

data class TextPosition(
    val filePath: String,
    val line: Int,
    val column: Int,
)

data class ProjectInfo(
    val projectName: String,
    val projectPath: String?,
    val isIndexing: Boolean,
    val ide: IdeInfo,
    val application: ApplicationState,
)

data class IdeInfo(
    val fullVersion: String,
    val build: String,
    val productName: String,
)

data class ApplicationState(
    val isDispatchThread: Boolean,
    val isReadAccessAllowed: Boolean,
    val isWriteAccessAllowed: Boolean,
)

data class ActionInfo(
    val actionId: String,
    val found: Boolean,
    val actionClass: String? = null,
    val text: String? = null,
    val description: String? = null,
)

data class EditorContext(
    val available: Boolean,
    val filePath: String? = null,
    val projectRelativePath: String? = null,
    val caret: CaretInfo? = null,
    val selection: SelectionInfo? = null,
    val message: String? = null,
)

data class CaretInfo(
    val offset: Int,
    val line: Int,
    val column: Int,
)

data class SelectionInfo(
    val hasSelection: Boolean,
    val startOffset: Int,
    val endOffset: Int,
    val text: String?,
)

data class PsiElementInfo(
    val elementClass: String?,
    val elementType: String?,
    val name: String?,
    val text: String?,
    val filePath: String?,
    val projectRelativePath: String?,
    val line: Int?,
    val column: Int?,
    val language: String?,
)

data class PsiElementAtResult(
    val offset: Int,
    val found: Boolean,
    val element: PsiElementInfo?,
    val parents: List<PsiElementInfo>,
)

data class PsiResolveResult(
    val offset: Int,
    val referenceClass: String?,
    val canonicalText: String?,
    val resolved: Boolean,
    val element: PsiElementInfo?,
)

data class ReferenceInfo(
    val referenceClass: String?,
    val canonicalText: String?,
    val filePath: String?,
    val projectRelativePath: String?,
    val line: Int?,
    val column: Int?,
    val text: String?,
)

data class ReferencesResult(
    val target: PsiElementInfo,
    val limit: Int,
    val count: Int,
    val references: List<ReferenceInfo>,
)

data class WordOccurrenceInfo(
    val filePath: String?,
    val projectRelativePath: String?,
    val line: Int?,
    val column: Int?,
    val text: String?,
    val element: PsiElementInfo?,
)

data class WordSearchResult(
    val word: String,
    val context: String,
    val limit: Int,
    val count: Int,
    val truncated: Boolean,
    val occurrences: List<WordOccurrenceInfo>,
)

data class VirtualFileInfo(
    val filePath: String,
    val projectRelativePath: String?,
    val fileType: String?,
    val psiLanguage: String? = null,
    val documentAvailable: Boolean? = null,
)

data class RefreshFileResult(
    val filePath: String,
    val found: Boolean,
    val projectRelativePath: String?,
    val fileType: String?,
    val psiLanguage: String?,
    val documentAvailable: Boolean,
)

data class ActionExecutionResult(
    val actionId: String,
    val scheduled: Boolean,
    val context: String?,
    val now: Boolean,
    val message: String? = null,
)

data class UnsupportedResult(
    val supported: Boolean = false,
    val reason: String,
    val requires: List<String> = emptyList(),
)
