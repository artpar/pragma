package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonObject
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class ReflectiveToolCatalogTest {
    @Test
    fun `default descriptors have stable unique names and schemas`() {
        val catalog = ReflectiveToolCatalog(FakeIdePorts())
        val definitions = catalog.toolDefinitions()
        val names = definitions.map { it["name"] as String }

        assertEquals(names.size, names.toSet().size)
        assertTrue("com.intellij.psi.PsiReference.resolve" in names)
        assertTrue("com.intellij.psi.search.PsiSearchHelper.processElementsWithWord" in names)
        assertTrue("com.intellij.openapi.actionSystem.ActionManager.tryToExecute" in names)
        assertTrue("java.lang.reflect.Method.invoke" in names)
        assertTrue("com.github.artpar.pragma.jetbrains.reflect.Roots.list" in names)
        assertTrue("ide.file.open" in names)
        assertTrue("ide.object.call" in names)
        assertTrue("ide.plugin.list" in names)
        assertTrue("ide.plugin.install" in names)
        assertTrue("ide.plugin.self.update" in names)
        assertTrue("ide.debug.breakpoint.set" in names)
        definitions.forEach { definition ->
            assertTrue((definition["description"] as String).isNotBlank())
            assertEquals("object", (definition["inputSchema"] as Map<*, *>)["type"])
            assertNotNull(definition["annotations"])
        }
    }

    @Test
    fun `catalog executes tool through fake ports`() {
        val catalog = ReflectiveToolCatalog(FakeIdePorts())
        val args = JsonObject().apply {
            addProperty("filePath", "src/Main.kt")
            addProperty("line", 10)
            addProperty("column", 3)
        }

        val result = catalog.call("com.intellij.psi.PsiReference.resolve", args)

        assertFalse(result["error"] == true)
        assertEquals("PsiElement", ((result["result"] as Map<*, *>)["element"] as Map<*, *>)["name"])
    }

    @Test
    fun `missing tool returns structured tool error`() {
        val catalog = ReflectiveToolCatalog(FakeIdePorts())

        val result = catalog.call("missing", JsonObject())

        assertEquals(true, result["error"])
        assertTrue((result["registeredTools"] as List<*>).isNotEmpty())
    }

    @Test
    fun `destructive action descriptor is annotated`() {
        val catalog = ReflectiveToolCatalog(FakeIdePorts())
        val definition = catalog.toolDefinitions()
            .first { it["name"] == "com.intellij.openapi.actionSystem.ActionManager.tryToExecute" }

        val annotations = definition["annotations"] as Map<*, *>
        assertEquals(false, annotations["readOnlyHint"])
        assertEquals(true, annotations["destructiveHint"])
    }

    @Test
    fun `generic reflective invocation descriptor is destructive`() {
        val catalog = ReflectiveToolCatalog(FakeIdePorts())
        val definition = catalog.toolDefinitions()
            .first { it["name"] == "java.lang.reflect.Method.invoke" }

        val annotations = definition["annotations"] as Map<*, *>
        assertEquals(false, annotations["readOnlyHint"])
        assertEquals(true, annotations["destructiveHint"])
    }

    @Test
    fun `agent ide file open returns object method catalog`() {
        val catalog = ReflectiveToolCatalog(FakeIdePorts())
        val args = JsonObject().apply { addProperty("filePath", "src/Main.kt") }

        val result = catalog.call("ide.file.open", args)

        assertFalse(result["error"] == true)
        val data = ((result["result"] as Map<*, *>)["data"] as Map<*, *>)
        val obj = data["object"] as Map<*, *>
        assertEquals("file1", obj["alias"])
        assertEquals("File", obj["type"])
        assertTrue((obj["methods"] as List<*>).isNotEmpty())
    }

    @Test
    fun `object call accepts stringified json arguments from model providers`() {
        val catalog = ReflectiveToolCatalog(FakeIdePorts())
        val args = JsonObject().apply {
            addProperty("object", "doc1")
            addProperty("method", "replace")
            addProperty("arguments", """{"startLine":25,"startColumn":1,"text":"hello"}""")
        }

        val result = catalog.call("ide.object.call", args)

        assertFalse(result["error"] == true)
        val data = (result["result"] as Map<*, *>)["data"] as Map<*, *>
        assertEquals("replace", data["method"])
        assertEquals(25, data["startLine"])
        assertEquals("hello", data["text"])
    }
}

