package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.intellij.codeInsight.daemon.impl.DaemonCodeAnalyzerImpl
import com.intellij.codeInsight.daemon.impl.HighlightInfo
import com.intellij.ide.plugins.DynamicPlugins
import com.intellij.ide.plugins.IdeaPluginDescriptor
import com.intellij.ide.plugins.IdeaPluginDescriptorImpl
import com.intellij.ide.plugins.InstalledPluginsState
import com.intellij.ide.plugins.PluginEnabler
import com.intellij.ide.plugins.PluginInstaller
import com.intellij.ide.plugins.PluginManagerCore
import com.intellij.ide.plugins.RepositoryHelper
import com.intellij.execution.ExecutionManager
import com.intellij.execution.Executor
import com.intellij.execution.ExecutorRegistry
import com.intellij.execution.ProgramRunnerUtil
import com.intellij.execution.RunManager
import com.intellij.execution.RunnerAndConfigurationSettings
import com.intellij.execution.configurations.ConfigurationFactory
import com.intellij.execution.configurations.ConfigurationType
import com.intellij.execution.configurations.RunConfiguration
import com.intellij.execution.executors.DefaultDebugExecutor
import com.intellij.execution.executors.DefaultRunExecutor
import com.intellij.execution.process.ProcessAdapter
import com.intellij.execution.process.ProcessEvent
import com.intellij.execution.process.ProcessHandler
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.editor.Document
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.extensions.PluginId
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.TextEditor
import com.intellij.openapi.progress.EmptyProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.project.DumbService
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Key
import com.intellij.openapi.util.Computable
import com.intellij.openapi.updateSettings.impl.PluginDownloader
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.psi.PsiDocumentManager
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiManager
import com.intellij.psi.PsiNamedElement
import com.intellij.psi.search.GlobalSearchScope
import com.intellij.psi.search.PsiSearchHelper
import com.intellij.psi.search.TextOccurenceProcessor
import com.intellij.psi.search.UsageSearchContext
import com.intellij.psi.search.searches.ReferencesSearch
import com.intellij.refactoring.rename.RenameProcessor
import com.intellij.usageView.UsageInfo
import com.intellij.xdebugger.XDebuggerManager
import com.intellij.xdebugger.breakpoints.XBreakpoint
import com.intellij.xdebugger.breakpoints.XBreakpointProperties
import com.intellij.xdebugger.breakpoints.XBreakpointType
import com.intellij.xdebugger.breakpoints.XLineBreakpoint
import com.intellij.xdebugger.breakpoints.XLineBreakpointType
import java.lang.reflect.Modifier
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference

class AgentIdeRuntime(private val project: Project) : AgentIdePort {
    private companion object {
        const val SELF_PLUGIN_ID = "com.github.artpar.pragma.jetbrains.reflectiveMcp"
    }

    private val objects = AgentObjectRegistry(project)
    private val executionBuffers = ConcurrentHashMap<ProcessHandler, StringBuilder>()

    override fun observe(): Map<String, Any?> = ideOk(
        summary = "Observed current IDE project state.",
        data = mapOf(
            "project" to projectInfo(),
            "editor" to selectedEditorInfo(),
            "openFiles" to openFileInfos(),
            "languages" to observedLanguages(),
        ),
        nextBestActions = listOf("ide.file.open", "ide.file.resolve", "ide.file.create", "ide.search.text", "ide.object.list"),
    )

    override fun capabilities(): Map<String, Any?> = ideOk(
        summary = "Reported IDE-native capability surface.",
        data = mapOf(
            "strictIdeOnly" to true,
            "taskFamilies" to listOf("files", "editor", "documents", "text_search", "semantic_psi_when_native", "run_configurations", "executions", "actions", "plugins", "debug_breakpoints", "reflection_escape_hatch"),
            "unsupportedFallbacks" to listOf("shell", "external_lsp", "filesystem_tools"),
            "semanticPolicy" to "Semantic tools return native_psi_unavailable when the IDE reports non-semantic PSI such as TextMate.",
            "editSafetyPolicy" to "replace and replaceOffsets require expectedText or oldText matching the current document range before mutation.",
            "pluginPolicy" to "Plugin load/unload/install/uninstall calls use IntelliJ plugin APIs and report restart_required when the platform cannot apply the change dynamically.",
            "runConfigurationPolicy" to "Run/build/test/debug work should use ide.run.* tools. ActionManager.tryToExecute is only a scheduled UI-action fallback.",
            "reflectiveEscapeHatch" to listOf(
                "com.github.artpar.pragma.jetbrains.reflect.Roots.list",
                "java.lang.Class.describe",
                "java.lang.reflect.Method.invoke",
            ),
        ),
        nextBestActions = listOf("ide.observe", "ide.file.open", "ide.run.config.list", "ide.run.config.types", "ide.object.describe"),
    )

    override fun objects(): Map<String, Any?> = ideOk(
        summary = "Listed live agent IDE objects.",
        data = mapOf("objects" to objects.list().map { objectValue(it) }),
    )

    override fun describeObject(input: AgentObjectInput): Map<String, Any?> {
        val entry = objects.get(input.objectRef) ?: return ideError("object_not_found", "No live IDE object named ${input.objectRef}.")
        return ideOk(
            summary = "Described ${entry.alias}.",
            data = mapOf("object" to objectValue(entry)),
            affordances = methodCatalog(entry),
        )
    }

    override fun releaseObject(input: AgentObjectInput): Map<String, Any?> {
        val released = objects.release(input.objectRef)
        return if (released) {
            ideOk("Released ${input.objectRef}.", data = mapOf("released" to true))
        } else {
            ideError("object_not_found", "No releasable IDE object named ${input.objectRef}.")
        }
    }

    override fun openFile(input: AgentFileInput): Map<String, Any?> {
        val file = resolveVirtualFile(input.filePath) ?: return ideError("file_not_found", "File not found: ${input.filePath}")
        val fileEntry = objects.put("file", file)
        val editorEntry = runCatching {
            val editors = runOnEdt { FileEditorManager.getInstance(project).openFile(file, true) }
            val editor = (editors.firstOrNull() as? TextEditor)?.editor
                ?: FileEditorManager.getInstance(project).selectedTextEditor
            editor?.let { objects.put("editor", it) }
        }.getOrNull()
        return ideOk(
            summary = "Opened ${file.path}.",
            data = mapOf(
                "object" to objectValue(fileEntry),
                "editor" to editorEntry?.let { objectValue(it) },
            ),
            affordances = methodCatalog(fileEntry),
            nextBestActions = listOf("${fileEntry.alias}.read", "${fileEntry.alias}.search", "${fileEntry.alias}.openEditor"),
        )
    }

    override fun resolveFile(input: AgentFileInput): Map<String, Any?> {
        val absolute = project.absolutePath(input.filePath)
        val file = LocalFileSystem.getInstance().refreshAndFindFileByPath(absolute)
        return if (file == null) {
            ideError("file_not_found", "File not found: $absolute")
        } else {
            val entry = objects.put(if (file.isDirectory) "directory" else "file", file)
            ideOk(
                summary = "Resolved $absolute.",
                data = mapOf("object" to objectValue(entry)),
                affordances = methodCatalog(entry),
            )
        }
    }

    override fun createFile(input: AgentFileCreateInput): Map<String, Any?> {
        val path = Path.of(project.absolutePath(input.filePath)).normalize()
        val parentPath = path.parent ?: return ideError("invalid_path", "File path has no parent: $path")
        val fileName = path.fileName?.toString()?.takeIf { it.isNotBlank() }
            ?: return ideError("invalid_path", "File path has no file name: $path")

        val existing = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(path)
        if (existing?.isDirectory == true) return ideError("invalid_path", "Path is a directory: $path")
        if (existing != null && !input.overwrite) {
            return ideError(
                "file_exists",
                "File already exists: $path",
                nextBestActions = listOf("ide.file.open", "ide.file.resolve"),
            )
        }

        val fileRef = AtomicReference<VirtualFile?>()
        val docRef = AtomicReference<Document?>()
        runOnEdt {
            WriteCommandAction.runWriteCommandAction(project) {
                Files.createDirectories(parentPath)
                LocalFileSystem.getInstance().refreshAndFindFileByNioFile(parentPath)?.refresh(false, true)
                val parent = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(parentPath)
                    ?: error("Parent directory unavailable after creation: $parentPath")
                val file = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(path)
                    ?: parent.createChildData(this, fileName)
                val doc = FileDocumentManager.getInstance().getDocument(file)
                if (doc != null) {
                    doc.setText(input.text)
                    FileDocumentManager.getInstance().saveDocument(doc)
                } else {
                    file.setBinaryContent(input.text.toByteArray())
                }
                file.refresh(false, false)
                fileRef.set(file)
                docRef.set(doc)
            }
        }

        val file = fileRef.get() ?: return ideError("file_not_found", "Created file was not resolved: $path")
        val fileEntry = objects.put("file", file)
        val docEntry = docRef.get()?.let { objects.put("doc", it) }
        val editorEntry = if (input.openEditor) {
            runCatching {
                val editors = runOnEdt { FileEditorManager.getInstance(project).openFile(file, true) }
                val editor = (editors.firstOrNull() as? TextEditor)?.editor
                    ?: FileEditorManager.getInstance(project).selectedTextEditor
                editor?.let { objects.put("editor", it) }
            }.getOrNull()
        } else {
            null
        }
        return ideOk(
            summary = "Created ${file.path}.",
            data = mapOf(
                "object" to objectValue(fileEntry),
                "document" to docEntry?.let { objectValue(it) },
                "editor" to editorEntry?.let { objectValue(it) },
                "path" to file.path,
                "overwritten" to (existing != null),
            ),
            affordances = methodCatalog(fileEntry),
            nextBestActions = listOf("${fileEntry.alias}.read", docEntry?.let { "${it.alias}.save" } ?: "${fileEntry.alias}.openEditor"),
        )
    }

    override fun searchText(input: AgentSearchInput): Map<String, Any?> {
        if (input.query.isBlank()) return ideError("invalid_query", "Search query must not be empty.")
        val result = textSearch(input.query, input.limit)
        val entry = objects.put("searchResult", result)
        return ideOk(
            summary = "Found ${result.items.size} occurrence(s) for ${input.query}.",
            data = mapOf("object" to objectValue(entry), "items" to result.items.map { it.toValue() }),
            affordances = methodCatalog(entry),
        )
    }

    override fun listPlugins(input: AgentPluginListInput): Map<String, Any?> {
        val query = input.query.trim()
        val plugins = PluginManagerCore.plugins.asSequence()
            .filter { input.includeBundled || !it.isBundled }
            .filter { input.includeDisabled || !PluginEnabler.getInstance().isDisabled(it.pluginId) }
            .filter {
                query.isEmpty() ||
                    it.pluginId.idString.contains(query, ignoreCase = true) ||
                    it.name.orEmpty().contains(query, ignoreCase = true)
            }
            .sortedWith(compareBy<IdeaPluginDescriptor> { !it.isBundled }.thenBy { it.name.orEmpty() })
            .take(input.limit)
            .map { objects.put("plugin", it) }
            .toList()
        return ideOk(
            summary = "Listed ${plugins.size} plugin(s).",
            data = mapOf("objects" to plugins.map { objectValue(it) }),
            nextBestActions = listOf("ide.plugin.resolve", "ide.object.describe", "ide.plugin.enable", "ide.plugin.disable"),
        )
    }

    override fun resolvePlugin(input: AgentPluginInput): Map<String, Any?> {
        val descriptor = pluginDescriptor(input.pluginId) ?: return ideError("plugin_not_found", "Plugin not found: ${input.pluginId}")
        val entry = objects.put("plugin", descriptor)
        return ideOk(
            summary = "Resolved plugin ${descriptor.pluginId.idString}.",
            data = mapOf("object" to objectValue(entry), "plugin" to descriptor.toPluginValue()),
            affordances = methodCatalog(entry),
        )
    }

    override fun enablePlugin(input: AgentPluginInput): Map<String, Any?> =
        mutatePlugin(input.pluginId, "Enabled") { descriptor ->
            val changed = runOnEdt { PluginEnabler.getInstance().enable(listOf(descriptor)) }
            if (isSelfPlugin(descriptor)) markRestartRequired()
            ideOk(
                summary = if (isSelfPlugin(descriptor)) {
                    "Enabled ${descriptor.pluginId.idString}; restart IntelliJ and reconnect the agent if this changed plugin state."
                } else {
                    "Enabled ${descriptor.pluginId.idString}."
                },
                data = pluginMutationData(descriptor, changed) + selfLifecycleData(descriptor, requestedOperation = "enable"),
                nextBestActions = selfRestartActions(descriptor),
            )
        }

