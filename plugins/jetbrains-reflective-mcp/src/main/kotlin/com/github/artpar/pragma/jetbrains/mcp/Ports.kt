package com.github.artpar.pragma.jetbrains.mcp

interface IdePorts {
    val application: ApplicationPort
    val actions: ActionPort
    val editor: EditorPort
    val psi: PsiPort
    val vfs: VfsPort
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
}

interface VfsPort {
    fun refreshAndFindFileByPath(filePath: String): RefreshFileResult
}
