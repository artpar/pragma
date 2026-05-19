package com.github.artpar.pragma.jetbrains.mcp

data class TargetApi(
    val category: String,
    val className: String,
    val methods: List<TargetMethod> = emptyList(),
    val fields: List<String> = emptyList(),
)

data class TargetMethod(
    val name: String,
    val parameterTypes: List<String>? = null,
    val static: Boolean? = null,
)

val targetApiMatrix: List<TargetApi> = listOf(
    TargetApi(
        category = "application-threading",
        className = "com.intellij.openapi.application.ApplicationManager",
        methods = listOf(TargetMethod("getApplication", emptyList(), static = true)),
    ),
    TargetApi(
        category = "application-threading",
        className = "com.intellij.openapi.application.Application",
        methods = listOf(
            TargetMethod("runReadAction", listOf("java.lang.Runnable")),
            TargetMethod("runReadAction", listOf("com.intellij.openapi.util.Computable")),
            TargetMethod("runWriteAction", listOf("java.lang.Runnable")),
            TargetMethod("invokeLater", listOf("java.lang.Runnable")),
            TargetMethod("invokeAndWait", listOf("java.lang.Runnable")),
            TargetMethod("isDispatchThread", emptyList()),
            TargetMethod("isReadAccessAllowed", emptyList()),
            TargetMethod("isWriteAccessAllowed", emptyList()),
        ),
    ),
    TargetApi(
        category = "application-threading",
        className = "com.intellij.openapi.command.WriteCommandAction",
        methods = listOf(
            TargetMethod("runWriteCommandAction", listOf("com.intellij.openapi.project.Project", "java.lang.Runnable"), static = true),
            TargetMethod("runWriteCommandAction", listOf("com.intellij.openapi.project.Project", "com.intellij.openapi.util.Computable"), static = true),
        ),
    ),
    TargetApi(
        category = "project-model",
        className = "com.intellij.openapi.project.Project",
        methods = listOf(TargetMethod("getName"), TargetMethod("getBasePath"), TargetMethod("isDisposed")),
    ),
    TargetApi(
        category = "project-model",
        className = "com.intellij.openapi.project.DumbService",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("isDumb", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("isDumb", emptyList()),
        ),
    ),
    TargetApi(
        category = "project-model",
        className = "com.intellij.openapi.roots.ProjectRootManager",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("getFileIndex", emptyList()),
            TargetMethod("getContentRoots", emptyList()),
            TargetMethod("getContentSourceRoots", emptyList()),
            TargetMethod("getProjectSdk", emptyList()),
        ),
    ),
    TargetApi(
        category = "editor-documents",
        className = "com.intellij.openapi.fileEditor.FileEditorManager",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("getSelectedTextEditor", emptyList()),
            TargetMethod("openTextEditor", listOf("com.intellij.openapi.fileEditor.OpenFileDescriptor", "boolean")),
            TargetMethod("openFile", listOf("com.intellij.openapi.vfs.VirtualFile", "boolean")),
            TargetMethod("getOpenFiles", emptyList()),
        ),
    ),
    TargetApi(
        category = "editor-documents",
        className = "com.intellij.openapi.fileEditor.FileDocumentManager",
        methods = listOf(
            TargetMethod("getInstance", emptyList(), static = true),
            TargetMethod("getDocument", listOf("com.intellij.openapi.vfs.VirtualFile")),
            TargetMethod("saveDocument", listOf("com.intellij.openapi.editor.Document")),
            TargetMethod("saveAllDocuments", emptyList()),
        ),
    ),
    TargetApi(
        category = "editor-documents",
        className = "com.intellij.openapi.editor.Document",
        methods = listOf(
            TargetMethod("getText", emptyList()),
            TargetMethod("setText", listOf("java.lang.CharSequence")),
            TargetMethod("replaceString", listOf("int", "int", "java.lang.CharSequence")),
            TargetMethod("getLineCount", emptyList()),
            TargetMethod("getLineStartOffset", listOf("int")),
            TargetMethod("getLineEndOffset", listOf("int")),
            TargetMethod("getModificationStamp", emptyList()),
        ),
    ),
    TargetApi(
        category = "vfs",
        className = "com.intellij.openapi.vfs.LocalFileSystem",
        methods = listOf(
            TargetMethod("getInstance", emptyList(), static = true),
            TargetMethod("refreshAndFindFileByPath", listOf("java.lang.String")),
            TargetMethod("refreshAndFindFileByNioFile", listOf("java.nio.file.Path")),
        ),
    ),
    TargetApi(
        category = "vfs",
        className = "com.intellij.openapi.vfs.VirtualFile",
        methods = listOf(
            TargetMethod("getPath", emptyList()),
            TargetMethod("getName", emptyList()),
            TargetMethod("getChildren", emptyList()),
            TargetMethod("contentsToByteArray", emptyList()),
            TargetMethod("setBinaryContent", listOf("[B")),
            TargetMethod("isDirectory", emptyList()),
            TargetMethod("isWritable", emptyList()),
        ),
    ),
    TargetApi(
        category = "psi-navigation",
        className = "com.intellij.psi.PsiManager",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("findFile", listOf("com.intellij.openapi.vfs.VirtualFile")),
            TargetMethod("findDirectory", listOf("com.intellij.openapi.vfs.VirtualFile")),
        ),
    ),
    TargetApi(
        category = "psi-navigation",
        className = "com.intellij.psi.PsiDocumentManager",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("getPsiFile", listOf("com.intellij.openapi.editor.Document")),
            TargetMethod("getDocument", listOf("com.intellij.psi.PsiFile")),
            TargetMethod("commitDocument", listOf("com.intellij.openapi.editor.Document")),
        ),
    ),
    TargetApi(
        category = "psi-navigation",
        className = "com.intellij.psi.PsiElement",
        methods = listOf(
            TargetMethod("findElementAt", listOf("int")),
            TargetMethod("findReferenceAt", listOf("int")),
            TargetMethod("getReference", emptyList()),
            TargetMethod("getReferences", emptyList()),
            TargetMethod("getText", emptyList()),
            TargetMethod("getTextRange", emptyList()),
            TargetMethod("replace", listOf("com.intellij.psi.PsiElement")),
            TargetMethod("delete", emptyList()),
        ),
    ),
    TargetApi(
        category = "search-indexes",
        className = "com.intellij.psi.search.GlobalSearchScope",
        methods = listOf(
            TargetMethod("projectScope", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("allScope", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("fileScope", listOf("com.intellij.psi.PsiFile"), static = true),
            TargetMethod("filesScope", listOf("com.intellij.openapi.project.Project", "java.util.Collection"), static = true),
        ),
    ),
    TargetApi(
        category = "search-indexes",
        className = "com.intellij.psi.search.FilenameIndex",
        methods = listOf(
            TargetMethod("getVirtualFilesByName", listOf("java.lang.String", "com.intellij.psi.search.GlobalSearchScope"), static = true),
            TargetMethod("getFilesByName", listOf("com.intellij.openapi.project.Project", "java.lang.String", "com.intellij.psi.search.GlobalSearchScope"), static = true),
            TargetMethod("processFilesByName"),
        ),
    ),
    TargetApi(
        category = "search-indexes",
        className = "com.intellij.psi.search.PsiSearchHelper",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("processElementsWithWord", listOf("com.intellij.psi.search.TextOccurenceProcessor", "com.intellij.psi.search.SearchScope", "java.lang.String", "short", "boolean")),
            TargetMethod("processAllFilesWithWord"),
        ),
    ),
    TargetApi(
        category = "search-indexes",
        className = "com.intellij.psi.search.searches.ReferencesSearch",
        methods = listOf(
            TargetMethod("search", listOf("com.intellij.psi.PsiElement"), static = true),
            TargetMethod("search", listOf("com.intellij.psi.PsiElement", "com.intellij.psi.search.SearchScope"), static = true),
            TargetMethod("searchOptimized", static = true),
        ),
    ),
    TargetApi(
        category = "actions",
        className = "com.intellij.openapi.actionSystem.ActionManager",
        methods = listOf(
            TargetMethod("getInstance", emptyList(), static = true),
            TargetMethod("getAction", listOf("java.lang.String")),
            TargetMethod("getActionIdList", listOf("java.lang.String")),
            TargetMethod("tryToExecute"),
        ),
    ),
    TargetApi(
        category = "intentions-quickfixes",
        className = "com.intellij.codeInsight.intention.IntentionManager",
        methods = listOf(
            TargetMethod("getInstance", emptyList(), static = true),
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("getIntentionActions", emptyList()),
            TargetMethod("getAvailableIntentions", emptyList()),
            TargetMethod("convertToFix", listOf("com.intellij.codeInsight.intention.IntentionAction")),
        ),
    ),
    TargetApi(
        category = "intentions-quickfixes",
        className = "com.intellij.codeInsight.intention.IntentionAction",
        methods = listOf(
            TargetMethod("getText", emptyList()),
            TargetMethod("isAvailable", listOf("com.intellij.openapi.project.Project", "com.intellij.openapi.editor.Editor", "com.intellij.psi.PsiFile")),
            TargetMethod("invoke", listOf("com.intellij.openapi.project.Project", "com.intellij.openapi.editor.Editor", "com.intellij.psi.PsiFile")),
            TargetMethod("startInWriteAction", emptyList()),
        ),
    ),
    TargetApi(
        category = "diagnostics-inspections",
        className = "com.intellij.codeInsight.daemon.DaemonCodeAnalyzer",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("isHighlightingAvailable", listOf("com.intellij.psi.PsiFile")),
            TargetMethod("restart", emptyList()),
            TargetMethod("restart", listOf("com.intellij.psi.PsiFile")),
        ),
    ),
    TargetApi(
        category = "diagnostics-inspections",
        className = "com.intellij.profile.codeInspection.InspectionProjectProfileManager",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("getInspectionProfile", emptyList()),
        ),
    ),
    TargetApi(
        category = "diagnostics-inspections",
        className = "com.intellij.codeInspection.ex.GlobalInspectionContextBase",
        methods = listOf(
            TargetMethod("doInspections", listOf("com.intellij.analysis.AnalysisScope")),
            TargetMethod("getTools", emptyList()),
            TargetMethod("getUsedTools", emptyList()),
            TargetMethod("codeCleanup"),
        ),
    ),
    TargetApi(
        category = "duplicates",
        className = "com.intellij.dupLocator.DuplicatesProfile",
        methods = listOf(
            TargetMethod("getAllProfiles", emptyList(), static = true),
            TargetMethod("findProfileForLanguage", listOf("com.intellij.lang.Language"), static = true),
            TargetMethod("createVisitor", listOf("com.intellij.dupLocator.treeHash.FragmentsCollector")),
            TargetMethod("getDuplocatorState", listOf("com.intellij.lang.Language")),
            TargetMethod("supportDuplicatesIndex", emptyList()),
        ),
    ),
    TargetApi(
        category = "duplicates",
        className = "com.intellij.dupLocator.LightDuplicateProfile",
        methods = listOf(
            TargetMethod("process", listOf("com.intellij.lang.LighterAST", "com.intellij.dupLocator.LightDuplicateProfile\$Callback")),
            TargetMethod("acceptsFile", listOf("com.intellij.openapi.vfs.VirtualFile")),
        ),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.RefactoringFactory",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("createRename", listOf("com.intellij.psi.PsiElement", "java.lang.String")),
            TargetMethod("createRename", listOf("com.intellij.psi.PsiElement", "java.lang.String", "boolean", "boolean")),
            TargetMethod("createSafeDelete", listOf("[Lcom.intellij.psi.PsiElement;")),
        ),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.extractMethod.ExtractMethodHandler",
        methods = listOf(
            TargetMethod("invoke", listOf("com.intellij.openapi.project.Project", "com.intellij.openapi.editor.Editor", "com.intellij.psi.PsiFile", "com.intellij.openapi.actionSystem.DataContext")),
            TargetMethod("invoke", listOf("com.intellij.openapi.project.Project", "[Lcom.intellij.psi.PsiElement;", "com.intellij.openapi.actionSystem.DataContext")),
            TargetMethod("getElements", static = true),
            TargetMethod("getProcessor", static = true),
        ),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.extractclass.ExtractClassHandler",
        methods = listOf(TargetMethod("invoke"), TargetMethod("isEnabledOnElements"), TargetMethod("getRefactoringName", emptyList(), static = true)),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.extractInterface.ExtractInterfaceHandler",
        methods = listOf(TargetMethod("invoke"), TargetMethod("isEnabledOnElements"), TargetMethod("getRefactoringName", emptyList(), static = true)),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.changeSignature.ChangeSignatureHandler",
        methods = listOf(TargetMethod("findTargetMember"), TargetMethod("invoke"), TargetMethod("getTargetNotFoundMessage")),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.introduceVariable.IntroduceVariableHandler",
        methods = listOf(TargetMethod("invoke"), TargetMethod("generatePreview")),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.introduceField.IntroduceFieldHandler",
        methods = listOf(TargetMethod("invoke"), TargetMethod("getRefactoringNameText", emptyList(), static = true)),
    ),
    TargetApi(
        category = "refactoring",
        className = "com.intellij.refactoring.safeDelete.SafeDeleteProcessor",
        methods = listOf(TargetMethod("createInstance", static = true), TargetMethod("findUsages"), TargetMethod("performRefactoring")),
    ),
    TargetApi(
        category = "run-build-test",
        className = "com.intellij.execution.RunManager",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("getAllSettings", emptyList()),
            TargetMethod("getAllConfigurationsList", emptyList()),
            TargetMethod("getSelectedConfiguration", emptyList()),
            TargetMethod("findConfigurationByName", listOf("java.lang.String")),
        ),
    ),
    TargetApi(
        category = "run-build-test",
        className = "com.intellij.execution.ExecutionManager",
        methods = listOf(
            TargetMethod("getInstance", listOf("com.intellij.openapi.project.Project"), static = true),
            TargetMethod("getRunningProcesses", emptyList()),
            TargetMethod("restartRunProfile"),
            TargetMethod("getContentManager", emptyList()),
        ),
    ),
    TargetApi(
        category = "run-build-test",
        className = "com.intellij.execution.ProgramRunnerUtil",
        methods = listOf(TargetMethod("executeConfiguration", static = true), TargetMethod("executeConfigurationAsync", static = true), TargetMethod("getRunner", static = true)),
    ),
)
