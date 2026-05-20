package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonObject
import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.createTempDirectory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class PluginCompatibilityTest {
    @Test
    fun `plugin manifest is platform only and does not require Java plugin`() {
        val pluginXml = Files.readString(Path.of("src/main/resources/META-INF/plugin.xml"))

        assertTrue("<depends>com.intellij.modules.platform</depends>" in pluginXml)
        assertFalse("<depends>com.intellij.java</depends>" in pluginXml)
    }

    @Test
    fun `gradle sandbox does not force Java plugin into runtime`() {
        val build = Files.readString(Path.of("build.gradle.kts"))

        assertFalse("plugins.set(listOf(\"java\"))" in build)
        assertFalse("plugins = [\"java\"]" in build)
    }

    @Test
    fun `plugin lifecycle mutations are dispatched on EDT`() {
        val source = Files.readString(Path.of("src/main/kotlin/com/github/artpar/pragma/jetbrains/mcp/AgentIdeRuntime.kt"))

        assertTrue("runOnEdt { PluginEnabler.getInstance().enable" in source)
        assertTrue("runOnEdt { PluginEnabler.getInstance().disable" in source)
        assertTrue("runOnEdt { PluginInstaller.installAndLoadDynamicPlugin" in source)
        assertTrue("runOnEdt { PluginInstaller.prepareToUninstall" in source)
        assertTrue("runOnEdt { InstalledPluginsState.getInstance().isRestartRequired = true }" in source)
        assertFalse("val changed = PluginEnabler.getInstance().enable" in source)
        assertFalse("val changed = PluginEnabler.getInstance().disable" in source)
        assertFalse("PluginInstaller.prepareToUninstall(impl)" in source.replace("runOnEdt { PluginInstaller.prepareToUninstall(impl) }", ""))
    }

    @Test
    fun `remembered project ports are read from stable per project file`() {
        val dir = createTempDirectory("pragma-mcp-discovery")
        val projectPath = "/tmp/example-web-project"
        val hash = sha256(projectPath).take(16)
        val ports = dir.resolve("ports")
        Files.createDirectories(ports)
        Files.writeString(ports.resolve("$hash.port"), "59901")

        assertEquals(59901, readRememberedPort(dir, projectPath))
    }

    @Test
    fun `invalid remembered project ports are ignored`() {
        val dir = createTempDirectory("pragma-mcp-discovery")
        val projectPath = "/tmp/example-web-project"
        val hash = sha256(projectPath).take(16)
        val ports = dir.resolve("ports")
        Files.createDirectories(ports)
        Files.writeString(ports.resolve("$hash.port"), "99999")

        assertEquals(null, readRememberedPort(dir, projectPath))
    }

    @Test
    fun `document replacement requires matching expected text`() {
        val missing = validateReplacementExpectedText(JsonObject(), "current")
        assertEquals(false, missing?.get("ok"))
        assertEquals("missing_expected_text", missing?.get("code"))

        val staleArgs = JsonObject().apply { addProperty("expectedText", "old") }
        val stale = validateReplacementExpectedText(staleArgs, "current")
        assertEquals(false, stale?.get("ok"))
        assertEquals("stale_text_range", stale?.get("code"))

        val matchingArgs = JsonObject().apply { addProperty("expectedText", "current") }
        assertNull(validateReplacementExpectedText(matchingArgs, "current"))
    }
}