    override fun disablePlugin(input: AgentPluginInput): Map<String, Any?> =
        mutatePlugin(input.pluginId, "Disabled") { descriptor ->
            val changed = runOnEdt { PluginEnabler.getInstance().disable(listOf(descriptor)) }
            if (isSelfPlugin(descriptor)) markRestartRequired()
            ideOk(
                summary = if (isSelfPlugin(descriptor)) {
                    "Disabled ${descriptor.pluginId.idString}; restart IntelliJ and reconnect the agent to apply the MCP server removal."
                } else {
                    "Disabled ${descriptor.pluginId.idString}."
                },
                data = pluginMutationData(descriptor, changed) + selfLifecycleData(descriptor, requestedOperation = "disable"),
                nextBestActions = selfRestartActions(descriptor),
            )
        }

    override fun loadPlugin(input: AgentPluginInput): Map<String, Any?> =
        mutatePlugin(input.pluginId, "Loaded") { descriptor ->
            val impl = descriptor as? IdeaPluginDescriptorImpl
                ?: return@mutatePlugin ideError("unsupported_plugin_descriptor", "Plugin ${descriptor.pluginId.idString} is not backed by an IdeaPluginDescriptorImpl.")
            val changed = runOnEdt { DynamicPlugins.loadPlugin(impl, project) }
            ideOk("Loaded ${descriptor.pluginId.idString}.", data = pluginMutationData(descriptor, changed))
        }

    override fun unloadPlugin(input: AgentPluginInput): Map<String, Any?> =
        mutatePlugin(input.pluginId, "Unloaded") { descriptor ->
            if (isSelfPlugin(descriptor)) {
                val changed = runOnEdt { PluginEnabler.getInstance().disable(listOf(descriptor)) }
                markRestartRequired()
                return@mutatePlugin ideOk(
                    summary = "Scheduled ${descriptor.pluginId.idString} to unload by disabling it after restart. Restart IntelliJ and reconnect the agent.",
                    data = pluginMutationData(descriptor, changed) + selfLifecycleData(
                        descriptor,
                        requestedOperation = "unload",
                        actualOperation = "disableAfterRestart",
                    ),
                    nextBestActions = selfRestartActions(descriptor),
                )
            }
            val impl = descriptor as? IdeaPluginDescriptorImpl
                ?: return@mutatePlugin ideError("unsupported_plugin_descriptor", "Plugin ${descriptor.pluginId.idString} is not backed by an IdeaPluginDescriptorImpl.")
            val cannotUnload = DynamicPlugins.checkCanUnloadWithoutRestart(impl)
            if (cannotUnload != null) {
                return@mutatePlugin ideUnavailable(
                    "restart_required",
                    "Plugin ${descriptor.pluginId.idString} cannot be unloaded dynamically: $cannotUnload",
                    nextBestActions = listOf("ide.plugin.disable"),
                )
            }
            val changed = runOnEdt { DynamicPlugins.unloadPlugin(impl) }
            ideOk("Unloaded ${descriptor.pluginId.idString}.", data = pluginMutationData(descriptor, changed))
        }

    override fun installPlugin(input: AgentPluginInstallInput): Map<String, Any?> {
        if (input.path.isNotBlank()) {
            val path = Path.of(project.absolutePath(input.path))
            val descriptor = loadPluginDescriptorFromArtifact(path)
                ?: return ideError("plugin_descriptor_unavailable", "Could not read plugin descriptor from $path.")
            if (isSelfPlugin(descriptor)) {
                return stageSelfUpdate(path, descriptor)
            }
            val changed = runOnEdt { PluginInstaller.installAndLoadDynamicPlugin(path, descriptor) }
            val entry = objects.put("plugin", descriptor)
            return ideOk(
                summary = "Installed ${descriptor.pluginId.idString} from $path.",
                data = mapOf("changed" to changed, "object" to objectValue(entry), "plugin" to descriptor.toPluginValue(), "restartRequired" to restartRequired()),
                affordances = methodCatalog(entry),
            )
        }

        if (input.pluginId.isBlank()) {
            return ideError("invalid_arguments", "Provide pluginId or path.")
        }

        val pluginId = PluginId.getId(input.pluginId)
        val descriptor = RepositoryHelper.loadPlugins(setOf(pluginId)).firstOrNull { it.pluginId == pluginId }
            ?: return ideError("plugin_not_found", "Marketplace plugin not found: ${input.pluginId}")
        val downloader = PluginDownloader.createDownloader(descriptor).withErrorsConsumer { }
        val prepared = downloader.prepareToInstall(EmptyProgressIndicator())
        if (!prepared) return ideError("plugin_install_failed", "Could not prepare ${input.pluginId} for installation.")
        val dynamic = runCatching { runOnEdt { downloader.installDynamically(null) } }.getOrDefault(false)
        if (!dynamic) runOnEdt { downloader.install() }
        val installed = pluginDescriptor(input.pluginId) ?: descriptor
        val entry = objects.put("plugin", installed)
        return ideOk(
            summary = if (dynamic) "Installed and loaded ${input.pluginId}." else "Installed ${input.pluginId}; restart may be required.",
            data = mapOf("dynamic" to dynamic, "object" to objectValue(entry), "plugin" to installed.toPluginValue(), "restartRequired" to restartRequired()),
            affordances = methodCatalog(entry),
        )
    }

    override fun uninstallPlugin(input: AgentPluginInput): Map<String, Any?> =
        mutatePlugin(input.pluginId, "Uninstalled") { descriptor ->
            val impl = descriptor as? IdeaPluginDescriptorImpl
                ?: return@mutatePlugin ideError("unsupported_plugin_descriptor", "Plugin ${descriptor.pluginId.idString} is not backed by an IdeaPluginDescriptorImpl.")
            if (isSelfPlugin(descriptor)) {
                runOnEdt { PluginInstaller.prepareToUninstall(impl) }
                markRestartRequired()
                return@mutatePlugin ideOk(
                    summary = "Scheduled ${descriptor.pluginId.idString} for uninstall after restart. Restart IntelliJ and reconnect the agent through a freshly installed plugin if needed.",
                    data = pluginMutationData(descriptor, true) + selfLifecycleData(
                        descriptor,
                        requestedOperation = "uninstall",
                        actualOperation = "uninstallAfterRestart",
                    ) + mapOf("dynamic" to false),
                    nextBestActions = selfRestartActions(descriptor),
                )
            }
            val dynamic = runCatching { runOnEdt { PluginInstaller.uninstallDynamicPlugin(null, impl, false) } }.getOrDefault(false)
            if (!dynamic) runOnEdt { PluginInstaller.prepareToUninstall(impl) }
            ideOk(
                summary = if (dynamic) "Uninstalled ${descriptor.pluginId.idString}." else "Scheduled ${descriptor.pluginId.idString} for uninstall after restart.",
                data = pluginMutationData(descriptor, true) + mapOf("dynamic" to dynamic),
            )
        }

    override fun selfUpdatePlugin(input: AgentPluginSelfUpdateInput): Map<String, Any?> {
        val path = Path.of(project.absolutePath(input.path))
        val descriptor = loadPluginDescriptorFromArtifact(path)
            ?: return ideError("plugin_descriptor_unavailable", "Could not read plugin descriptor from $path.")
        if (!isSelfPlugin(descriptor)) {
            return ideError(
                "plugin_id_mismatch",
                "Artifact $path contains ${descriptor.pluginId.idString}, expected $SELF_PLUGIN_ID.",
            )
        }
        return stageSelfUpdate(path, descriptor)
    }

    override fun listBreakpoints(): Map<String, Any?> {
        val entries = breakpointManager().allBreakpoints.map { objects.put("breakpoint", it) }
        return ideOk(
            summary = "Listed ${entries.size} breakpoint(s).",
            data = mapOf(
                "objects" to entries.map { objectValue(it) },
                "lineBreakpointTypes" to lineBreakpointTypes().map { mapOf("id" to it.id, "title" to it.title) },
            ),
            nextBestActions = listOf("ide.debug.breakpoint.set", "ide.object.describe"),
        )
    }

    override fun setBreakpoint(input: AgentBreakpointInput): Map<String, Any?> {
        val file = resolveVirtualFile(input.filePath) ?: return ideError("file_not_found", "File not found: ${input.filePath}")
        val line = (input.line - 1).coerceAtLeast(0)
        val type = lineBreakpointTypeFor(file, line, input.typeId)
            ?: return ideError("breakpoint_type_not_found", "No compatible line breakpoint type found for ${file.path}:${input.line}.")
        val manager = breakpointManager()
        val breakpoint = runOnEdt {
            val existing = manager.findBreakpointAtLine(type, file, line)
            val target = existing ?: manager.addLineBreakpoint(type, file.url, line, type.createBreakpointProperties(file, line), input.temporary)
            target.isEnabled = input.enabled
            target.isTemporary = input.temporary
            target
        }
        val entry = objects.put("breakpoint", breakpoint)
        return ideOk(
            summary = "Set breakpoint at ${file.path}:${input.line}.",
            data = mapOf("object" to objectValue(entry), "breakpoint" to breakpoint.toBreakpointValue()),
            affordances = methodCatalog(entry),
        )
    }

    override fun removeBreakpoint(input: AgentBreakpointInput): Map<String, Any?> {
        val file = resolveVirtualFile(input.filePath) ?: return ideError("file_not_found", "File not found: ${input.filePath}")
        val line = (input.line - 1).coerceAtLeast(0)
        val type = lineBreakpointTypeFor(file, line, input.typeId)
            ?: return ideError("breakpoint_type_not_found", "No compatible line breakpoint type found for ${file.path}:${input.line}.")
        val removed = runOnEdt {
            val breakpoint = breakpointManager().findBreakpointAtLine(type, file, line) ?: return@runOnEdt false
            breakpointManager().removeBreakpoint(breakpoint)
            true
        }
        if (!removed) return ideError("breakpoint_not_found", "No breakpoint found at ${file.path}:${input.line}.")
        return ideOk("Removed breakpoint at ${file.path}:${input.line}.", data = mapOf("filePath" to file.path, "line" to input.line))
    }

    override fun listRunConfigurationTypes(input: AgentRunConfigTypesInput): Map<String, Any?> {
        val query = input.query.trim()
        val entries = configurationTypes()
            .filter { type ->
                query.isBlank() ||
                    type.id.contains(query, ignoreCase = true) ||
                    type.displayName.contains(query, ignoreCase = true) ||
                    type.configurationFactories.any {
                        it.id.contains(query, ignoreCase = true) || it.name.contains(query, ignoreCase = true)
                    }
            }
            .take(input.limit)
            .map { objects.put("runConfigType", it) }
        return ideOk(
            summary = "Listed ${entries.size} run configuration type(s).",
            data = mapOf("objects" to entries.map { objectValue(it) }, "types" to entries.map { (it.value as ConfigurationType).toRunConfigTypeValue() }),
            nextBestActions = listOf("ide.run.config.create", "ide.object.describe"),
        )
    }

    override fun listRunConfigurations(input: AgentRunConfigListInput): Map<String, Any?> {
        val query = input.query.trim()
        val entries = runManager().allSettings.asSequence()
            .filter { input.includeTemporary || !it.isTemporary }
            .filter { settings ->
                query.isBlank() ||
                    settings.name.contains(query, ignoreCase = true) ||
                    settings.uniqueID.contains(query, ignoreCase = true) ||
                    settings.type.id.contains(query, ignoreCase = true) ||
                    settings.factory.id.contains(query, ignoreCase = true)
            }
            .take(input.limit)
            .map { objects.put("runConfig", it) }
            .toList()
        return ideOk(
            summary = "Listed ${entries.size} run configuration(s).",
            data = mapOf(
                "objects" to entries.map { objectValue(it) },
                "configurations" to entries.map { (it.value as RunnerAndConfigurationSettings).toRunConfigValue() },
                "selected" to runManager().selectedConfiguration?.let { it.uniqueID },
            ),
            nextBestActions = listOf("ide.run.config.types", "ide.run.config.create", "ide.object.describe"),
        )
    }

    override fun resolveRunConfiguration(input: AgentRunConfigResolveInput): Map<String, Any?> {
        val settings = resolveRunConfigSettings(input) ?: return ideError("run_config_not_found", "Run configuration not found.")
        val entry = objects.put("runConfig", settings)
        return ideOk(
            summary = "Resolved run configuration ${settings.name}.",
            data = mapOf("object" to objectValue(entry), "configuration" to settings.toRunConfigValue()),
            affordances = methodCatalog(entry),
        )
    }

    override fun createRunConfiguration(input: AgentRunConfigCreateInput): Map<String, Any?> {
        val name = input.name.trim().takeIf { it.isNotBlank() } ?: return ideError("invalid_arguments", "name is required.")
        val factory = configurationFactory(input.typeId, input.factoryId)
            ?: return ideError("run_config_factory_not_found", "No run configuration factory found for typeId=${input.typeId} factoryId=${input.factoryId}.")
        val manager = runManager()
        val settings = runOnEdt {
            val created = manager.createConfiguration(name, factory)
            created.isTemporary = input.temporary
            if (input.temporary) {
                manager.setTemporaryConfiguration(created)
            } else {
                manager.addConfiguration(created)
            }
            created
        }
        val patchResult = applyRunConfigPatch(settings, input.patch)
        val entry = objects.put("runConfig", settings)
        return ideOk(
            summary = "Created run configuration ${settings.name}.",
            data = mapOf(
                "object" to objectValue(entry),
                "configuration" to settings.toRunConfigValue(),
                "patch" to patchResult,
            ),
            affordances = methodCatalog(entry),
            nextBestActions = listOf("${entry.alias}.schema", "${entry.alias}.run", "${entry.alias}.debug"),
        )
    }

