package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonElement

interface IdePorts {
    val application: ApplicationPort
    val actions: ActionPort
    val editor: EditorPort
    val psi: PsiPort
    val vfs: VfsPort
    val agentIde: AgentIdePort
    val reflection: ReflectionPort
}

interface ApplicationPort {
    fun projectInfo(): ProjectInfo
}

interface ActionPort {
    fun actionIds(prefix: String): List<String>
    fun actionInfo(actionId: String): ActionInfo
    fun tryToExecute(actionId: String, now: Boolean): ActionExecutionResult
}

interface EditorPort {
    fun selectedTextEditor(): EditorContext
}

interface PsiPort {
    fun elementAt(position: TextPosition): PsiElementAtResult
    fun resolveReference(position: TextPosition): PsiResolveResult
    fun references(position: TextPosition, limit: Int): ReferencesResult
    fun filesByName(name: String): List<VirtualFileInfo>
    fun elementsWithWord(word: String, context: String, limit: Int, includeHidden: Boolean): WordSearchResult
}

interface VfsPort {
    fun refreshAndFindFileByPath(filePath: String): RefreshFileResult
}

interface AgentIdePort {
    fun observe(): Map<String, Any?>
    fun capabilities(): Map<String, Any?>
    fun objects(): Map<String, Any?>
    fun describeObject(input: AgentObjectInput): Map<String, Any?>
    fun callObject(input: AgentObjectCallInput): Map<String, Any?>
    fun releaseObject(input: AgentObjectInput): Map<String, Any?>
    fun openFile(input: AgentFileInput): Map<String, Any?>
    fun resolveFile(input: AgentFileInput): Map<String, Any?>
    fun searchText(input: AgentSearchInput): Map<String, Any?>
    fun listPlugins(input: AgentPluginListInput): Map<String, Any?>
    fun resolvePlugin(input: AgentPluginInput): Map<String, Any?>
    fun enablePlugin(input: AgentPluginInput): Map<String, Any?>
    fun disablePlugin(input: AgentPluginInput): Map<String, Any?>
    fun loadPlugin(input: AgentPluginInput): Map<String, Any?>
    fun unloadPlugin(input: AgentPluginInput): Map<String, Any?>
    fun installPlugin(input: AgentPluginInstallInput): Map<String, Any?>
    fun uninstallPlugin(input: AgentPluginInput): Map<String, Any?>
    fun selfUpdatePlugin(input: AgentPluginSelfUpdateInput): Map<String, Any?>
    fun listBreakpoints(): Map<String, Any?>
    fun setBreakpoint(input: AgentBreakpointInput): Map<String, Any?>
    fun removeBreakpoint(input: AgentBreakpointInput): Map<String, Any?>
}

interface ReflectionPort {
    fun protocol(): Map<String, Any?>
    fun roots(): Map<String, Any?>
    fun classForName(className: String): Map<String, Any?>
    fun describeClass(input: ReflectiveClassInput): Map<String, Any?>
    fun constructors(input: ReflectiveClassInput): Map<String, Any?>
    fun getField(input: ReflectiveFieldInput): Map<String, Any?>
    fun newInstance(input: ReflectiveConstructorInput): Map<String, Any?>
    fun invoke(input: ReflectiveInvocationInput): Map<String, Any?>
    fun invokeReadAction(input: ReflectiveInvocationInput): Map<String, Any?>
    fun invokeWriteCommand(input: ReflectiveInvocationInput): Map<String, Any?>
    fun handles(): Map<String, Any?>
    fun handle(ref: String): Map<String, Any?>
    fun release(ref: String): Map<String, Any?>
}

data class ReflectiveClassInput(
    val className: String,
    val ref: String,
    val includeDeclared: Boolean,
    val limit: Int,
)

data class ReflectiveFieldInput(
    val className: String,
    val targetRef: String,
    val fieldName: String,
    val storeResult: Boolean,
)

data class ReflectiveConstructorInput(
    val className: String,
    val parameterTypes: List<String>,
    val arguments: List<JsonElement>,
    val storeResult: Boolean,
)

data class ReflectiveInvocationInput(
    val className: String,
    val targetRef: String,
    val methodName: String,
    val parameterTypes: List<String>,
    val arguments: List<JsonElement>,
    val storeResult: Boolean,
    val dispatchThread: Boolean,
)

data class AgentObjectInput(val objectRef: String)

data class AgentObjectCallInput(
    val objectRef: String,
    val method: String,
    val arguments: JsonElement?,
)

data class AgentFileInput(val filePath: String)

data class AgentSearchInput(
    val query: String,
    val scope: String,
    val limit: Int,
)

data class AgentPluginListInput(
    val query: String,
    val includeBundled: Boolean,
    val includeDisabled: Boolean,
    val limit: Int,
)

data class AgentPluginInput(val pluginId: String)

data class AgentPluginInstallInput(
    val pluginId: String,
    val path: String,
)

data class AgentPluginSelfUpdateInput(val path: String)

data class AgentBreakpointInput(
    val filePath: String,
    val line: Int,
    val typeId: String,
    val enabled: Boolean,
    val temporary: Boolean,
)
