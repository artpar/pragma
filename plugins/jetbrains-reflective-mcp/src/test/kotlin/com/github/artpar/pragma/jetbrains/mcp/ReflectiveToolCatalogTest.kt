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