    override fun updateRunConfiguration(input: AgentRunConfigUpdateInput): Map<String, Any?> {
        val settings = resolveRunConfigSettings(AgentRunConfigResolveInput(input.name, input.configId, input.objectRef))
            ?: return ideError("run_config_not_found", "Run configuration not found.")
        val patchResult = applyRunConfigPatch(settings, input.patch)
        val entry = objects.put("runConfig", settings)
        return ideOk(
            summary = "Updated run configuration ${settings.name}.",
            data = mapOf("object" to objectValue(entry), "configuration" to settings.toRunConfigValue(), "patch" to patchResult),
            affordances = methodCatalog(entry),
        )
    }

    override fun deleteRunConfiguration(input: AgentRunConfigResolveInput): Map<String, Any?> {
        val settings = resolveRunConfigSettings(input) ?: return ideError("run_config_not_found", "Run configuration not found.")
        runOnEdt { runManager().removeConfiguration(settings) }
        return ideOk("Deleted run configuration ${settings.name}.", data = mapOf("configId" to settings.uniqueID, "name" to settings.name))
    }

    override fun listExecutions(): Map<String, Any?> {
        val entries = ExecutionManager.getInstance(project).getRunningProcesses().map { process ->
            trackProcessOutput(process)
            objects.put("execution", process)
        }
        return ideOk(
            summary = "Listed ${entries.size} running execution(s).",
            data = mapOf("objects" to entries.map { objectValue(it) }, "executions" to entries.map { (it.value as ProcessHandler).toExecutionValue() }),
        )
    }

    override fun callObject(input: AgentObjectCallInput): Map<String, Any?> {
        val entry = objects.get(input.objectRef) ?: return ideError("object_not_found", "No live IDE object named ${input.objectRef}.")
        return try {
            when (entry.type) {
                "Project" -> callProject(entry, input.method, input.arguments)
                "File" -> callFile(entry, input.method, input.arguments)
                "Directory" -> callDirectory(entry, input.method, input.arguments)
                "Document" -> callDocument(entry, input.method, input.arguments)
                "Editor" -> callEditor(entry, input.method, input.arguments)
                "Symbol" -> callSymbol(entry, input.method, input.arguments)
                "SearchResult" -> callSearchResult(entry, input.method, input.arguments)
                "Plugin" -> callPlugin(entry, input.method, input.arguments)
                "Breakpoint" -> callBreakpoint(entry, input.method, input.arguments)
                "RunConfigurationType" -> callRunConfigurationType(entry, input.method, input.arguments)
                "RunConfiguration" -> callRunConfiguration(entry, input.method, input.arguments)
                "Execution" -> callExecution(entry, input.method, input.arguments)
                else -> ideError("unsupported_method", "${entry.type} does not support ${input.method}.")
            }
        } catch (t: Throwable) {
            normalizeThrowable(t)
        }
    }