class FakeIdePorts : IdePorts {
    override val application: ApplicationPort = object : ApplicationPort {
        override fun projectInfo(): ProjectInfo = ProjectInfo(
            projectName = "test",
            projectPath = "/tmp/test",
            isIndexing = false,
            ide = IdeInfo("2024.1", "241", "IntelliJ IDEA"),
            application = ApplicationState(false, false, false),
        )
    }

    override val actions: ActionPort = object : ActionPort {
        override fun actionIds(prefix: String): List<String> = listOf("${prefix}SaveAll")

        override fun actionInfo(actionId: String): ActionInfo =
            ActionInfo(actionId, found = true, actionClass = "Action", text = "Save All", description = "Save files")

        override fun tryToExecute(actionId: String, now: Boolean): ActionExecutionResult =
            ActionExecutionResult(actionId, scheduled = true, context = "fake", now = now)
    }

    override val editor: EditorPort = object : EditorPort {
        override fun selectedTextEditor(): EditorContext = EditorContext(
            available = true,
            filePath = "/tmp/test/src/Main.kt",
            projectRelativePath = "src/Main.kt",
            caret = CaretInfo(42, 10, 3),
            selection = SelectionInfo(false, 42, 42, null),
        )
    }

    override val psi: PsiPort = object : PsiPort {
        override fun elementAt(position: TextPosition): PsiElementAtResult =
            PsiElementAtResult(41, true, element("Leaf"), listOf(element("Parent")))

        override fun resolveReference(position: TextPosition): PsiResolveResult =
            PsiResolveResult(41, "Reference", "PsiElement", true, element("PsiElement"))

        override fun references(position: TextPosition, limit: Int): ReferencesResult =
            ReferencesResult(element("Target"), limit, 1, listOf(reference()))

        override fun filesByName(name: String): List<VirtualFileInfo> =
            listOf(VirtualFileInfo("/tmp/test/$name", name, "Kotlin"))

        override fun elementsWithWord(word: String, context: String, limit: Int, includeHidden: Boolean): WordSearchResult =
            WordSearchResult(word, context, limit, 1, false, listOf(wordOccurrence(word)))
    }

    override val vfs: VfsPort = object : VfsPort {
        override fun refreshAndFindFileByPath(filePath: String): RefreshFileResult =
            RefreshFileResult(filePath, true, filePath.substringAfterLast('/'), "Kotlin", "kotlin", true)
    }

    override val agentIde: AgentIdePort = object : AgentIdePort {
        override fun observe(): Map<String, Any?> = ok("Observed.")
        override fun capabilities(): Map<String, Any?> = ok("Capabilities.")
        override fun objects(): Map<String, Any?> = ok("Objects.", mapOf("objects" to emptyList<Any>()))
        override fun describeObject(input: AgentObjectInput): Map<String, Any?> = ok("Described.", mapOf("object" to fileObject()))
        override fun callObject(input: AgentObjectCallInput): Map<String, Any?> = ok(
            "Called.",
            mapOf(
                "method" to input.method,
                "startLine" to input.arguments?.asJsonObject?.get("startLine")?.asInt,
                "text" to input.arguments?.asJsonObject?.get("text")?.asString,
            ),
        )
        override fun releaseObject(input: AgentObjectInput): Map<String, Any?> = ok("Released.", mapOf("released" to true))
        override fun openFile(input: AgentFileInput): Map<String, Any?> = ok("Opened.", mapOf("object" to fileObject()))
        override fun resolveFile(input: AgentFileInput): Map<String, Any?> = ok("Resolved.", mapOf("object" to fileObject()))
        override fun searchText(input: AgentSearchInput): Map<String, Any?> = ok("Searched.", mapOf("items" to emptyList<Any>()))
        override fun listPlugins(input: AgentPluginListInput): Map<String, Any?> = ok("Plugins.", mapOf("objects" to listOf(pluginObject())))
        override fun resolvePlugin(input: AgentPluginInput): Map<String, Any?> = ok("Plugin.", mapOf("object" to pluginObject()))
        override fun enablePlugin(input: AgentPluginInput): Map<String, Any?> = ok("Enabled.", mapOf("changed" to true))
        override fun disablePlugin(input: AgentPluginInput): Map<String, Any?> = ok("Disabled.", mapOf("changed" to true))
        override fun loadPlugin(input: AgentPluginInput): Map<String, Any?> = ok("Loaded.", mapOf("changed" to true))
        override fun unloadPlugin(input: AgentPluginInput): Map<String, Any?> = ok("Unloaded.", mapOf("changed" to true))
        override fun installPlugin(input: AgentPluginInstallInput): Map<String, Any?> = ok("Installed.", mapOf("changed" to true))
        override fun uninstallPlugin(input: AgentPluginInput): Map<String, Any?> = ok("Uninstalled.", mapOf("changed" to true))
        override fun selfUpdatePlugin(input: AgentPluginSelfUpdateInput): Map<String, Any?> = ok("Self update staged.", mapOf("selfUpdate" to true, "restartRequired" to true))
        override fun listBreakpoints(): Map<String, Any?> = ok("Breakpoints.", mapOf("objects" to emptyList<Any>()))
        override fun setBreakpoint(input: AgentBreakpointInput): Map<String, Any?> = ok("Breakpoint set.", mapOf("object" to breakpointObject()))
        override fun removeBreakpoint(input: AgentBreakpointInput): Map<String, Any?> = ok("Breakpoint removed.", mapOf("removed" to true))

        private fun ok(summary: String, data: Map<String, Any?> = emptyMap()): Map<String, Any?> = mapOf(
            "ok" to true,
            "code" to "ok",
            "summary" to summary,
            "data" to data,
            "affordances" to emptyList<Any>(),
            "limitations" to emptyList<Any>(),
            "nextBestActions" to emptyList<Any>(),
        )

        private fun fileObject(): Map<String, Any?> = mapOf(
            "alias" to "file1",
            "ref" to "file1",
            "type" to "File",
            "methods" to listOf(mapOf("name" to "read", "signature" to "read(): DocumentText", "readOnly" to true)),
        )

        private fun pluginObject(): Map<String, Any?> = mapOf(
            "alias" to "plugin1",
            "ref" to "plugin1",
            "type" to "Plugin",
            "methods" to listOf(mapOf("name" to "info", "signature" to "info(): PluginInfo", "readOnly" to true)),
        )

        private fun breakpointObject(): Map<String, Any?> = mapOf(
            "alias" to "breakpoint1",
            "ref" to "breakpoint1",
            "type" to "Breakpoint",
            "methods" to listOf(mapOf("name" to "info", "signature" to "info(): BreakpointInfo", "readOnly" to true)),
        )
    }

