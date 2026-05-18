package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonObject

data object NoInput

data class PrefixInput(val prefix: String)
data class ActionIdInput(val actionId: String)
data class ActionExecuteInput(val actionId: String, val now: Boolean)
data class FilePositionInput(val position: TextPosition)
data class ReferencesInput(val position: TextPosition, val limit: Int)
data class FileNameInput(val name: String)
data class FilePathInput(val filePath: String)
data class WordSearchInput(val word: String, val context: String, val limit: Int, val includeHidden: Boolean)

fun defaultToolDescriptors(): List<ToolDescriptor<*>> = listOf(
    ToolDescriptor(
        name = "com.intellij.openapi.application.ApplicationInfo.getInstance",
        source = "ApplicationInfo.getInstance(), Project, DumbService",
        decode = { NoInput },
        execute = { _, ports -> ports.application.projectInfo() },
    ),
    ToolDescriptor(
        name = "com.intellij.openapi.actionSystem.ActionManager.getActionIdList",
        source = "ActionManager.getActionIdList(prefix)",
        inputSchema = objectSchema(
            properties = mapOf("prefix" to stringProp("Optional action ID prefix. Empty string returns every registered action ID.")),
        ),
        decode = { PrefixInput(it.stringArg("prefix")) },
        execute = { input, ports -> ports.actions.actionIds(input.prefix) },
    ),
    ToolDescriptor(
        name = "com.intellij.openapi.actionSystem.ActionManager.getAction",
        source = "ActionManager.getAction(actionId)",
        inputSchema = objectSchema(
            properties = mapOf("actionId" to stringProp("Registered IntelliJ action ID.")),
            required = listOf("actionId"),
        ),
        decode = { ActionIdInput(it.stringArg("actionId")) },
        execute = { input, ports -> ports.actions.actionInfo(input.actionId) },
    ),
    ToolDescriptor(
        name = "com.intellij.openapi.fileEditor.FileEditorManager.getSelectedTextEditor",
        source = "FileEditorManager.getInstance(project).selectedTextEditor",
        decode = { NoInput },
        execute = { _, ports -> ports.editor.selectedTextEditor() },
    ),
    ToolDescriptor(
        name = "com.intellij.psi.PsiFile.findElementAt",
        source = "PsiManager.findFile(virtualFile).findElementAt(offset)",
        inputSchema = filePositionSchema(),
        decode = { FilePositionInput(it.textPosition()) },
        execute = { input, ports -> ports.psi.elementAt(input.position) },
    ),
    ToolDescriptor(
        name = "com.intellij.psi.PsiReference.resolve",
        source = "PsiElement.referenceAtOrParent(offset).resolve()",
        inputSchema = filePositionSchema(),
        decode = { FilePositionInput(it.textPosition()) },
        execute = { input, ports -> ports.psi.resolveReference(input.position) },
    ),
    ToolDescriptor(
        name = "com.intellij.psi.search.searches.ReferencesSearch.search",
        source = "ReferencesSearch.search(resolvedElement, GlobalSearchScope.projectScope(project))",
        inputSchema = objectSchema(
            properties = filePositionProperties() + mapOf("limit" to intProp("Maximum references to return. Defaults to 200.")),
            required = listOf("filePath", "line", "column"),
        ),
        decode = { ReferencesInput(it.textPosition(), it.intArg("limit", 200).coerceIn(1, 1000)) },
        execute = { input, ports -> ports.psi.references(input.position, input.limit) },
    ),
    ToolDescriptor(
        name = "com.intellij.psi.search.FilenameIndex.getVirtualFilesByName",
        source = "FilenameIndex.getVirtualFilesByName(name, GlobalSearchScope.projectScope(project))",
        inputSchema = objectSchema(
            properties = mapOf("name" to stringProp("Exact file name to resolve through the IntelliJ filename index.")),
            required = listOf("name"),
        ),
        decode = { FileNameInput(it.stringArg("name")) },
        execute = { input, ports -> ports.psi.filesByName(input.name) },
    ),
    ToolDescriptor(
        name = "com.intellij.psi.search.PsiSearchHelper.processElementsWithWord",
        source = "PsiSearchHelper.processElementsWithWord(processor, GlobalSearchScope.projectScope(project), word, UsageSearchContext.*, true)",
        inputSchema = objectSchema(
            properties = mapOf(
                "word" to stringProp("Word to search through the IntelliJ word index."),
                "context" to stringProp("UsageSearchContext name: any, code, comments, strings, plain_text, or foreign_languages. Defaults to any."),
                "limit" to intProp("Maximum occurrences to return. Defaults to 100."),
                "includeHidden" to boolProp("If true, include dot-directories such as .git, .pragma, IDE metadata, testdata, and build output. Defaults to false."),
            ),
            required = listOf("word"),
        ),
        decode = {
            WordSearchInput(
                word = it.stringArg("word"),
                context = it.stringArg("context", "any"),
                limit = it.intArg("limit", 100).coerceIn(1, 1000),
                includeHidden = it.boolArg("includeHidden", false),
            )
        },
        execute = { input, ports -> ports.psi.elementsWithWord(input.word, input.context, input.limit, input.includeHidden) },
    ),
    ToolDescriptor(
        name = "com.intellij.openapi.vfs.LocalFileSystem.refreshAndFindFileByPath",
        source = "LocalFileSystem.refreshAndFindFileByPath(path)",
        inputSchema = objectSchema(
            properties = mapOf("filePath" to stringProp("Absolute or project-relative file path to refresh and resolve.")),
            required = listOf("filePath"),
        ),
        decode = { FilePathInput(it.stringArg("filePath")) },
        execute = { input, ports -> ports.vfs.refreshAndFindFileByPath(input.filePath) },
    ),
    ToolDescriptor(
        name = "com.intellij.openapi.actionSystem.ActionManager.tryToExecute",
        source = "ActionManager.tryToExecute(action, inputEvent, contextComponent, place, now)",
        inputSchema = objectSchema(
            properties = mapOf(
                "actionId" to stringProp("Registered IntelliJ action ID."),
                "now" to boolProp("If true, request immediate execution. Defaults to true."),
            ),
            required = listOf("actionId"),
        ),
        readOnly = false,
        destructive = true,
        decode = { ActionExecuteInput(it.stringArg("actionId"), it.boolArg("now", true)) },
        execute = { input, ports -> ports.actions.tryToExecute(input.actionId, input.now) },
    ),
)

private fun JsonObject.textPosition(): TextPosition =
    TextPosition(
        filePath = stringArg("filePath"),
        line = intArg("line", 1),
        column = intArg("column", 1),
    )

private fun filePositionSchema(): Map<String, Any> =
    objectSchema(properties = filePositionProperties(), required = listOf("filePath", "line", "column"))

private fun filePositionProperties(): Map<String, Any> = mapOf(
    "filePath" to stringProp("Absolute or project-relative file path."),
    "line" to intProp("1-based line number."),
    "column" to intProp("1-based column number."),
)
