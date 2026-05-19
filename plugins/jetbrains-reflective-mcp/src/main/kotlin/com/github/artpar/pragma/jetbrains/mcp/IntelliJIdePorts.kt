package com.github.artpar.pragma.jetbrains.mcp

import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.actionSystem.ActionPlaces
import com.intellij.openapi.application.ApplicationInfo
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.progress.EmptyProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.project.DumbService
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.util.Computable
import com.intellij.psi.PsiDocumentManager
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiManager
import com.intellij.psi.PsiNamedElement
import com.intellij.psi.PsiReference
import com.intellij.psi.search.FilenameIndex
import com.intellij.psi.search.GlobalSearchScope
import com.intellij.psi.search.PsiSearchHelper
import com.intellij.psi.search.TextOccurenceProcessor
import com.intellij.psi.search.UsageSearchContext
import com.intellij.psi.search.searches.ReferencesSearch
import java.nio.file.Path

class IntelliJIdePorts(private val project: Project) : IdePorts {
    override val application: ApplicationPort = IntelliJApplicationPort(project)
    override val actions: ActionPort = IntelliJActionPort(project)
    override val editor: EditorPort = IntelliJEditorPort(project)
    override val psi: PsiPort = IntelliJPsiPort(project)
    override val vfs: VfsPort = IntelliJVfsPort(project)
    override val agentIde: AgentIdePort = AgentIdeRuntime(project)
    override val reflection: ReflectionPort = ReflectiveRuntime(project)
}

private class IntelliJApplicationPort(private val project: Project) : ApplicationPort {
    override fun projectInfo(): ProjectInfo {
        val info = ApplicationInfo.getInstance()
        val app = ApplicationManager.getApplication()
        return ProjectInfo(
            projectName = project.name,
            projectPath = project.basePath,
            isIndexing = DumbService.isDumb(project),
            ide = IdeInfo(
                fullVersion = info.fullVersion,
                build = info.build.asString(),
                productName = info.fullApplicationName,
            ),
            application = ApplicationState(
                isDispatchThread = app.isDispatchThread,
                isReadAccessAllowed = app.isReadAccessAllowed,
                isWriteAccessAllowed = app.isWriteAccessAllowed,
            ),
        )
    }
}

private class IntelliJActionPort(private val project: Project) : ActionPort {
    override fun actionIds(prefix: String): List<String> =
        ActionManager.getInstance().getActionIdList(prefix).sorted()

    override fun actionInfo(actionId: String): ActionInfo {
        val action = ActionManager.getInstance().getAction(actionId)
        val presentation = action?.templatePresentation
        return ActionInfo(
            actionId = actionId,
            found = action != null,
            actionClass = action?.javaClass?.name,
            text = presentation?.text,
            description = presentation?.description,
        )
    }

    override fun tryToExecute(actionId: String, now: Boolean): ActionExecutionResult {
        val action = ActionManager.getInstance().getAction(actionId)
            ?: return ActionExecutionResult(actionId, scheduled = false, context = null, now = now, message = "Action not found")
        val editor = FileEditorManager.getInstance(project).selectedTextEditor
            ?: return ActionExecutionResult(actionId, scheduled = false, context = null, now = now, message = "No selected text editor")
        val component = editor.contentComponent

        ApplicationManager.getApplication().invokeLater {
            ActionManager.getInstance().tryToExecute(action, null, component, ActionPlaces.UNKNOWN, now)
        }
        return ActionExecutionResult(actionId, scheduled = true, context = "selectedTextEditor", now = now)
    }
}

private class IntelliJEditorPort(private val project: Project) : EditorPort {
    override fun selectedTextEditor(): EditorContext {
        val editor = FileEditorManager.getInstance(project).selectedTextEditor
            ?: return EditorContext(available = false, message = "No selected text editor")

        val document = editor.document
        val psiFile = readAction { PsiDocumentManager.getInstance(project).getPsiFile(document) }
        val virtualFile = psiFile?.virtualFile
        return EditorContext(
            available = true,
            filePath = virtualFile?.path,
            projectRelativePath = virtualFile?.let { project.relativePath(it) },
            caret = CaretInfo(
                offset = editor.caretModel.offset,
                line = editor.caretModel.logicalPosition.line + 1,
                column = editor.caretModel.logicalPosition.column + 1,
            ),
            selection = SelectionInfo(
                hasSelection = editor.selectionModel.hasSelection(),
                startOffset = editor.selectionModel.selectionStart,
                endOffset = editor.selectionModel.selectionEnd,
                text = editor.selectionModel.selectedText,
            ),
        )
    }
}

