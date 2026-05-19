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
data class ClassNameInput(val className: String)
data class RefInput(val ref: String)

fun defaultToolDescriptors(): List<ToolDescriptor<*>> = listOf(
    ToolDescriptor(
        name = "ide.observe",
        source = "Agent IDE object interface current project/editor scene",
        decode = { NoInput },
        execute = { _, ports -> ports.agentIde.observe() },
    ),
    ToolDescriptor(
        name = "ide.capabilities",
        source = "Agent IDE object interface capability report",
        decode = { NoInput },
        execute = { _, ports -> ports.agentIde.capabilities() },
    ),
    ToolDescriptor(
        name = "ide.object.list",
        source = "Agent IDE object registry list",
        decode = { NoInput },
        execute = { _, ports -> ports.agentIde.objects() },
    ),
    ToolDescriptor(
        name = "ide.object.describe",
        source = "Agent IDE object method catalog and metadata",
        inputSchema = objectRefSchema(),
        decode = { AgentObjectInput(it.stringArg("object")) },
        execute = { input, ports -> ports.agentIde.describeObject(input) },
    ),
    ToolDescriptor(
        name = "ide.object.call",
        source = "Agent IDE object method invocation",
        inputSchema = objectSchema(
            properties = mapOf(
                "object" to stringProp("Server-assigned object alias such as file1, doc1, editor1, symbol1, or search1."),
                "method" to stringProp("Method name from the object's method catalog."),
                "arguments" to anyProp("Method arguments as a JSON object. Defaults to {}."),
            ),
            required = listOf("object", "method"),
        ),
        readOnly = false,
        destructive = true,
        decode = { AgentObjectCallInput(it.stringArg("object"), it.stringArg("method"), it.jsonArg("arguments")) },
        execute = { input, ports -> ports.agentIde.callObject(input) },
    ),
    ToolDescriptor(
        name = "ide.object.release",
        source = "Agent IDE object registry release",
        inputSchema = objectRefSchema(),
        readOnly = false,
        destructive = true,
        decode = { AgentObjectInput(it.stringArg("object")) },
        execute = { input, ports -> ports.agentIde.releaseObject(input) },
    ),
    ToolDescriptor(
        name = "ide.file.open",
        source = "Agent IDE File object creation through the editor",
        inputSchema = filePathOnlySchema(),
        readOnly = false,
        destructive = false,
        decode = { AgentFileInput(it.stringArg("filePath")) },
        execute = { input, ports -> ports.agentIde.openFile(input) },
    ),
    ToolDescriptor(
        name = "ide.file.resolve",
        source = "Agent IDE File or Directory object creation without opening an editor",
        inputSchema = filePathOnlySchema(),
        decode = { AgentFileInput(it.stringArg("filePath")) },
        execute = { input, ports -> ports.agentIde.resolveFile(input) },
    ),
    ToolDescriptor(
        name = "ide.search.text",
        source = "Agent IDE indexed text search returning a SearchResult object",
        inputSchema = objectSchema(
            properties = mapOf(
                "query" to stringProp("Text or word to search for through the IDE project index."),
                "scope" to stringProp("Search scope hint. Defaults to project."),
                "limit" to intProp("Maximum occurrences to return. Defaults to 100."),
            ),
            required = listOf("query"),
        ),
        decode = { AgentSearchInput(it.stringArg("query"), it.stringArg("scope", "project"), it.intArg("limit", 100).coerceIn(1, 1000)) },
        execute = { input, ports -> ports.agentIde.searchText(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.list",
        source = "Agent IDE installed plugin list returning Plugin objects",
        inputSchema = objectSchema(
            properties = mapOf(
                "query" to stringProp("Optional plugin id or name substring."),
                "includeBundled" to boolProp("If true, include bundled JetBrains/platform plugins. Defaults to true."),
                "includeDisabled" to boolProp("If true, include disabled plugins. Defaults to true."),
                "limit" to intProp("Maximum plugins to return. Defaults to 200."),
            ),
        ),
        decode = {
            AgentPluginListInput(
                query = it.stringArg("query"),
                includeBundled = it.boolArg("includeBundled", true),
                includeDisabled = it.boolArg("includeDisabled", true),
                limit = it.intArg("limit", 200).coerceIn(1, 1000),
            )
        },
        execute = { input, ports -> ports.agentIde.listPlugins(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.resolve",
        source = "Agent IDE plugin descriptor object creation",
        inputSchema = pluginIdOnlySchema(),
        decode = { AgentPluginInput(it.stringArg("pluginId")) },
        execute = { input, ports -> ports.agentIde.resolvePlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.enable",
        source = "Agent IDE plugin enable through IntelliJ PluginEnabler",
        inputSchema = pluginIdOnlySchema(),
        readOnly = false,
        destructive = true,
        decode = { AgentPluginInput(it.stringArg("pluginId")) },
        execute = { input, ports -> ports.agentIde.enablePlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.disable",
        source = "Agent IDE plugin disable through IntelliJ PluginEnabler",
        inputSchema = pluginIdOnlySchema(),
        readOnly = false,
        destructive = true,
        decode = { AgentPluginInput(it.stringArg("pluginId")) },
        execute = { input, ports -> ports.agentIde.disablePlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.load",
        source = "Agent IDE dynamic plugin load through IntelliJ DynamicPlugins",
        inputSchema = pluginIdOnlySchema(),
        readOnly = false,
        destructive = true,
        decode = { AgentPluginInput(it.stringArg("pluginId")) },
        execute = { input, ports -> ports.agentIde.loadPlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.unload",
        source = "Agent IDE dynamic plugin unload through IntelliJ DynamicPlugins",
        inputSchema = pluginIdOnlySchema(),
        readOnly = false,
        destructive = true,
        decode = { AgentPluginInput(it.stringArg("pluginId")) },
        execute = { input, ports -> ports.agentIde.unloadPlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.install",
        source = "Agent IDE plugin install from Marketplace plugin id or local ZIP/path",
        inputSchema = objectSchema(
            properties = mapOf(
                "pluginId" to stringProp("Marketplace plugin id to install. Use either pluginId or path."),
                "path" to stringProp("Local plugin artifact path to install. Use either pluginId or path."),
            ),
        ),
        readOnly = false,
        destructive = true,
        decode = { AgentPluginInstallInput(it.stringArg("pluginId"), it.stringArg("path")) },
        execute = { input, ports -> ports.agentIde.installPlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.uninstall",
        source = "Agent IDE plugin uninstall through IntelliJ PluginInstaller",
        inputSchema = pluginIdOnlySchema(),
        readOnly = false,
        destructive = true,
        decode = { AgentPluginInput(it.stringArg("pluginId")) },
        execute = { input, ports -> ports.agentIde.uninstallPlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.plugin.self.update",
        source = "Agent IDE MCP plugin self-update staged through IntelliJ restart action script",
        inputSchema = filePathOnlySchema(),
        readOnly = false,
        destructive = true,
        decode = { AgentPluginSelfUpdateInput(it.stringArg("filePath")) },
        execute = { input, ports -> ports.agentIde.selfUpdatePlugin(input) },
    ),
    ToolDescriptor(
        name = "ide.debug.breakpoints",
        source = "Agent IDE breakpoint list through IntelliJ XDebuggerManager",
        decode = { NoInput },
        execute = { _, ports -> ports.agentIde.listBreakpoints() },
    ),
    ToolDescriptor(
        name = "ide.debug.breakpoint.set",
        source = "Agent IDE line breakpoint creation through IntelliJ XBreakpointManager",
        inputSchema = breakpointSchema(),
        readOnly = false,
        destructive = true,
        decode = { it.breakpointInput() },
        execute = { input, ports -> ports.agentIde.setBreakpoint(input) },
    ),
    ToolDescriptor(
        name = "ide.debug.breakpoint.remove",
        source = "Agent IDE line breakpoint removal through IntelliJ XBreakpointManager",
        inputSchema = breakpointSchema(),
        readOnly = false,
        destructive = true,
        decode = { it.breakpointInput() },
        execute = { input, ports -> ports.agentIde.removeBreakpoint(input) },
    ),
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
    ToolDescriptor(
        name = "com.github.artpar.pragma.jetbrains.reflect.Protocol.describe",
        source = "Pragma reflective bridge value protocol and handle lifetime contract",
        decode = { NoInput },
        execute = { _, ports -> ports.reflection.protocol() },
    ),
    ToolDescriptor(
        name = "com.github.artpar.pragma.jetbrains.reflect.Roots.list",
        source = "Project-scoped reflective root object registry",
        decode = { NoInput },
        execute = { _, ports -> ports.reflection.roots() },
    ),
    ToolDescriptor(
        name = "java.lang.Class.forName",
        source = "Class.forName(className)",
        inputSchema = objectSchema(
            properties = mapOf("className" to stringProp("Fully qualified JVM class name.")),
            required = listOf("className"),
        ),
        decode = { ClassNameInput(it.stringArg("className")) },
        execute = { input, ports -> ports.reflection.classForName(input.className) },
    ),
    ToolDescriptor(
        name = "java.lang.Class.describe",
        source = "Class method and field reflection metadata",
        inputSchema = reflectiveClassSchema(),
        decode = { it.reflectiveClassInput() },
        execute = { input, ports -> ports.reflection.describeClass(input) },
    ),
    ToolDescriptor(
        name = "java.lang.Class.getConstructors",
        source = "Class constructor reflection metadata",
        inputSchema = reflectiveClassSchema(),
        decode = { it.reflectiveClassInput() },
        execute = { input, ports -> ports.reflection.constructors(input) },
    ),
    ToolDescriptor(
        name = "java.lang.reflect.Field.get",
        source = "Field.get(target)",
        inputSchema = objectSchema(
            properties = mapOf(
                "className" to stringProp("Fully qualified class name for static fields, or fallback target class."),
                "targetRef" to stringProp("Reflective handle for the target object. Omit for static fields."),
                "fieldName" to stringProp("Field name."),
                "storeResult" to boolProp("If true, non-primitive results are stored as handles. Defaults to true."),
            ),
            required = listOf("fieldName"),
        ),
        decode = {
            ReflectiveFieldInput(
                className = it.stringArg("className"),
                targetRef = it.stringArg("targetRef"),
                fieldName = it.stringArg("fieldName"),
                storeResult = it.boolArg("storeResult", true),
            )
        },
        execute = { input, ports -> ports.reflection.getField(input) },
    ),
    ToolDescriptor(
        name = "java.lang.reflect.Constructor.newInstance",
        source = "Constructor.newInstance(arguments)",
        inputSchema = reflectiveConstructorSchema(),
        readOnly = false,
        destructive = true,
        decode = { it.reflectiveConstructorInput() },
        execute = { input, ports -> ports.reflection.newInstance(input) },
    ),
    ToolDescriptor(
        name = "java.lang.reflect.Method.invoke",
        source = "Method.invoke(target, arguments)",
        inputSchema = reflectiveInvocationSchema(),
        readOnly = false,
        destructive = true,
        decode = { it.reflectiveInvocationInput() },
        execute = { input, ports -> ports.reflection.invoke(input) },
    ),
    ToolDescriptor(
        name = "com.intellij.openapi.application.Application.runReadAction",
        source = "Application.runReadAction { Method.invoke(...) }",
        inputSchema = reflectiveInvocationSchema(),
        decode = { it.reflectiveInvocationInput() },
        execute = { input, ports -> ports.reflection.invokeReadAction(input) },
    ),
    ToolDescriptor(
        name = "com.intellij.openapi.command.WriteCommandAction.runWriteCommandAction",
        source = "WriteCommandAction.runWriteCommandAction(project) { Method.invoke(...) }",
        inputSchema = reflectiveInvocationSchema(),
        readOnly = false,
        destructive = true,
        decode = { it.reflectiveInvocationInput() },
        execute = { input, ports -> ports.reflection.invokeWriteCommand(input) },
    ),
    ToolDescriptor(
        name = "com.github.artpar.pragma.jetbrains.reflect.ObjectStore.list",
        source = "Reflective object handle registry list",
        decode = { NoInput },
        execute = { _, ports -> ports.reflection.handles() },
    ),
    ToolDescriptor(
        name = "com.github.artpar.pragma.jetbrains.reflect.ObjectStore.get",
        source = "Reflective object handle registry get",
        inputSchema = refSchema(),
        decode = { RefInput(it.stringArg("ref")) },
        execute = { input, ports -> ports.reflection.handle(input.ref) },
    ),
    ToolDescriptor(
        name = "com.github.artpar.pragma.jetbrains.reflect.ObjectStore.release",
        source = "Reflective object handle registry release",
        inputSchema = refSchema(),
        readOnly = false,
        destructive = true,
        decode = { RefInput(it.stringArg("ref")) },
        execute = { input, ports -> ports.reflection.release(input.ref) },
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

private fun JsonObject.reflectiveClassInput(): ReflectiveClassInput =
    ReflectiveClassInput(
        className = stringArg("className"),
        ref = stringArg("ref"),
        includeDeclared = boolArg("includeDeclared", true),
        limit = intArg("limit", 200).coerceIn(1, 2000),
    )

private fun JsonObject.reflectiveConstructorInput(): ReflectiveConstructorInput =
    ReflectiveConstructorInput(
        className = stringArg("className"),
        parameterTypes = stringListArg("parameterTypes"),
        arguments = jsonListArg("arguments"),
        storeResult = boolArg("storeResult", true),
    )

private fun JsonObject.reflectiveInvocationInput(): ReflectiveInvocationInput =
    ReflectiveInvocationInput(
        className = stringArg("className"),
        targetRef = stringArg("targetRef"),
        methodName = stringArg("methodName"),
        parameterTypes = stringListArg("parameterTypes"),
        arguments = jsonListArg("arguments"),
        storeResult = boolArg("storeResult", true),
        dispatchThread = boolArg("dispatchThread", false),
    )

private fun JsonObject.breakpointInput(): AgentBreakpointInput =
    AgentBreakpointInput(
        filePath = stringArg("filePath"),
        line = intArg("line", 1),
        typeId = stringArg("typeId"),
        enabled = boolArg("enabled", true),
        temporary = boolArg("temporary", false),
    )

private fun reflectiveClassSchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf(
            "className" to stringProp("Fully qualified JVM class name. Use this or ref."),
            "ref" to stringProp("Reflective object handle. Use this or className."),
            "includeDeclared" to boolProp("If true, include declared non-public members. Defaults to true."),
            "limit" to intProp("Maximum members to return. Defaults to 200."),
        ),
    )

private fun reflectiveConstructorSchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf(
            "className" to stringProp("Fully qualified JVM class name."),
            "parameterTypes" to arrayProp("Optional JVM parameter type names for overload resolution."),
            "arguments" to arrayProp("Arguments encoded with the reflective protocol."),
            "storeResult" to boolProp("If true, non-primitive results are stored as handles. Defaults to true."),
        ),
        required = listOf("className"),
    )

private fun reflectiveInvocationSchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf(
            "className" to stringProp("Fully qualified JVM class name for static calls, or fallback target class."),
            "targetRef" to stringProp("Reflective handle for the target object. Omit for static calls."),
            "methodName" to stringProp("Method name."),
            "parameterTypes" to arrayProp("Optional JVM parameter type names for overload resolution."),
            "arguments" to arrayProp("Arguments encoded with the reflective protocol."),
            "storeResult" to boolProp("If true, non-primitive results are stored as handles. Defaults to true."),
            "dispatchThread" to boolProp("If true, invoke from the IDE event dispatch thread. Defaults to false."),
        ),
        required = listOf("methodName"),
    )

private fun refSchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf("ref" to stringProp("Reflective object handle.")),
        required = listOf("ref"),
    )

private fun objectRefSchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf("object" to stringProp("Server-assigned object alias such as file1, doc1, editor1, symbol1, plugin1, or search1.")),
        required = listOf("object"),
    )

private fun filePathOnlySchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf("filePath" to stringProp("Absolute or project-relative file path.")),
        required = listOf("filePath"),
    )

private fun pluginIdOnlySchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf("pluginId" to stringProp("IntelliJ plugin id.")),
        required = listOf("pluginId"),
    )

private fun breakpointSchema(): Map<String, Any> =
    objectSchema(
        properties = mapOf(
            "filePath" to stringProp("Absolute or project-relative file path."),
            "line" to intProp("One-based source line number."),
            "typeId" to stringProp("Optional XLineBreakpointType id. When omitted, the first compatible line breakpoint type is used."),
            "enabled" to boolProp("If true, enable the breakpoint. Defaults to true."),
            "temporary" to boolProp("If true, create a temporary breakpoint. Defaults to false."),
        ),
        required = listOf("filePath", "line"),
    )
