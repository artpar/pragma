package com.github.artpar.pragma.jetbrains.mcp

import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertFalse
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
}