private class IntelliJPsiPort(private val project: Project) : PsiPort {
    override fun elementAt(position: TextPosition): PsiElementAtResult = readAction {
        val psiFile = project.findPsiFile(position.filePath) ?: error("PSI file not found")
        val document = psiFile.viewProvider.document ?: error("Document not available")
        val offset = document.offsetAt(position.line, position.column)
        val element = psiFile.findElementAt(offset)
        PsiElementAtResult(
            offset = offset,
            found = element != null,
            element = element?.toStableValue(project),
            parents = generateSequence(element?.parent) { it.parent }
                .take(8)
                .map { it.toStableValue(project) }
                .toList(),
        )
    }

    override fun resolveReference(position: TextPosition): PsiResolveResult = readAction {
        val psiFile = project.findPsiFile(position.filePath) ?: error("PSI file not found")
        val document = psiFile.viewProvider.document ?: error("Document not available")
        val offset = document.offsetAt(position.line, position.column)
        val reference = psiFile.referenceAtOrParent(offset)
        val resolved = reference?.resolve()
        PsiResolveResult(
            offset = offset,
            referenceClass = reference?.javaClass?.name,
            canonicalText = reference?.canonicalText,
            resolved = resolved != null,
            element = resolved?.toStableValue(project),
        )
    }

    override fun references(position: TextPosition, limit: Int): ReferencesResult = readAction {
        val psiFile = project.findPsiFile(position.filePath) ?: error("PSI file not found")
        val document = psiFile.viewProvider.document ?: error("Document not available")
        val offset = document.offsetAt(position.line, position.column)
        val target = psiFile.referenceAtOrParent(offset)?.resolve()
            ?: psiFile.findElementAt(offset)
            ?: error("No PSI element or resolved reference at position")
        val refs = ReferencesSearch.search(target, GlobalSearchScope.projectScope(project))
            .asIterable()
            .take(limit)
            .map { it.toReferenceValue(project) }
        ReferencesResult(
            target = target.toStableValue(project),
            limit = limit,
            count = refs.size,
            references = refs,
        )
    }

    override fun filesByName(name: String): List<VirtualFileInfo> = readAction {
        FilenameIndex.getVirtualFilesByName(name, GlobalSearchScope.projectScope(project)).map {
            VirtualFileInfo(
                filePath = it.path,
                projectRelativePath = project.relativePath(it),
                fileType = it.fileType.name,
            )
        }
    }

    override fun elementsWithWord(word: String, context: String, limit: Int, includeHidden: Boolean): WordSearchResult = readAction {
        val occurrences = mutableListOf<WordOccurrenceInfo>()
        val seen = mutableSetOf<String>()
        val normalizedContext = context.ifBlank { "any" }.lowercase()
        val searchContext = normalizedContext.toUsageSearchContext()
        val scope = GlobalSearchScope.projectScope(project)
        val scanLimit = (limit * 10).coerceAtLeast(limit).coerceAtMost(5000)
        PsiSearchHelper.getInstance(project).processElementsWithWord(
            TextOccurenceProcessor { element, offsetInElement ->
                if (occurrences.size >= scanLimit) {
                    return@TextOccurenceProcessor false
                }
                val file = element.containingFile?.virtualFile
                val document = element.containingFile?.viewProvider?.document
                val offset = (element.textRange?.startOffset ?: 0) + offsetInElement
                val line = document?.getLineNumber(offset)?.plus(1)
                val column = if (document != null && line != null) {
                    offset - document.getLineStartOffset(line - 1) + 1
                } else {
                    null
                }
                val relativePath = file?.let { project.relativePath(it) }
                if (!includeHidden && relativePath?.isHiddenProjectPath() == true) {
                    return@TextOccurenceProcessor true
                }
                val text = document?.lineTextAt(offset)?.take(500)
                val key = listOf(relativePath, line, column, text).joinToString("|")
                if (!seen.add(key)) {
                    return@TextOccurenceProcessor true
                }
                occurrences += WordOccurrenceInfo(
                    filePath = file?.path,
                    projectRelativePath = relativePath,
                    line = line,
                    column = column,
                    text = text,
                    element = element.toStableValue(project),
                )
                true
            },
            scope,
            word,
            searchContext,
            true,
        )
        val sorted = occurrences.sortedWith(
            compareBy<WordOccurrenceInfo>(
                { it.projectRelativePath ?: "" },
                { it.line ?: Int.MAX_VALUE },
                { it.column ?: Int.MAX_VALUE },
            ),
        )
        val limited = sorted.take(limit)
        WordSearchResult(
            word = word,
            context = normalizedContext,
            limit = limit,
            count = limited.size,
            truncated = sorted.size > limit,
            occurrences = limited,
        )
    }
}

private class IntelliJVfsPort(private val project: Project) : VfsPort {
    override fun refreshAndFindFileByPath(filePath: String): RefreshFileResult {
        val absolute = project.absolutePath(filePath)
        val file = LocalFileSystem.getInstance().refreshAndFindFileByPath(absolute)
        val psi = readAction { file?.let { PsiManager.getInstance(project).findFile(it) } }
        val document = file?.let { FileDocumentManager.getInstance().getDocument(it) }
        return RefreshFileResult(
            filePath = absolute,
            found = file != null,
            projectRelativePath = file?.let { project.relativePath(it) },
            fileType = file?.fileType?.name,
            psiLanguage = psi?.language?.id,
            documentAvailable = document != null,
        )
    }
}

