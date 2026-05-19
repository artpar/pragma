package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonElement

interface IdePorts {
    val application: ApplicationPort
    val actions: ActionPort
    val editor: EditorPort
    val psi: PsiPort
    val vfs: VfsPort
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