    private fun callProject(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> =
        when (method) {
            "observe" -> observe()
            "capabilities" -> capabilities()
            "openFile" -> openFile(AgentFileInput(args.obj().stringArg("filePath")))
            "runConfigurations" -> listRunConfigurations(
                AgentRunConfigListInput(
                    query = args.obj().stringArg("query"),
                    includeTemporary = args.obj().boolArg("includeTemporary", true),
                    limit = args.obj().intArg("limit", 200).coerceIn(1, 1000),
                ),
            )
            "runConfigurationTypes" -> listRunConfigurationTypes(
                AgentRunConfigTypesInput(
                    query = args.obj().stringArg("query"),
                    limit = args.obj().intArg("limit", 200).coerceIn(1, 1000),
                ),
            )
            "createRunConfiguration" -> createRunConfiguration(
                AgentRunConfigCreateInput(
                    name = args.obj().stringArg("name"),
                    typeId = args.obj().stringArg("typeId"),
                    factoryId = args.obj().stringArg("factoryId"),
                    temporary = args.obj().boolArg("temporary", false),
                    patch = args.obj().jsonArg("patch"),
                ),
            )
            "createFile" -> createFile(
                AgentFileCreateInput(
                    filePath = args.obj().stringArg("filePath"),
                    text = args.obj().stringArg("text"),
                    overwrite = args.obj().boolArg("overwrite"),
                    openEditor = args.obj().boolArg("openEditor"),
                ),
            )
            "search" -> searchText(AgentSearchInput(args.obj().stringArg("query"), "project", args.obj().intArg("limit", 100)))
            else -> unsupported(entry, method)
        }

    private fun callFile(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val file = entry.value as VirtualFile
        if (!file.isValid) return ideError("stale_object", "${entry.alias} is no longer valid.")
        return when (method) {
            "read" -> {
                val doc = documentFor(file) ?: return ideError("document_unavailable", "No document is available for ${file.path}.")
                val docEntry = objects.put("doc", doc)
                ideOk(
                    summary = "Read ${file.path}.",
                    data = mapOf(
                        "object" to objectValue(docEntry),
                        "text" to doc.text,
                        "lineCount" to doc.lineCount,
                        "psiQuality" to psiQuality(file),
                    ),
                    affordances = methodCatalog(docEntry),
                    nextBestActions = listOf("${docEntry.alias}.replace", "${docEntry.alias}.save"),
                )
            }
            "openEditor" -> openFile(AgentFileInput(file.path))
            "search" -> {
                val query = args.obj().stringArg("query")
                if (query.isBlank()) return ideError("invalid_query", "Search query must not be empty.")
                val result = searchInFile(file, query, args.obj().intArg("limit", 100))
                val resultEntry = objects.put("searchResult", result)
                ideOk(
                    summary = "Found ${result.items.size} occurrence(s) in ${file.name}.",
                    data = mapOf("object" to objectValue(resultEntry), "items" to result.items.map { it.toValue() }),
                    affordances = methodCatalog(resultEntry),
                )
            }
            "replace" -> replaceInFile(file, args.obj())
            "append" -> appendToFile(file, args.obj().stringArg("text"))
            "delete" -> deleteFile(file)
            "diagnostics" -> diagnosticsForFile(file)
            "symbolAt" -> symbolAt(file, args.obj().intArg("line", 1), args.obj().intArg("column", 1))
            "breakpoints" -> breakpointsForFile(file)
            "setBreakpoint" -> setBreakpoint(
                AgentBreakpointInput(
                    filePath = file.path,
                    line = args.obj().intArg("line", 1),
                    typeId = args.obj().stringArg("typeId"),
                    enabled = args.obj().boolArg("enabled", true),
                    temporary = args.obj().boolArg("temporary", false),
                ),
            )
            "removeBreakpoint" -> removeBreakpoint(
                AgentBreakpointInput(
                    filePath = file.path,
                    line = args.obj().intArg("line", 1),
                    typeId = args.obj().stringArg("typeId"),
                    enabled = true,
                    temporary = false,
                ),
            )
            "close" -> closeFile(file)
            else -> unsupported(entry, method)
        }
    }

    private fun callDirectory(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val dir = entry.value as VirtualFile
        if (!dir.isValid) return ideError("stale_object", "${entry.alias} is no longer valid.")
        return when (method) {
            "children" -> {
                val children = dir.children.map { objectValue(objects.put(if (it.isDirectory) "directory" else "file", it)) }
                ideOk("Listed ${dir.path}.", data = mapOf("children" to children))
            }
            "find" -> {
                val child = dir.findChild(args.obj().stringArg("name")) ?: return ideError("file_not_found", "Child not found.")
                val childEntry = objects.put(if (child.isDirectory) "directory" else "file", child)
                ideOk("Resolved ${child.path}.", data = mapOf("object" to objectValue(childEntry)), affordances = methodCatalog(childEntry))
            }
            "search" -> searchText(AgentSearchInput(args.obj().stringArg("query"), dir.path, args.obj().intArg("limit", 100)))
            "createFile" -> {
                val name = args.obj().stringArg("name").takeIf { it.isNotBlank() }
                    ?: return ideError("invalid_arguments", "name is required.")
                createFile(
                    AgentFileCreateInput(
                        filePath = Path.of(dir.path, name).toString(),
                        text = args.obj().stringArg("text"),
                        overwrite = args.obj().boolArg("overwrite"),
                        openEditor = args.obj().boolArg("openEditor"),
                    ),
                )
            }
            "createDirectory" -> {
                val name = args.obj().stringArg("name").takeIf { it.isNotBlank() }
                    ?: return ideError("invalid_arguments", "name is required.")
                createDirectory(Path.of(dir.path, name))
            }
            else -> unsupported(entry, method)
        }
    }

    private fun callDocument(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val doc = entry.value as Document
        return when (method) {
            "text" -> ideOk("Read ${entry.alias}.", data = mapOf("text" to doc.text, "lineCount" to doc.lineCount))
            "replace" -> replaceInDocument(doc, args.obj())
            "replaceOffsets" -> replaceOffsetsInDocument(doc, args.obj())
            "append" -> {
                runWrite { doc.insertString(doc.textLength, args.obj().stringArg("text")) }
                ideOk("Appended text.", data = mapOf("lineCount" to doc.lineCount))
            }
            "save" -> {
                runOnEdt { FileDocumentManager.getInstance().saveDocument(doc) }
                ideOk("Saved document.", data = mapOf("saved" to true))
            }
            "lineInfo" -> {
                val line = args.obj().intArg("line", 1).coerceIn(1, doc.lineCount.coerceAtLeast(1)) - 1
                ideOk(
                    "Read line ${line + 1}.",
                    data = mapOf(
                        "line" to line + 1,
                        "startOffset" to doc.getLineStartOffset(line),
                        "endOffset" to doc.getLineEndOffset(line),
                        "text" to doc.lineTextAt(doc.getLineStartOffset(line)),
                    ),
                )
            }
            else -> unsupported(entry, method)
        }
    }

    private fun callEditor(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val editor = entry.value as? Editor ?: return ideError("stale_object", "${entry.alias} is no longer an editor.")
        val file = PsiDocumentManager.getInstance(project).getPsiFile(editor.document)?.virtualFile
        return when (method) {
            "caret" -> ideOk("Read caret.", data = editorCaretValue(editor))
            "selection" -> ideOk("Read selection.", data = editorSelectionValue(editor))
            "select" -> {
                val start = editor.document.offsetAt(args.obj().intArg("startLine", 1), args.obj().intArg("startColumn", 1), args.obj().boolArg("clamp", false))
                    ?: return ideError("invalid_position", "Selection start is outside the document. Pass clamp:true to clamp to the nearest valid offset.")
                val end = editor.document.offsetAt(args.obj().intArg("endLine", 1), args.obj().intArg("endColumn", 1), args.obj().boolArg("clamp", false))
                    ?: return ideError("invalid_position", "Selection end is outside the document. Pass clamp:true to clamp to the nearest valid offset.")
                runOnEdt { editor.selectionModel.setSelection(start, end) }
                ideOk("Updated selection.", data = mapOf("startOffset" to start, "endOffset" to end))
            }
            "selectOffsets" -> {
                val start = args.obj().intArg("startOffset", 0)
                val end = args.obj().intArg("endOffset", start)
                if (start !in 0..editor.document.textLength || end !in 0..editor.document.textLength) {
                    return ideError("invalid_position", "Selection offsets must be within 0..${editor.document.textLength}.")
                }
                runOnEdt { editor.selectionModel.setSelection(start, end) }
                ideOk("Updated selection.", data = mapOf("startOffset" to start, "endOffset" to end))
            }
            "insert" -> {
                val offset = readAction { editor.caretModel.offset }
                runWrite { editor.document.insertString(offset, args.obj().stringArg("text")) }
                ideOk("Inserted text.", data = mapOf("offset" to offset))
            }
            "replaceSelection" -> {
                val (start, end) = readAction { editor.selectionModel.selectionStart to editor.selectionModel.selectionEnd }
                runWrite { editor.document.replaceString(start, end, args.obj().stringArg("text")) }
                ideOk("Replaced selection.", data = mapOf("startOffset" to start, "endOffset" to end))
            }
            "symbolAt" -> {
                if (file == null) return ideError("file_not_found", "Selected editor has no virtual file.")
                val position = readAction { editor.caretModel.logicalPosition }
                symbolAt(file, position.line + 1, position.column + 1)
            }
            "setBreakpoint" -> {
                if (file == null) return ideError("file_not_found", "Selected editor has no virtual file.")
                val line = readAction { editor.caretModel.logicalPosition.line + 1 }
                setBreakpoint(
                    AgentBreakpointInput(
                        filePath = file.path,
                        line = args.obj().intArg("line", line),
                        typeId = args.obj().stringArg("typeId"),
                        enabled = args.obj().boolArg("enabled", true),
                        temporary = args.obj().boolArg("temporary", false),
                    ),
                )
            }
            "removeBreakpoint" -> {
                if (file == null) return ideError("file_not_found", "Selected editor has no virtual file.")
                val line = readAction { editor.caretModel.logicalPosition.line + 1 }
                removeBreakpoint(
                    AgentBreakpointInput(
                        filePath = file.path,
                        line = args.obj().intArg("line", line),
                        typeId = args.obj().stringArg("typeId"),
                        enabled = true,
                        temporary = false,
                    ),
                )
            }
            "close" -> if (file != null) closeFile(file) else ideError("file_not_found", "Selected editor has no virtual file.")
            else -> unsupported(entry, method)
        }
    }

    private fun callSymbol(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val symbol = entry.value as PsiElement
        if (!symbol.isValid) return ideError("stale_object", "${entry.alias} is no longer valid.")
        return when (method) {
            "definition" -> ideOk("Returned symbol definition.", data = mapOf("symbol" to symbol.toAgentValue()))
            "references" -> readAction {
                val limit = args.obj().intArg("limit", 100).coerceIn(1, 1000)
                val refs = ReferencesSearch.search(symbol, GlobalSearchScope.projectScope(project)).asIterable().take(limit).map { it.toAgentReferenceValue() }
                ideOk("Found ${refs.size} reference(s).", data = mapOf("references" to refs, "limit" to limit, "count" to refs.size))
            }
            "renamePreview" -> renameSymbol(symbol, args.obj().stringArg("newName"), apply = false)
            "renameApply" -> renameSymbol(symbol, args.obj().stringArg("newName"), apply = true)
            else -> unsupported(entry, method)
        }
    }

    private fun callSearchResult(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val result = entry.value as AgentSearchResult
        return when (method) {
            "items" -> ideOk("Listed search result items.", data = mapOf("items" to result.items.map { it.toValue() }))
            "openItem" -> {
                val index = args.obj().intArg("index", 0)
                val item = result.items.getOrNull(index) ?: return ideError("invalid_position", "Search item index is out of range.")
                openFile(AgentFileInput(item.filePath))
            }
            "replaceAllPreview" -> replaceAllSearchResults(result, args.obj().stringArg("text"), apply = false)
            "replaceAllApply" -> replaceAllSearchResults(result, args.obj().stringArg("text"), apply = true)
            else -> unsupported(entry, method)
        }
    }

    private fun callPlugin(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val plugin = entry.value as IdeaPluginDescriptor
        return when (method) {
            "info" -> ideOk("Read plugin ${plugin.pluginId.idString}.", data = mapOf("plugin" to plugin.toPluginValue()))
            "enable" -> enablePlugin(AgentPluginInput(plugin.pluginId.idString))
            "disable" -> disablePlugin(AgentPluginInput(plugin.pluginId.idString))
            "load" -> loadPlugin(AgentPluginInput(plugin.pluginId.idString))
            "unload" -> unloadPlugin(AgentPluginInput(plugin.pluginId.idString))
            "uninstall" -> uninstallPlugin(AgentPluginInput(plugin.pluginId.idString))
            "dependencies" -> ideOk(
                "Listed plugin dependencies.",
                data = mapOf(
                    "dependencies" to plugin.dependencies.map {
                        mapOf(
                            "pluginId" to it.pluginId.idString,
                            "optional" to it.isOptional,
                        )
                    },
                ),
            )
            else -> unsupported(entry, method)
        }
    }

    private fun callBreakpoint(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val breakpoint = entry.value as XBreakpoint<*>
        return when (method) {
            "info" -> ideOk("Read breakpoint.", data = mapOf("breakpoint" to breakpoint.toBreakpointValue()))
            "enable" -> {
                breakpoint.isEnabled = true
                ideOk("Enabled breakpoint.", data = mapOf("breakpoint" to breakpoint.toBreakpointValue()))
            }
            "disable" -> {
                breakpoint.isEnabled = false
                ideOk("Disabled breakpoint.", data = mapOf("breakpoint" to breakpoint.toBreakpointValue()))
            }
            "remove" -> {
                runOnEdt { breakpointManager().removeBreakpoint(breakpoint) }
                ideOk("Removed breakpoint.", data = mapOf("removed" to true))
            }
            else -> unsupported(entry, method)
        }
    }

    private fun callRunConfigurationType(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val type = entry.value as ConfigurationType
        return when (method) {
            "info" -> ideOk("Read run configuration type ${type.id}.", data = mapOf("type" to type.toRunConfigTypeValue()))
            "factories" -> ideOk("Listed factories for ${type.id}.", data = mapOf("factories" to type.configurationFactories.map { it.toFactoryValue() }))
            "template" -> {
                val factory = type.configurationFactories.firstOrNull {
                    args.obj().stringArg("factoryId").isBlank() || it.id == args.obj().stringArg("factoryId")
                } ?: return ideError("run_config_factory_not_found", "No factory found for ${type.id}.")
                val template = runManager().getConfigurationTemplate(factory)
                val templateEntry = objects.put("runConfig", template)
                ideOk(
                    "Resolved template for ${type.id}/${factory.id}.",
                    data = mapOf("object" to objectValue(templateEntry), "configuration" to template.toRunConfigValue(), "schema" to template.toRunConfigSchema()),
                    affordances = methodCatalog(templateEntry),
                )
            }
            else -> unsupported(entry, method)
        }
    }

    private fun callRunConfiguration(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val settings = entry.value as RunnerAndConfigurationSettings
        return when (method) {
            "info" -> ideOk("Read run configuration ${settings.name}.", data = mapOf("configuration" to settings.toRunConfigValue()))
            "schema" -> ideOk("Read run configuration schema ${settings.name}.", data = mapOf("schema" to settings.toRunConfigSchema()))
            "update" -> {
                val patchResult = applyRunConfigPatch(settings, args.obj().jsonArg("patch") ?: args.obj())
                ideOk("Updated run configuration ${settings.name}.", data = mapOf("configuration" to settings.toRunConfigValue(), "patch" to patchResult))
            }
            "select" -> {
                runOnEdt { runManager().selectedConfiguration = settings }
                ideOk("Selected run configuration ${settings.name}.", data = mapOf("configuration" to settings.toRunConfigValue()))
            }
            "run" -> executeRunConfiguration(settings, "run")
            "debug" -> executeRunConfiguration(settings, "debug")
            "clone" -> cloneRunConfiguration(settings, args.obj().stringArg("name"))
            "delete" -> deleteRunConfiguration(AgentRunConfigResolveInput(name = "", configId = settings.uniqueID, objectRef = entry.alias))
            else -> unsupported(entry, method)
        }
    }

    private fun callExecution(entry: AgentObjectEntry, method: String, args: JsonElement?): Map<String, Any?> {
        val execution = entry.value
        return when (method) {
            "status" -> ideOk("Read execution status.", data = mapOf("execution" to execution.toAnyExecutionValue()))
            "console" -> {
                if (execution !is ProcessHandler) {
                    return ideUnavailable("console_unavailable", "This execution is a scheduled run request. Call ide.run.executions after the IDE starts the process.")
                }
                trackProcessOutput(execution)
                val text = synchronized(executionBuffers.getValue(execution)) { executionBuffers.getValue(execution).toString() }
                val offset = args.obj().intArg("offset", 0).coerceIn(0, text.length)
                val limit = args.obj().intArg("limit", 12000).coerceIn(1, 200000)
                val chunk = text.substring(offset, (offset + limit).coerceAtMost(text.length))
                ideOk(
                    "Read execution console buffer.",
                    data = mapOf(
                        "offset" to offset,
                        "nextOffset" to (offset + chunk.length),
                        "totalBuffered" to text.length,
                        "truncated" to (offset + chunk.length < text.length),
                        "text" to chunk,
                    ),
                )
            }
            "stop" -> {
                if (execution is ProcessHandler && !execution.isProcessTerminated && !execution.isProcessTerminating) execution.destroyProcess()
                ideOk("Stop requested for execution.", data = mapOf("execution" to execution.toAnyExecutionValue()))
            }
            else -> unsupported(entry, method)
        }
    }

    private fun breakpointsForFile(file: VirtualFile): Map<String, Any?> {
        val entries = breakpointManager().allBreakpoints
            .filterIsInstance<XLineBreakpoint<*>>()
            .filter { it.fileUrl == file.url }
            .map { objects.put("breakpoint", it) }
        return ideOk(
            "Listed ${entries.size} breakpoint(s) for ${file.path}.",
            data = mapOf("objects" to entries.map { objectValue(it) }, "breakpoints" to entries.map { (it.value as XBreakpoint<*>).toBreakpointValue() }),
        )
    }

    private fun breakpointManager() = XDebuggerManager.getInstance(project).breakpointManager

    @Suppress("UNCHECKED_CAST")
    private fun lineBreakpointTypes(): List<XLineBreakpointType<XBreakpointProperties<*>>> =
        XBreakpointType.EXTENSION_POINT_NAME.extensionList.filterIsInstance<XLineBreakpointType<XBreakpointProperties<*>>>()

    private fun lineBreakpointTypeFor(file: VirtualFile, lineZeroBased: Int, typeId: String): XLineBreakpointType<XBreakpointProperties<*>>? = readAction {
        lineBreakpointTypes()
            .filter { typeId.isBlank() || it.id == typeId }
            .firstOrNull { it.canPutAt(file, lineZeroBased, project) }
    }

    private fun XBreakpoint<*>.toBreakpointValue(): Map<String, Any?> {
        val line = this as? XLineBreakpoint<*>
        return mapOf(
            "typeId" to type.id,
            "typeTitle" to type.title,
            "enabled" to isEnabled,
            "suspendPolicy" to suspendPolicy.name,
            "fileUrl" to line?.fileUrl,
            "filePath" to line?.presentableFilePath,
            "shortFilePath" to line?.shortFilePath,
            "line" to line?.line?.plus(1),
            "temporary" to line?.isTemporary,
            "condition" to conditionExpression?.expression,
            "logMessage" to isLogMessage,
            "logStack" to isLogStack,
            "timestamp" to timeStamp,
        )
    }

    private fun runManager(): RunManager = RunManager.getInstance(project)

    private fun configurationTypes(): List<ConfigurationType> =
        ConfigurationType.CONFIGURATION_TYPE_EP.extensionList.sortedBy { it.displayName }

    private fun configurationFactory(typeId: String, factoryId: String): ConfigurationFactory? =
        configurationTypes()
            .firstOrNull { it.id == typeId || it.displayName.equals(typeId, ignoreCase = true) }
            ?.configurationFactories
            ?.firstOrNull { factoryId.isBlank() || it.id == factoryId || it.name.equals(factoryId, ignoreCase = true) }

    private fun resolveRunConfigSettings(input: AgentRunConfigResolveInput): RunnerAndConfigurationSettings? {
        if (input.objectRef.isNotBlank()) {
            val entry = objects.get(input.objectRef)
            val value = entry?.value
            if (value is RunnerAndConfigurationSettings) return value
        }
        val manager = runManager()
        if (input.configId.isNotBlank()) {
            manager.allSettings.firstOrNull { it.uniqueID == input.configId }?.let { return it }
        }
        if (input.name.isNotBlank()) {
            manager.findConfigurationByName(input.name)?.let { return it }
            manager.allSettings.firstOrNull { it.name == input.name }?.let { return it }
        }
        return null
    }

    private fun cloneRunConfiguration(settings: RunnerAndConfigurationSettings, requestedName: String): Map<String, Any?> {
        val factory = settings.factory
        val name = requestedName.takeIf { it.isNotBlank() } ?: runManager().suggestUniqueName("${settings.name} Copy", settings.type)
        val clone = settings.configuration.clone()
        clone.name = name
        val copied = runOnEdt {
            val created = runManager().createConfiguration(clone, factory)
            runManager().addConfiguration(created)
            created
        }
        val entry = objects.put("runConfig", copied)
        return ideOk(
            "Cloned run configuration ${settings.name} to ${copied.name}.",
            data = mapOf("object" to objectValue(entry), "configuration" to copied.toRunConfigValue()),
            affordances = methodCatalog(entry),
        )
    }

    private fun executeRunConfiguration(settings: RunnerAndConfigurationSettings, mode: String): Map<String, Any?> {
        val executor = executorFor(mode)
            ?: return ideError("executor_not_found", "No IntelliJ executor found for $mode.")
        val checkError = runCatching { settings.checkSettings(executor) }.exceptionOrNull()
        if (checkError != null) {
            return ideError("run_config_invalid", checkError.message ?: checkError.javaClass.name)
        }
        runOnEdt {
            runManager().selectedConfiguration = settings
            ProgramRunnerUtil.executeConfiguration(project, settings, executor)
        }
        val execution = AgentScheduledExecution(settings.uniqueID, settings.name, mode, System.currentTimeMillis())
        val entry = objects.put("execution", execution)
        return ideOk(
            summary = "Scheduled ${mode} for ${settings.name}.",
            data = mapOf(
                "object" to objectValue(entry),
                "configuration" to settings.toRunConfigValue(),
                "execution" to execution.toValue(),
                "scheduledOnly" to true,
            ),
            affordances = methodCatalog(entry),
            nextBestActions = listOf("ide.run.executions", "${entry.alias}.status"),
        )
    }

    private fun executorFor(mode: String): Executor? =
        when (mode) {
            "debug" -> ExecutorRegistry.getInstance().getExecutorById(DefaultDebugExecutor.EXECUTOR_ID)
                ?: DefaultDebugExecutor.getDebugExecutorInstance()
            else -> ExecutorRegistry.getInstance().getExecutorById(DefaultRunExecutor.EXECUTOR_ID)
                ?: DefaultRunExecutor.getRunExecutorInstance()
        }

    private fun trackProcessOutput(process: ProcessHandler) {
        if (executionBuffers.containsKey(process)) return
        val buffer = StringBuilder()
        val existing = executionBuffers.putIfAbsent(process, buffer)
        if (existing != null) return
        process.addProcessListener(object : ProcessAdapter() {
            override fun onTextAvailable(event: ProcessEvent, outputType: Key<*>) {
                synchronized(buffer) {
                    buffer.append(event.text)
                    val overflow = buffer.length - 500_000
                    if (overflow > 0) buffer.delete(0, overflow)
                }
            }
        })
    }

    private fun applyRunConfigPatch(settings: RunnerAndConfigurationSettings, patch: JsonElement?): Map<String, Any?> {
        val obj = patch?.takeIf { it.isJsonObject }?.asJsonObject ?: return mapOf("applied" to emptyList<String>(), "rejected" to emptyList<Any>())
        val applied = mutableListOf<String>()
        val rejected = mutableListOf<Map<String, String>>()
        obj.entrySet().forEach { (key, value) ->
            val result = runCatching {
                when (key) {
                    "name" -> {
                        settings.name = value.asString
                        true
                    }
                    "temporary" -> {
                        settings.isTemporary = value.asBoolean
                        true
                    }
                    "editBeforeRun" -> {
                        settings.isEditBeforeRun = value.asBoolean
                        true
                    }
                    "activateToolWindowBeforeRun" -> {
                        settings.isActivateToolWindowBeforeRun = value.asBoolean
                        true
                    }
                    "focusToolWindowBeforeRun" -> {
                        settings.isFocusToolWindowBeforeRun = value.asBoolean
                        true
                    }
                    "folderName" -> {
                        settings.folderName = value.takeUnless { it.isJsonNull }?.asString
                        true
                    }
                    "singleton" -> {
                        settings.isSingleton = value.asBoolean
                        true
                    }
                    "allowRunningInParallel" -> {
                        settings.configuration.isAllowRunningInParallel = value.asBoolean
                        true
                    }
                    "store" -> {
                        when (value.asString) {
                            "localWorkspace" -> settings.storeInLocalWorkspace()
                            "dotIdea" -> settings.storeInDotIdeaFolder()
                            else -> return@runCatching false
                        }
                        true
                    }
                    "storePath" -> {
                        settings.storeInArbitraryFileInProject(value.asString)
                        true
                    }
                    else -> applyPublicSetter(settings.configuration, key, value)
                }
            }
            if (result.getOrDefault(false)) {
                applied += key
            } else {
                rejected += mapOf("field" to key, "reason" to (result.exceptionOrNull()?.message ?: "No compatible public setter or supported settings field."))
            }
        }
        runCatching { settings.checkSettings() }
        return mapOf("applied" to applied, "rejected" to rejected)
    }

    private fun applyPublicSetter(target: Any, property: String, value: JsonElement): Boolean {
        val setter = "set" + property.replaceFirstChar { it.uppercase() }
        val method = target.javaClass.methods.firstOrNull {
            it.name == setter &&
                it.parameterCount == 1 &&
                Modifier.isPublic(it.modifiers) &&
                jsonCanCoerce(value, it.parameterTypes[0])
        } ?: return false
        method.invoke(target, coerceJson(value, method.parameterTypes[0]))
        return true
    }

    private fun jsonCanCoerce(value: JsonElement, type: Class<*>): Boolean =
        value.isJsonNull ||
            type == String::class.java ||
            type == java.lang.Boolean.TYPE ||
            type == java.lang.Boolean::class.java ||
            type == java.lang.Integer.TYPE ||
            type == java.lang.Integer::class.java ||
            type == java.lang.Long.TYPE ||
            type == java.lang.Long::class.java ||
            type == java.lang.Double.TYPE ||
            type == java.lang.Double::class.java ||
            type == java.lang.Float.TYPE ||
            type == java.lang.Float::class.java

    private fun coerceJson(value: JsonElement, type: Class<*>): Any? {
        if (value.isJsonNull) return null
        return when (type) {
            String::class.java -> value.asString
            java.lang.Boolean.TYPE, java.lang.Boolean::class.java -> value.asBoolean
            java.lang.Integer.TYPE, java.lang.Integer::class.java -> value.asInt
            java.lang.Long.TYPE, java.lang.Long::class.java -> value.asLong
            java.lang.Double.TYPE, java.lang.Double::class.java -> value.asDouble
            java.lang.Float.TYPE, java.lang.Float::class.java -> value.asFloat
            else -> error("Unsupported patch type: ${type.name}")
        }
    }

    private fun RunnerAndConfigurationSettings.toRunConfigValue(): Map<String, Any?> = mapOf(
        "name" to name,
        "configId" to uniqueID,
        "typeId" to type.id,
        "typeName" to type.displayName,
        "factoryId" to factory.id,
        "factoryName" to factory.name,
        "configurationClass" to configuration.javaClass.name,
        "temporary" to isTemporary,
        "template" to isTemplate,
        "selected" to (runManager().selectedConfiguration?.uniqueID == uniqueID),
        "storedInLocalWorkspace" to isStoredInLocalWorkspace,
        "storedInDotIdeaFolder" to isStoredInDotIdeaFolder,
        "storedInArbitraryFileInProject" to isStoredInArbitraryFileInProject,
        "pathIfStoredInArbitraryFileInProject" to pathIfStoredInArbitraryFileInProject,
        "editBeforeRun" to isEditBeforeRun,
        "activateToolWindowBeforeRun" to isActivateToolWindowBeforeRun,
        "focusToolWindowBeforeRun" to isFocusToolWindowBeforeRun,
        "folderName" to folderName,
        "singleton" to runCatching { isSingleton }.getOrNull(),
        "allowRunningInParallel" to runCatching { configuration.isAllowRunningInParallel }.getOrNull(),
        "valid" to (runCatching { checkSettings(); true }.getOrDefault(false)),
    )

    private fun RunnerAndConfigurationSettings.toRunConfigSchema(): Map<String, Any?> {
        val setters = configuration.javaClass.methods
            .filter { it.name.startsWith("set") && it.parameterCount == 1 && Modifier.isPublic(it.modifiers) && jsonCanCoerce(com.google.gson.JsonPrimitive(""), it.parameterTypes[0]) }
            .map {
                mapOf(
                    "field" to it.name.removePrefix("set").replaceFirstChar { ch -> ch.lowercase() },
                    "type" to it.parameterTypes[0].name,
                    "method" to it.name,
                )
            }
            .distinctBy { it["field"] }
            .sortedBy { it["field"] as String }
        return mapOf(
            "configurationClass" to configuration.javaClass.name,
            "settingsFields" to listOf(
                "name",
                "temporary",
                "editBeforeRun",
                "activateToolWindowBeforeRun",
                "focusToolWindowBeforeRun",
                "folderName",
                "singleton",
                "allowRunningInParallel",
                "store",
                "storePath",
            ),
            "configurationSetters" to setters,
            "notes" to "Only primitive, boolean, number, and string public setters are patched generically. Unsupported product-specific structures are rejected with reasons.",
        )
    }

    private fun ConfigurationType.toRunConfigTypeValue(): Map<String, Any?> = mapOf(
        "typeId" to id,
        "displayName" to displayName,
        "description" to configurationTypeDescription,
        "managed" to isManaged,
        "factories" to configurationFactories.map { it.toFactoryValue() },
    )

    private fun ConfigurationFactory.toFactoryValue(): Map<String, Any?> = mapOf(
        "factoryId" to id,
        "name" to name,
        "typeId" to type.id,
        "applicable" to runCatching { isApplicable(project) }.getOrDefault(true),
        "singletonByDefault" to isConfigurationSingletonByDefault,
        "canBeSingleton" to canConfigurationBeSingleton(),
        "optionsClass" to optionsClass?.name,
    )

    private fun ProcessHandler.toExecutionValue(): Map<String, Any?> = mapOf(
        "executionClass" to javaClass.name,
        "terminated" to isProcessTerminated,
        "terminating" to isProcessTerminating,
        "startNotified" to isStartNotified,
        "exitCode" to exitCode,
        "detachIsDefault" to detachIsDefault(),
    )

    private fun Any.toAnyExecutionValue(): Map<String, Any?> =
        when (this) {
            is ProcessHandler -> toExecutionValue()
            is AgentScheduledExecution -> toValue()
            else -> mapOf("executionClass" to javaClass.name, "display" to toString())
        }

    private fun AgentScheduledExecution.toValue(): Map<String, Any?> = mapOf(
        "configId" to configId,
        "configurationName" to configurationName,
        "mode" to mode,
        "scheduledAtMillis" to scheduledAtMillis,
        "scheduledOnly" to true,
    )

    private fun mutatePlugin(pluginId: String, pastTense: String, body: (IdeaPluginDescriptor) -> Map<String, Any?>): Map<String, Any?> {
        val descriptor = pluginDescriptor(pluginId) ?: return ideError("plugin_not_found", "Plugin not found: $pluginId")
        return runCatching { body(descriptor) }.getOrElse { error ->
            ideError("plugin_operation_failed", "$pastTense operation failed for $pluginId: ${error.message ?: error.javaClass.name}")
        }
    }

    private fun pluginDescriptor(pluginId: String): IdeaPluginDescriptor? =
        PluginManagerCore.findPlugin(PluginId.getId(pluginId))
            ?: PluginManagerCore.plugins.firstOrNull { it.pluginId.idString == pluginId }

    private fun loadPluginDescriptorFromArtifact(path: Path): IdeaPluginDescriptorImpl? {
        val loader = Class.forName("com.intellij.ide.plugins.PluginDescriptorLoader")
        val method = loader.getMethod(
            "loadDescriptorFromArtifact",
            Path::class.java,
            com.intellij.openapi.util.BuildNumber::class.java,
        )
        return method.invoke(null, path, PluginManagerCore.buildNumber) as? IdeaPluginDescriptorImpl
    }

    private fun pluginMutationData(descriptor: IdeaPluginDescriptor, changed: Boolean): Map<String, Any?> =
        mapOf(
            "changed" to changed,
            "plugin" to descriptor.toPluginValue(),
            "restartRequired" to restartRequired(),
        )

    private fun selfLifecycleData(
        descriptor: IdeaPluginDescriptor,
        requestedOperation: String,
        actualOperation: String = requestedOperation,
    ): Map<String, Any?> =
        if (!isSelfPlugin(descriptor)) {
            emptyMap()
        } else {
            mapOf(
                "selfMutation" to true,
                "staged" to true,
                "dynamic" to false,
                "requestedOperation" to requestedOperation,
                "actualOperation" to actualOperation,
                "restartRequired" to true,
                "agentMustReconnectAfterRestart" to true,
                "serverMayDisconnect" to false,
            )
        }

    private fun stageSelfUpdate(path: Path, descriptor: IdeaPluginDescriptor): Map<String, Any?> {
        val current = pluginDescriptor(SELF_PLUGIN_ID)
        runOnEdt {
            PluginInstaller.installAfterRestart(
                descriptor,
                path,
                current?.pluginPath,
                false,
            )
        }
        markRestartRequired()
        val entry = objects.put("plugin", descriptor)
        return ideOk(
            summary = "Staged self-update for $SELF_PLUGIN_ID from $path. Restart IntelliJ to apply it.",
            data = selfLifecycleData(descriptor, requestedOperation = "self.update", actualOperation = "installAfterRestart") + mapOf(
                "selfUpdate" to true,
                "staged" to true,
                "dynamic" to false,
                "restartRequired" to true,
                "object" to objectValue(entry),
                "plugin" to descriptor.toPluginValue(),
                "sourcePath" to path.toString(),
                "existingPluginPath" to current?.pluginPath?.toString(),
            ),
            affordances = methodCatalog(entry),
            nextBestActions = selfRestartActions(descriptor),
        )
    }

    private fun restartRequired(): Boolean =
        InstalledPluginsState.getInstanceIfLoaded()?.isRestartRequired ?: false

    private fun markRestartRequired() {
        runOnEdt { InstalledPluginsState.getInstance().isRestartRequired = true }
    }

    private fun selfRestartActions(descriptor: IdeaPluginDescriptor): List<String> =
        if (isSelfPlugin(descriptor)) listOf("Restart IntelliJ IDEA", "Reconnect MCP client") else emptyList()

    private fun isSelfPlugin(descriptor: IdeaPluginDescriptor): Boolean =
        descriptor.pluginId.idString == SELF_PLUGIN_ID

    private fun IdeaPluginDescriptor.toPluginValue(): Map<String, Any?> {
        val impl = this as? IdeaPluginDescriptorImpl
        return mapOf(
            "pluginId" to pluginId.idString,
            "name" to name,
            "version" to version,
            "vendor" to vendor,
            "category" to category,
            "enabled" to isEnabled,
            "disabled" to PluginEnabler.getInstance().isDisabled(pluginId),
            "loaded" to PluginManagerCore.loadedPlugins.any { it.pluginId == pluginId },
            "bundled" to isBundled,
            "path" to pluginPath?.toString(),
            "sinceBuild" to sinceBuild,
            "untilBuild" to untilBuild,
            "requiresRestart" to isRequireRestart,
            "dynamicUnloadProblem" to impl?.let { runCatching { DynamicPlugins.checkCanUnloadWithoutRestart(it) }.getOrNull() },
            "dependencies" to dependencies.map { mapOf("pluginId" to it.pluginId.idString, "optional" to it.isOptional) },
        )
    }

    private fun replaceInFile(file: VirtualFile, args: JsonObject): Map<String, Any?> {
        val doc = documentFor(file) ?: return ideError("document_unavailable", "No document is available for ${file.path}.")
        return replaceInDocument(doc, args)
    }

    private fun replaceInDocument(doc: Document, args: JsonObject): Map<String, Any?> {
        val clamp = args.boolArg("clamp", false)
        val start = doc.offsetAt(args.intArg("startLine", 1), args.intArg("startColumn", 1), clamp)
            ?: return ideError("invalid_position", "Replacement start is outside the document. Pass clamp:true to clamp to the nearest valid offset.")
        val end = doc.offsetAt(args.intArg("endLine", 1), args.intArg("endColumn", 1), clamp)
            ?: return ideError("invalid_position", "Replacement end is outside the document. Pass clamp:true to clamp to the nearest valid offset.")
        if (end < start) return ideError("invalid_position", "Replacement range end is before start.")
        val current = doc.charsSequence.subSequence(start, end).toString()
        validateReplacementExpectedText(args, current)?.let { return it }
        runWrite { doc.replaceString(start, end, args.stringArg("text")) }
        return ideOk("Replaced text.", data = mapOf("startOffset" to start, "endOffset" to end, "lineCount" to doc.lineCount, "replacedLength" to current.length))
    }

    private fun replaceOffsetsInDocument(doc: Document, args: JsonObject): Map<String, Any?> {
        val start = args.intArg("startOffset", 0)
        val end = args.intArg("endOffset", start)
        if (start !in 0..doc.textLength || end !in 0..doc.textLength) {
            return ideError("invalid_position", "Replacement offsets must be within 0..${doc.textLength}.")
        }
        if (end < start) return ideError("invalid_position", "Replacement range end is before start.")
        val current = doc.charsSequence.subSequence(start, end).toString()
        validateReplacementExpectedText(args, current)?.let { return it }
        runWrite { doc.replaceString(start, end, args.stringArg("text")) }
        return ideOk("Replaced text.", data = mapOf("startOffset" to start, "endOffset" to end, "lineCount" to doc.lineCount, "replacedLength" to current.length))
    }

    private fun appendToFile(file: VirtualFile, text: String): Map<String, Any?> {
        val doc = documentFor(file) ?: return ideError("document_unavailable", "No document is available for ${file.path}.")
        runWrite { doc.insertString(doc.textLength, text) }
        return ideOk("Appended text to ${file.path}.", data = mapOf("lineCount" to doc.lineCount))
    }

    private fun deleteFile(file: VirtualFile): Map<String, Any?> {
        val path = file.path
        runWrite { file.delete(this) }
        return ideOk("Deleted $path.", data = mapOf("path" to path), nextBestActions = listOf("ide.object.release"))
    }

    private fun createDirectory(path: Path): Map<String, Any?> {
        val existing = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(path)
        if (existing != null && !existing.isDirectory) return ideError("invalid_path", "Path is a file: $path")
        runOnEdt {
            WriteCommandAction.runWriteCommandAction(project) {
                Files.createDirectories(path)
                LocalFileSystem.getInstance().refreshAndFindFileByNioFile(path)?.refresh(false, true)
            }
        }
        val dir = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(path)
            ?: return ideError("file_not_found", "Directory was not resolved after creation: $path")
        val entry = objects.put("directory", dir)
        return ideOk(
            "Created ${dir.path}.",
            data = mapOf("object" to objectValue(entry), "path" to dir.path, "existed" to (existing != null)),
            affordances = methodCatalog(entry),
        )
    }

    private fun closeFile(file: VirtualFile): Map<String, Any?> {
        runOnEdt { FileEditorManager.getInstance(project).closeFile(file) }
        return ideOk("Closed ${file.path}.", data = mapOf("path" to file.path))
    }

    private fun diagnosticsForFile(file: VirtualFile): Map<String, Any?> = readAction {
        val psi = PsiManager.getInstance(project).findFile(file)
            ?: return@readAction ideError("native_psi_unavailable", "No PSI file is available for ${file.path}.")
        val doc = psi.viewProvider.document
            ?: return@readAction ideError("document_unavailable", "No document is available for ${file.path}.")
        val analyzer = com.intellij.codeInsight.daemon.DaemonCodeAnalyzer.getInstance(project)
        if (!analyzer.isHighlightingAvailable(psi)) {
            return@readAction ideUnavailable("diagnostics_unavailable", "IDE highlighting is not available for ${file.path}.")
        }
        val highlights = listOf(
            DaemonCodeAnalyzerImpl.getHighlights(doc, com.intellij.lang.annotation.HighlightSeverity.ERROR, project),
            DaemonCodeAnalyzerImpl.getHighlights(doc, com.intellij.lang.annotation.HighlightSeverity.WARNING, project),
            DaemonCodeAnalyzerImpl.getHighlights(doc, com.intellij.lang.annotation.HighlightSeverity.WEAK_WARNING, project),
            DaemonCodeAnalyzerImpl.getHighlights(doc, com.intellij.lang.annotation.HighlightSeverity.INFORMATION, project),
        ).flatten().distinctBy { "${it.startOffset}:${it.endOffset}:${it.severity}:${it.description}" }
            .sortedWith(compareBy<HighlightInfo> { it.startOffset }.thenBy { it.endOffset })
        val diagnostics = highlights.map { it.toDiagnosticValue(doc, file) }
        ideOk(
            "Read ${diagnostics.size} diagnostic(s) for ${file.path}.",
            data = mapOf(
                "filePath" to file.path,
                "diagnostics" to diagnostics,
                "count" to diagnostics.size,
                "analysisFinished" to ((analyzer as? DaemonCodeAnalyzerImpl)?.isAllAnalysisFinished(psi)),
            ),
        )
    }

    private fun symbolAt(file: VirtualFile, line: Int, column: Int): Map<String, Any?> = readAction {
        val psi = PsiManager.getInstance(project).findFile(file) ?: return@readAction ideError("native_psi_unavailable", "No PSI file is available for ${file.path}.")
        val quality = psiQuality(file)
        if (quality["semantic"] != true) {
            return@readAction ideUnavailable(
                "native_psi_unavailable",
                "This file is handled as ${quality["language"] ?: "unknown"}, so semantic PSI operations are unavailable.",
                nextBestActions = listOf("${objects.findAlias(file) ?: "file"}.read", "${objects.findAlias(file) ?: "file"}.search"),
            )
        }
        val doc = psi.viewProvider.document ?: return@readAction ideError("document_unavailable", "No document is available for ${file.path}.")
        val offset = doc.offsetAt(line, column, clamp = false)
            ?: return@readAction ideError("invalid_position", "Position $line:$column is outside ${file.path}.")
        val element = psi.findElementAt(offset) ?: return@readAction ideError("no_reference_at_position", "No PSI element at $line:$column.")
        val symbol = generateSequence(element) { it.parent }.firstOrNull { it is PsiNamedElement } ?: element
        val symbolEntry = objects.put("symbol", symbol)
        ideOk(
            summary = "Resolved symbol at $line:$column.",
            data = mapOf("object" to objectValue(symbolEntry), "symbol" to symbol.toAgentValue()),
            affordances = methodCatalog(symbolEntry),
        )
    }

    private fun renameSymbol(symbol: PsiElement, newName: String, apply: Boolean): Map<String, Any?> {
        if (newName.isBlank()) return ideError("invalid_arguments", "newName is required.")
        val target = (symbol as? PsiNamedElement)
            ?: generateSequence(symbol) { it.parent }.filterIsInstance<PsiNamedElement>().firstOrNull()
            ?: return ideError("unsupported_refactoring", "Selected PSI element is not renameable.")
        val processor = RenameProcessor(project, target, newName, true, true)
        val usages = readAction { processor.findUsages().filter { it.isValid } }
        val preview = usages.map { it.toUsageValue() }
        if (!apply) {
            return ideOk(
                "Previewed rename to $newName with ${preview.size} usage(s).",
                data = mapOf("newName" to newName, "target" to target.toAgentValue(), "usageCount" to preview.size, "usages" to preview.take(1000)),
            )
        }
        runOnEdt {
            WriteCommandAction.runWriteCommandAction(project) {
                processor.performRefactoring(usages.toTypedArray())
            }
        }
        return ideOk(
            "Applied rename to $newName with ${preview.size} usage(s).",
            data = mapOf("newName" to newName, "target" to target.toAgentValue(), "usageCount" to preview.size, "usages" to preview.take(1000)),
        )
    }

    private fun textSearch(query: String, limit: Int): AgentSearchResult = readAction {
        val items = mutableListOf<AgentSearchItem>()
        PsiSearchHelper.getInstance(project).processElementsWithWord(
            TextOccurenceProcessor { element, offsetInElement ->
                if (items.size >= limit.coerceIn(1, 1000)) return@TextOccurenceProcessor false
                val file = element.containingFile?.virtualFile ?: return@TextOccurenceProcessor true
                val doc = element.containingFile?.viewProvider?.document ?: return@TextOccurenceProcessor true
                val start = (element.textRange?.startOffset ?: 0) + offsetInElement
                val line = doc.getLineNumber(start)
                items += AgentSearchItem(
                    filePath = file.path,
                    projectRelativePath = project.relativePath(file),
                    line = line + 1,
                    column = start - doc.getLineStartOffset(line) + 1,
                    text = doc.lineTextAt(start).take(500),
                )
                true
            },
            GlobalSearchScope.projectScope(project),
            query,
            UsageSearchContext.ANY,
            true,
        )
        AgentSearchResult(query, items)
    }

    private fun searchInFile(file: VirtualFile, query: String, limit: Int): AgentSearchResult {
        val doc = documentFor(file) ?: return AgentSearchResult(query, emptyList())
        val items = mutableListOf<AgentSearchItem>()
        var from = 0
        while (items.size < limit.coerceIn(1, 1000)) {
            val index = doc.text.indexOf(query, from)
            if (index < 0) break
            val line = doc.getLineNumber(index)
            items += AgentSearchItem(file.path, project.relativePath(file), line + 1, index - doc.getLineStartOffset(line) + 1, doc.lineTextAt(index).take(500))
            from = index + query.length.coerceAtLeast(1)
        }
        return AgentSearchResult(query, items)
    }

    private fun replaceAllSearchResults(result: AgentSearchResult, replacement: String, apply: Boolean): Map<String, Any?> {
        if (result.query.isEmpty()) return ideError("invalid_query", "Search query must not be empty.")
        val grouped = result.items.groupBy { it.filePath }
        val edits = grouped.flatMap { (path, items) ->
            val file = resolveVirtualFile(path) ?: return@flatMap emptyList()
            val doc = documentFor(file) ?: return@flatMap emptyList()
            items.mapNotNull { item ->
                val start = doc.offsetAt(item.line, item.column, clamp = false) ?: return@mapNotNull null
                val end = (start + result.query.length).coerceAtMost(doc.textLength)
                if (doc.text.substring(start, end) != result.query) return@mapNotNull null
                AgentTextEdit(path, project.relativePath(file), start, end, item.line, item.column, result.query, replacement)
            }
        }.sortedWith(compareBy<AgentTextEdit> { it.filePath }.thenBy { it.startOffset })
        if (!apply) {
            return ideOk(
                "Previewed ${edits.size} replacement(s) for ${result.query}.",
                data = mapOf("query" to result.query, "replacement" to replacement, "count" to edits.size, "edits" to edits.map { it.toValue() }),
            )
        }
        runWrite {
            edits.groupBy { it.filePath }.forEach { (path, fileEdits) ->
                val file = resolveVirtualFile(path) ?: return@forEach
                val doc = documentFor(file) ?: return@forEach
                fileEdits.sortedByDescending { it.startOffset }.forEach { doc.replaceString(it.startOffset, it.endOffset, replacement) }
                FileDocumentManager.getInstance().saveDocument(doc)
            }
        }
        return ideOk(
            "Applied ${edits.size} replacement(s) for ${result.query}.",
            data = mapOf("query" to result.query, "replacement" to replacement, "count" to edits.size, "edits" to edits.map { it.toValue() }),
        )
    }

    private fun documentFor(file: VirtualFile): Document? = FileDocumentManager.getInstance().getDocument(file)

    private fun resolveVirtualFile(path: String): VirtualFile? =
        LocalFileSystem.getInstance().refreshAndFindFileByPath(project.absolutePath(path))

    private fun projectInfo(): Map<String, Any?> {
        val info = com.intellij.openapi.application.ApplicationInfo.getInstance()
        val projectEntry = objects.put("project", project)
        return mapOf(
            "object" to objectValue(projectEntry),
            "name" to project.name,
            "path" to project.basePath,
            "isIndexing" to DumbService.isDumb(project),
            "ide" to mapOf("productName" to info.fullApplicationName, "build" to info.build.asString(), "fullVersion" to info.fullVersion),
        )
    }

    private fun selectedEditorInfo(): Map<String, Any?> {
        val editor = FileEditorManager.getInstance(project).selectedTextEditor ?: return mapOf("available" to false)
        val entry = objects.put("editor", editor)
        return mapOf(
            "available" to true,
            "object" to objectValue(entry),
            "caret" to editorCaretValue(editor),
            "selection" to editorSelectionValue(editor),
        )
    }

    private fun editorCaretValue(editor: Editor): Map<String, Any?> = readAction {
        mapOf(
            "offset" to editor.caretModel.offset,
            "line" to editor.caretModel.logicalPosition.line + 1,
            "column" to editor.caretModel.logicalPosition.column + 1,
        )
    }

    private fun editorSelectionValue(editor: Editor): Map<String, Any?> = readAction {
        mapOf(
            "hasSelection" to editor.selectionModel.hasSelection(),
            "text" to editor.selectionModel.selectedText,
            "startOffset" to editor.selectionModel.selectionStart,
            "endOffset" to editor.selectionModel.selectionEnd,
        )
    }

    private fun openFileInfos(): List<Map<String, Any?>> =
        FileEditorManager.getInstance(project).openFiles.map { file ->
            val entry = objects.put("file", file)
            mapOf("object" to objectValue(entry), "path" to file.path, "fileType" to file.fileType.name, "psiQuality" to psiQuality(file))
        }

    private fun observedLanguages(): List<Map<String, Any?>> =
        FileEditorManager.getInstance(project).openFiles.map { psiQuality(it) }.distinctBy { it["language"] }

    private fun psiQuality(file: VirtualFile): Map<String, Any?> = readAction {
        val psi = PsiManager.getInstance(project).findFile(file)
        val language = psi?.language?.id ?: file.fileType.name
        val semantic = psi != null && !language.equals("textmate", ignoreCase = true)
        mapOf(
            "language" to language,
            "fileType" to file.fileType.name,
            "psiAvailable" to (psi != null),
            "semantic" to semantic,
            "semanticReason" to if (semantic) null else "native_psi_unavailable",
        )
    }

    private fun methodCatalog(entry: AgentObjectEntry): List<Map<String, Any?>> =
        methodCatalogForType(entry.type, entry.value).map { it.toValue() }

    private fun objectValue(entry: AgentObjectEntry): Map<String, Any?> =
        objects.describe(entry) + mapOf("methods" to methodCatalog(entry))

    private fun methodCatalogForType(type: String, value: Any?): List<AgentMethod> =
        when (type) {
            "Project" -> listOf(
                AgentMethod("observe", "observe(): IdeObservation", readOnly = true),
                AgentMethod("capabilities", "capabilities(): IdeCapabilities", readOnly = true),
                AgentMethod("openFile", "openFile(filePath: string): File", readOnly = false),
                AgentMethod("createFile", "createFile(filePath:string,text?:string,overwrite?:boolean,openEditor?:boolean): File", readOnly = false),
                AgentMethod("search", "search(query: string, limit?: int): SearchResult", readOnly = true),
                AgentMethod("runConfigurations", "runConfigurations(query?:string,includeTemporary?:boolean,limit?:int): RunConfiguration[]", readOnly = true),
                AgentMethod("runConfigurationTypes", "runConfigurationTypes(query?:string,limit?:int): RunConfigurationType[]", readOnly = true),
                AgentMethod("createRunConfiguration", "createRunConfiguration(name,typeId,factoryId?:string,temporary?:boolean,patch?:object): RunConfiguration", readOnly = false, destructive = true),
            )
            "File" -> {
                val quality = (value as? VirtualFile)?.let { psiQuality(it) } ?: emptyMap()
                listOf(
                    AgentMethod("read", "read(): DocumentText", readOnly = true),
                    AgentMethod("openEditor", "openEditor(): Editor", readOnly = false),
                    AgentMethod("search", "search(query: string, limit?: int): SearchResult", readOnly = true),
                    AgentMethod("replace", "replace(startLine,startColumn,endLine,endColumn,text,expectedText): EditResult", readOnly = false),
                    AgentMethod("append", "append(text: string): EditResult", readOnly = false),
                    AgentMethod("delete", "delete(): DeleteResult", readOnly = false, destructive = true),
                    AgentMethod("diagnostics", "diagnostics(): DiagnosticList", readOnly = true),
                    AgentMethod("symbolAt", "symbolAt(line:int,column:int): Symbol", readOnly = true, available = quality["semantic"] == true, unavailableReason = if (quality["semantic"] == true) null else "native_psi_unavailable"),
                    AgentMethod("breakpoints", "breakpoints(): Breakpoint[]", readOnly = true),
                    AgentMethod("setBreakpoint", "setBreakpoint(line:int,typeId?:string,enabled?:boolean,temporary?:boolean): Breakpoint", readOnly = false, destructive = true),
                    AgentMethod("removeBreakpoint", "removeBreakpoint(line:int,typeId?:string): BreakpointRemoval", readOnly = false, destructive = true),
                    AgentMethod("close", "close(): CloseResult", readOnly = false),
                )
            }
            "Directory" -> listOf(
                AgentMethod("children", "children(): List<File|Directory>", readOnly = true),
                AgentMethod("find", "find(name: string): File|Directory", readOnly = true),
                AgentMethod("search", "search(query: string, limit?: int): SearchResult", readOnly = true),
                AgentMethod("createFile", "createFile(name:string,text?:string,overwrite?:boolean,openEditor?:boolean): File", readOnly = false),
                AgentMethod("createDirectory", "createDirectory(name:string): Directory", readOnly = false),
            )
            "Document" -> listOf(
                AgentMethod("text", "text(): string", readOnly = true),
                AgentMethod("replace", "replace(startLine,startColumn,endLine,endColumn,text,expectedText,clamp?:boolean): EditResult", readOnly = false),
                AgentMethod("replaceOffsets", "replaceOffsets(startOffset,endOffset,text,expectedText): EditResult", readOnly = false),
                AgentMethod("append", "append(text: string): EditResult", readOnly = false),
                AgentMethod("save", "save(): SaveResult", readOnly = false),
                AgentMethod("lineInfo", "lineInfo(line: int): LineInfo", readOnly = true),
            )
            "Editor" -> listOf(
                AgentMethod("caret", "caret(): CaretInfo", readOnly = true),
                AgentMethod("selection", "selection(): SelectionInfo", readOnly = true),
                AgentMethod("select", "select(startLine,startColumn,endLine,endColumn,clamp?:boolean): SelectionInfo", readOnly = false),
                AgentMethod("selectOffsets", "selectOffsets(startOffset,endOffset): SelectionInfo", readOnly = false),
                AgentMethod("insert", "insert(text: string): EditResult", readOnly = false),
                AgentMethod("replaceSelection", "replaceSelection(text: string): EditResult", readOnly = false),
                AgentMethod("symbolAt", "symbolAt(): Symbol", readOnly = true),
                AgentMethod("setBreakpoint", "setBreakpoint(line?:int,typeId?:string,enabled?:boolean,temporary?:boolean): Breakpoint", readOnly = false, destructive = true),
                AgentMethod("removeBreakpoint", "removeBreakpoint(line?:int,typeId?:string): BreakpointRemoval", readOnly = false, destructive = true),
                AgentMethod("close", "close(): CloseResult", readOnly = false),
            )
            "Symbol" -> listOf(
                AgentMethod("definition", "definition(): SymbolLocation", readOnly = true),
                AgentMethod("references", "references(limit?: int): ReferenceList", readOnly = true),
                AgentMethod("renamePreview", "renamePreview(newName: string): RefactorPreview", readOnly = true),
                AgentMethod("renameApply", "renameApply(newName: string): RefactorResult", readOnly = false, destructive = true),
            )
            "SearchResult" -> listOf(
                AgentMethod("items", "items(): SearchItem[]", readOnly = true),
                AgentMethod("openItem", "openItem(index: int): File", readOnly = false),
                AgentMethod("replaceAllPreview", "replaceAllPreview(text: string): EditPreview", readOnly = true),
                AgentMethod("replaceAllApply", "replaceAllApply(text: string): EditResult", readOnly = false, destructive = true),
            )
            "Plugin" -> {
                listOf(
                    AgentMethod("info", "info(): PluginInfo", readOnly = true),
                    AgentMethod("dependencies", "dependencies(): PluginDependency[]", readOnly = true),
                    AgentMethod("enable", "enable(): PluginMutationResult", readOnly = false, destructive = true),
                    AgentMethod("disable", "disable(): PluginMutationResult", readOnly = false, destructive = true),
                    AgentMethod("load", "load(): PluginMutationResult", readOnly = false, destructive = true),
                    AgentMethod("unload", "unload(): PluginMutationResult", readOnly = false, destructive = true),
                    AgentMethod("uninstall", "uninstall(): PluginMutationResult", readOnly = false, destructive = true),
                )
            }
            "Breakpoint" -> listOf(
                AgentMethod("info", "info(): BreakpointInfo", readOnly = true),
                AgentMethod("enable", "enable(): BreakpointInfo", readOnly = false, destructive = true),
                AgentMethod("disable", "disable(): BreakpointInfo", readOnly = false, destructive = true),
                AgentMethod("remove", "remove(): BreakpointRemoval", readOnly = false, destructive = true),
            )
            "RunConfigurationType" -> listOf(
                AgentMethod("info", "info(): RunConfigurationTypeInfo", readOnly = true),
                AgentMethod("factories", "factories(): RunConfigurationFactory[]", readOnly = true),
                AgentMethod("template", "template(factoryId?: string): RunConfiguration", readOnly = true),
            )
            "RunConfiguration" -> listOf(
                AgentMethod("info", "info(): RunConfigurationInfo", readOnly = true),
                AgentMethod("schema", "schema(): RunConfigurationPatchSchema", readOnly = true),
                AgentMethod("update", "update(patch: object): RunConfigurationInfo", readOnly = false, destructive = true),
                AgentMethod("select", "select(): RunConfigurationInfo", readOnly = false),
                AgentMethod("run", "run(): Execution", readOnly = false, destructive = true),
                AgentMethod("debug", "debug(): Execution", readOnly = false, destructive = true),
                AgentMethod("clone", "clone(name?: string): RunConfiguration", readOnly = false, destructive = true),
                AgentMethod("delete", "delete(): DeleteResult", readOnly = false, destructive = true),
            )
            "Execution" -> listOf(
                AgentMethod("status", "status(): ExecutionStatus", readOnly = true),
                AgentMethod("console", "console(limit?:int,offset?:int): ConsoleText", readOnly = true),
                AgentMethod("stop", "stop(): ExecutionStatus", readOnly = false, destructive = true),
            )
            else -> emptyList()
        }

    private fun unsupported(entry: AgentObjectEntry, method: String): Map<String, Any?> =
        ideError("unsupported_method", "${entry.type} object ${entry.alias} does not support method $method.", nextBestActions = methodCatalog(entry).map { "${entry.alias}.${it["name"]}" })

    private fun normalizeThrowable(t: Throwable): Map<String, Any?> {
        val message = t.cause?.message ?: t.message ?: t.javaClass.name
        return ideError("ide_exception", message)
    }

    private fun ideOk(
        summary: String,
        data: Map<String, Any?> = emptyMap(),
        affordances: List<Map<String, Any?>> = emptyList(),
        limitations: List<Any?> = emptyList(),
        nextBestActions: List<String> = emptyList(),
    ): Map<String, Any?> = mapOf(
        "ok" to true,
        "code" to "ok",
        "summary" to summary,
        "data" to data,
        "affordances" to affordances,
        "limitations" to limitations,
        "nextBestActions" to nextBestActions,
    )

    private fun ideUnavailable(code: String, summary: String, nextBestActions: List<String> = emptyList()): Map<String, Any?> =
        mapOf(
            "ok" to false,
            "code" to code,
            "summary" to summary,
            "data" to emptyMap<String, Any?>(),
            "affordances" to emptyList<Any>(),
            "limitations" to listOf(mapOf("code" to code, "message" to summary)),
            "nextBestActions" to nextBestActions,
        )

    private fun ideError(code: String, summary: String, nextBestActions: List<String> = emptyList()): Map<String, Any?> =
        ideUnavailable(code, summary, nextBestActions)

    private fun JsonElement?.obj(): JsonObject = if (this != null && isJsonObject) asJsonObject else JsonObject()

    private fun Project.absolutePath(path: String): String =
        if (Path.of(path).isAbsolute) path else Path.of(basePath ?: "", path).normalize().toString()

    private fun Project.relativePath(file: VirtualFile): String {
        val base = basePath ?: return file.path
        return Path.of(base).relativize(Path.of(file.path)).toString()
    }

    private fun Document.offsetAt(lineOneBased: Int, columnOneBased: Int, clamp: Boolean = true): Int? {
        if (!clamp && lineOneBased !in 1..lineCount.coerceAtLeast(1)) return null
        val line = (lineOneBased - 1).coerceIn(0, (lineCount - 1).coerceAtLeast(0))
        val lineStart = getLineStartOffset(line)
        val lineEnd = getLineEndOffset(line)
        val raw = lineStart + (columnOneBased - 1)
        if (!clamp && raw !in lineStart..lineEnd) return null
        return raw.coerceIn(lineStart, lineEnd)
    }

    private fun Document.lineTextAt(offset: Int): String {
        val safeOffset = offset.coerceIn(0, textLength.coerceAtLeast(0))
        val line = getLineNumber(safeOffset).coerceIn(0, (lineCount - 1).coerceAtLeast(0))
        return charsSequence.subSequence(getLineStartOffset(line), getLineEndOffset(line)).toString()
    }

    private fun <T> readAction(body: () -> T): T =
        ProgressManager.getInstance().runProcess(
            Computable { ApplicationManager.getApplication().runReadAction<T>(body) },
            EmptyProgressIndicator(),
        )

    private fun runWrite(body: () -> Unit) {
        WriteCommandAction.runWriteCommandAction(project) { body() }
    }

    private fun <T> runOnEdt(body: () -> T): T {
        val app = ApplicationManager.getApplication()
        if (app.isDispatchThread) return body()
        val result = AtomicReference<T?>()
        val error = AtomicReference<Throwable?>()
        app.invokeAndWait {
            try {
                result.set(body())
            } catch (t: Throwable) {
                error.set(t)
            }
        }
        error.get()?.let { throw it }
        @Suppress("UNCHECKED_CAST")
        return result.get() as T
    }

    private fun PsiElement.toAgentValue(): Map<String, Any?> {
        val file = containingFile?.virtualFile
        val doc = containingFile?.viewProvider?.document
        val offset = textOffset.coerceAtLeast(0)
        val line = doc?.getLineNumber(offset)?.plus(1)
        val col = if (doc != null && line != null) offset - doc.getLineStartOffset(line - 1) + 1 else null
        return mapOf(
            "elementClass" to javaClass.name,
            "elementType" to node?.elementType?.toString(),
            "name" to (this as? PsiNamedElement)?.name,
            "text" to text?.take(500),
            "filePath" to file?.path,
            "projectRelativePath" to file?.let { project.relativePath(it) },
            "line" to line,
            "column" to col,
            "language" to containingFile?.language?.id,
        )
    }

    private fun HighlightInfo.toDiagnosticValue(doc: Document, file: VirtualFile): Map<String, Any?> {
        val safeStart = startOffset.coerceIn(0, doc.textLength)
        val safeEnd = endOffset.coerceIn(safeStart, doc.textLength)
        val line = doc.getLineNumber(safeStart).coerceIn(0, (doc.lineCount - 1).coerceAtLeast(0))
        val endLine = doc.getLineNumber(safeEnd).coerceIn(0, (doc.lineCount - 1).coerceAtLeast(0))
        return mapOf(
            "severity" to severity.name,
            "description" to description,
            "toolId" to inspectionToolId,
            "filePath" to file.path,
            "projectRelativePath" to project.relativePath(file),
            "startOffset" to safeStart,
            "endOffset" to safeEnd,
            "line" to line + 1,
            "column" to safeStart - doc.getLineStartOffset(line) + 1,
            "endLine" to endLine + 1,
            "endColumn" to safeEnd - doc.getLineStartOffset(endLine) + 1,
            "text" to doc.charsSequence.subSequence(safeStart, safeEnd).toString().take(500),
        )
    }

    private fun UsageInfo.toUsageValue(): Map<String, Any?> {
        val element = element
        val file = virtualFile
        val doc = file?.let { documentFor(it) }
        val segment = segment ?: navigationRange
        val start = segment?.startOffset ?: navigationOffset
        val end = segment?.endOffset ?: start
        val line = doc?.getLineNumber(start.coerceIn(0, doc.textLength))?.plus(1)
        val column = if (doc != null && line != null) start - doc.getLineStartOffset(line - 1) + 1 else null
        return mapOf(
            "filePath" to file?.path,
            "projectRelativePath" to file?.let { project.relativePath(it) },
            "startOffset" to start,
            "endOffset" to end,
            "line" to line,
            "column" to column,
            "nonCodeUsage" to isNonCodeUsage,
            "dynamicUsage" to isDynamicUsage,
            "valid" to isValid,
            "text" to element?.text?.take(300),
        )
    }

    private fun com.intellij.psi.PsiReference.toAgentReferenceValue(): Map<String, Any?> {
        val element = element
        val file = element.containingFile?.virtualFile
        val doc = element.containingFile?.viewProvider?.document
        val start = element.textRange?.startOffset
        val line = if (doc != null && start != null) doc.getLineNumber(start).plus(1) else null
        val col = if (doc != null && line != null && start != null) start - doc.getLineStartOffset(line - 1) + 1 else null
        return mapOf(
            "referenceClass" to javaClass.name,
            "canonicalText" to canonicalText,
            "filePath" to file?.path,
            "projectRelativePath" to file?.let { project.relativePath(it) },
            "line" to line,
            "column" to col,
            "text" to element.text?.take(300),
        )
    }
}

private data class AgentObjectEntry(
    val alias: String,
    val type: String,
    val value: Any,
)

private data class AgentMethod(
    val name: String,
    val signature: String,
    val readOnly: Boolean,
    val destructive: Boolean = false,
    val available: Boolean = true,
    val unavailableReason: String? = null,
) {
    fun toValue(): Map<String, Any?> = mapOf(
        "name" to name,
        "signature" to signature,
        "readOnly" to readOnly,
        "destructive" to destructive,
        "available" to available,
        "unavailableReason" to unavailableReason,
    )
}

private data class AgentSearchResult(val query: String, val items: List<AgentSearchItem>)

private data class AgentSearchItem(
    val filePath: String,
    val projectRelativePath: String?,
    val line: Int,
    val column: Int,
    val text: String,
) {
    fun toValue(): Map<String, Any?> = mapOf(
        "filePath" to filePath,
        "projectRelativePath" to projectRelativePath,
        "line" to line,
        "column" to column,
        "text" to text,
    )
}

private data class AgentTextEdit(
    val filePath: String,
    val projectRelativePath: String?,
    val startOffset: Int,
    val endOffset: Int,
    val line: Int,
    val column: Int,
    val oldText: String,
    val newText: String,
) {
    fun toValue(): Map<String, Any?> = mapOf(
        "filePath" to filePath,
        "projectRelativePath" to projectRelativePath,
        "startOffset" to startOffset,
        "endOffset" to endOffset,
        "line" to line,
        "column" to column,
        "oldText" to oldText,
        "newText" to newText,
    )
}

private data class AgentScheduledExecution(
    val configId: String,
    val configurationName: String,
    val mode: String,
    val scheduledAtMillis: Long,
)

private class AgentObjectRegistry(private val project: Project) {
    private val counters = ConcurrentHashMap<String, AtomicInteger>()
    private val entries = ConcurrentHashMap<String, AgentObjectEntry>()
    private val identities = java.util.IdentityHashMap<Any, String>()