    override val reflection: ReflectionPort = object : ReflectionPort {
        override fun protocol(): Map<String, Any?> = mapOf("formatVersion" to 1)
        override fun roots(): Map<String, Any?> = mapOf("roots" to mapOf("root:project" to mapOf("className" to "Project")))
        override fun classForName(className: String): Map<String, Any?> = mapOf("class" to className)
        override fun describeClass(input: ReflectiveClassInput): Map<String, Any?> = mapOf("class" to input.className)
        override fun constructors(input: ReflectiveClassInput): Map<String, Any?> = mapOf("constructors" to emptyList<Any>())
        override fun getField(input: ReflectiveFieldInput): Map<String, Any?> = mapOf("value" to input.fieldName)
        override fun newInstance(input: ReflectiveConstructorInput): Map<String, Any?> = mapOf("value" to input.className)
        override fun invoke(input: ReflectiveInvocationInput): Map<String, Any?> = mapOf("value" to input.methodName)
        override fun invokeReadAction(input: ReflectiveInvocationInput): Map<String, Any?> = mapOf("value" to input.methodName)
        override fun invokeWriteCommand(input: ReflectiveInvocationInput): Map<String, Any?> = mapOf("value" to input.methodName)
        override fun handles(): Map<String, Any?> = mapOf("handles" to emptyMap<String, Any>())
        override fun handle(ref: String): Map<String, Any?> = mapOf("\$ref" to ref)
        override fun release(ref: String): Map<String, Any?> = mapOf("released" to true)
    }

    private fun element(name: String): PsiElementInfo = PsiElementInfo(
        elementClass = "FakePsiElement",
        elementType = "IDENTIFIER",
        name = name,
        text = name,
        filePath = "/tmp/test/src/Main.kt",
        projectRelativePath = "src/Main.kt",
        line = 10,
        column = 3,
        language = "kotlin",
    )

    private fun reference(): ReferenceInfo = ReferenceInfo(
        referenceClass = "FakeReference",
        canonicalText = "Target",
        filePath = "/tmp/test/src/Main.kt",
        projectRelativePath = "src/Main.kt",
        line = 10,
        column = 3,
        text = "Target",
    )

    private fun wordOccurrence(word: String): WordOccurrenceInfo = WordOccurrenceInfo(
        filePath = "/tmp/test/src/Main.kt",
        projectRelativePath = "src/Main.kt",
        line = 10,
        column = 3,
        text = "fun $word()",
        element = element(word),
    )
}