private fun Project.findVirtualFile(path: String): VirtualFile? =
    LocalFileSystem.getInstance().refreshAndFindFileByPath(absolutePath(path))

private fun Project.findPsiFile(path: String): PsiFile? {
    val vf = findVirtualFile(path) ?: return null
    return PsiManager.getInstance(this).findFile(vf)
}

private fun Project.absolutePath(path: String): String =
    if (Path.of(path).isAbsolute) path else Path.of(basePath ?: "", path).normalize().toString()

private fun com.intellij.openapi.editor.Document.offsetAt(lineOneBased: Int, columnOneBased: Int): Int {
    val line = (lineOneBased - 1).coerceIn(0, (lineCount - 1).coerceAtLeast(0))
    val lineStart = getLineStartOffset(line)
    val lineEnd = getLineEndOffset(line)
    return (lineStart + (columnOneBased - 1).coerceAtLeast(0)).coerceIn(lineStart, lineEnd)
}

private fun com.intellij.openapi.editor.Document.lineTextAt(offset: Int): String {
    val safeOffset = offset.coerceIn(0, textLength.coerceAtLeast(0))
    val line = getLineNumber(safeOffset).coerceIn(0, (lineCount - 1).coerceAtLeast(0))
    return charsSequence.subSequence(getLineStartOffset(line), getLineEndOffset(line)).toString()
}

private fun String.toUsageSearchContext(): Short =
    when (this) {
        "code" -> UsageSearchContext.IN_CODE
        "comments" -> UsageSearchContext.IN_COMMENTS
        "strings" -> UsageSearchContext.IN_STRINGS
        "plain_text", "plaintext", "plain-text" -> UsageSearchContext.IN_PLAIN_TEXT
        "foreign_languages", "foreign", "foreign-languages" -> UsageSearchContext.IN_FOREIGN_LANGUAGES
        else -> UsageSearchContext.ANY
    }

private fun String.isHiddenProjectPath(): Boolean =
    split('/', '\\').any { it.startsWith(".") } ||
        endsWith(".jsonl") ||
        endsWith("_test.go") ||
        startsWith("build/") ||
        startsWith("dist/") ||
        startsWith("bin/") ||
        startsWith("testdata/") ||
        contains("/build/") ||
        contains("/dist/") ||
        contains("/bin/") ||
        contains("/testdata/")

private fun <T> readAction(body: () -> T): T =
    ProgressManager.getInstance().runProcess(
        Computable { ApplicationManager.getApplication().runReadAction<T>(body) },
        EmptyProgressIndicator(),
    )

private fun PsiElement.toStableValue(project: Project): PsiElementInfo {
    val file = containingFile?.virtualFile
    val document = containingFile?.viewProvider?.document
    val offset = textOffset.coerceAtLeast(0)
    val line = document?.getLineNumber(offset)?.plus(1)
    val col = if (document != null && line != null) offset - document.getLineStartOffset(line - 1) + 1 else null
    return PsiElementInfo(
        elementClass = javaClass.name,
        elementType = node?.elementType?.toString(),
        name = (this as? PsiNamedElement)?.name,
        text = text?.take(500),
        filePath = file?.path,
        projectRelativePath = file?.let { project.relativePath(it) },
        line = line,
        column = col,
        language = containingFile?.language?.id,
    )
}

private fun PsiReference.toReferenceValue(project: Project): ReferenceInfo {
    val element = element
    val file = element.containingFile?.virtualFile
    val doc = element.containingFile?.viewProvider?.document
    val start = element.textRange?.startOffset
    val line = if (doc != null && start != null) doc.getLineNumber(start).plus(1) else null
    val col = if (doc != null && line != null && start != null) start - doc.getLineStartOffset(line - 1) + 1 else null
    return ReferenceInfo(
        referenceClass = javaClass.name,
        canonicalText = canonicalText,
        filePath = file?.path,
        projectRelativePath = file?.let { project.relativePath(it) },
        line = line,
        column = col,
        text = element.text?.take(300),
    )
}

private fun Project.relativePath(file: VirtualFile): String {
    val base = basePath ?: return file.path
    return Path.of(base).relativize(Path.of(file.path)).toString()
}

private fun PsiElement.referenceAtOrParent(offset: Int): PsiReference? {
    var element: PsiElement? = containingFile?.findElementAt(offset)
    while (element != null) {
        val range = element.textRange
        if (range != null && range.containsOffset(offset)) {
            element.findReferenceAt(offset - range.startOffset)?.let { return it }
        }
        element.reference?.let { return it }
        element = element.parent
    }
    return null
}