    @Synchronized
    fun put(prefix: String, value: Any): AgentObjectEntry {
        identities[value]?.let { existing ->
            entries[existing]?.let { return it }
        }
        val type = when (prefix) {
            "project" -> "Project"
            "file" -> "File"
            "directory" -> "Directory"
            "doc" -> "Document"
            "editor" -> "Editor"
            "symbol" -> "Symbol"
            "searchResult" -> "SearchResult"
            "plugin" -> "Plugin"
            "breakpoint" -> "Breakpoint"
            "runConfigType" -> "RunConfigurationType"
            "runConfig" -> "RunConfiguration"
            "execution" -> "Execution"
            else -> prefix.replaceFirstChar { it.uppercase() }
        }
        val aliasPrefix = when (prefix) {
            "searchResult" -> "search"
            "runConfigType" -> "runType"
            "runConfig" -> "runConfig"
            else -> prefix
        }
        val count = counters.computeIfAbsent(aliasPrefix) { AtomicInteger(0) }.incrementAndGet()
        val alias = "$aliasPrefix$count"
        val entry = AgentObjectEntry(alias, type, value)
        entries[alias] = entry
        identities[value] = alias
        return entry
    }

    fun get(ref: String): AgentObjectEntry? = entries[ref]

    fun list(): List<AgentObjectEntry> = entries.values.sortedBy { it.alias }

    @Synchronized
    fun release(ref: String): Boolean {
        val entry = entries.remove(ref) ?: return false
        identities.remove(entry.value)
        return true
    }

    @Synchronized
    fun findAlias(value: Any): String? = identities[value]

    fun describe(entry: AgentObjectEntry): Map<String, Any?> {
        val metadata = when (val value = entry.value) {
            is Project -> mapOf("name" to value.name, "path" to value.basePath, "valid" to !value.isDisposed)
            is VirtualFile -> mapOf(
                "path" to value.path,
                "name" to value.name,
                "directory" to value.isDirectory,
                "valid" to value.isValid,
                "fileType" to value.fileType.name,
                "projectRelativePath" to runCatching { project.relativePath(value) }.getOrNull(),
            )
            is Document -> registryReadAction { mapOf("lineCount" to value.lineCount, "textLength" to value.textLength, "valid" to true) }
            is Editor -> registryReadAction { mapOf("offset" to value.caretModel.offset, "valid" to !value.isDisposed) }
            is PsiElement -> mapOf("valid" to value.isValid, "text" to value.text?.take(120))
            is AgentSearchResult -> mapOf("query" to value.query, "count" to value.items.size, "valid" to true)
            is IdeaPluginDescriptor -> mapOf(
                "pluginId" to value.pluginId.idString,
                "name" to value.name,
                "version" to value.version,
                "enabled" to value.isEnabled,
                "bundled" to value.isBundled,
                "valid" to true,
            )
            is XBreakpoint<*> -> mapOf(
                "typeId" to value.type.id,
                "enabled" to value.isEnabled,
                "filePath" to (value as? XLineBreakpoint<*>)?.presentableFilePath,
                "line" to (value as? XLineBreakpoint<*>)?.line?.plus(1),
                "valid" to true,
            )
            is ConfigurationType -> mapOf(
                "typeId" to value.id,
                "displayName" to value.displayName,
                "factoryCount" to value.configurationFactories.size,
                "valid" to true,
            )
            is RunnerAndConfigurationSettings -> mapOf(
                "name" to value.name,
                "configId" to value.uniqueID,
                "typeId" to value.type.id,
                "factoryId" to value.factory.id,
                "temporary" to value.isTemporary,
                "template" to value.isTemplate,
                "valid" to true,
            )
            is ProcessHandler -> mapOf(
                "executionClass" to value.javaClass.name,
                "terminated" to value.isProcessTerminated,
                "terminating" to value.isProcessTerminating,
                "exitCode" to value.exitCode,
                "valid" to true,
            )
            is AgentScheduledExecution -> mapOf(
                "configId" to value.configId,
                "configurationName" to value.configurationName,
                "mode" to value.mode,
                "scheduledOnly" to true,
                "valid" to true,
            )
            else -> mapOf("valid" to true, "display" to value.toString().take(200))
        }
        return mapOf(
            "alias" to entry.alias,
            "ref" to entry.alias,
            "type" to entry.type,
            "className" to entry.value.javaClass.name,
            "metadata" to metadata,
        )
    }

    private fun Project.relativePath(file: VirtualFile): String {
        val base = basePath ?: return file.path
        return Path.of(base).relativize(Path.of(file.path)).toString()
    }

    private fun <T> registryReadAction(body: () -> T): T =
        ApplicationManager.getApplication().runReadAction<T>(body)
}
