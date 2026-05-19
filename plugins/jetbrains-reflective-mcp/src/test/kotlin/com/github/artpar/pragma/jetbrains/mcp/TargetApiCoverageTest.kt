package com.github.artpar.pragma.jetbrains.mcp

import java.lang.reflect.Method
import java.lang.reflect.Modifier
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlin.test.fail

class TargetApiCoverageTest {
    @Test
    fun `target api matrix has complete category coverage`() {
        val categories = targetApiMatrix.map { it.category }.toSet()

        assertEquals(
            setOf(
                "actions",
                "application-threading",
                "diagnostics-inspections",
                "duplicates",
                "debug-breakpoints",
                "editor-documents",
                "intentions-quickfixes",
                "plugin-management",
                "project-model",
                "psi-navigation",
                "refactoring",
                "run-build-test",
                "search-indexes",
                "vfs",
            ),
            categories,
        )
        targetApiMatrix.groupBy { it.category }.forEach { (category, entries) ->
            assertTrue(entries.isNotEmpty(), "No target APIs declared for $category")
        }
    }

    @Test
    fun `all declared target classes are present in IntelliJ runtime`() {
        val missing = targetApiMatrix.mapNotNull { api ->
            runCatching { loadClass(api.className) }
                .exceptionOrNull()
                ?.let { "${api.category}: ${api.className} (${it.javaClass.simpleName}: ${it.message})" }
        }

        assertTrue(missing.isEmpty(), "Missing target classes:\n${missing.joinToString("\n")}")
    }

    @Test
    fun `all declared target methods are present with expected static shape`() {
        val missing = mutableListOf<String>()

        targetApiMatrix.forEach { api ->
            val clazz = loadClass(api.className)
            api.methods.forEach { spec ->
                val method = findMethod(clazz, spec)
                if (method == null) {
                    missing += "${api.category}: ${api.className}.${spec.name}${spec.parameterTypes?.joinToString(prefix = "(", postfix = ")") ?: "(*)"}"
                } else if (spec.static != null && Modifier.isStatic(method.modifiers) != spec.static) {
                    missing += "${api.category}: ${api.className}.${spec.name} static=${Modifier.isStatic(method.modifiers)}, expected=${spec.static}"
                }
            }
        }

        assertTrue(missing.isEmpty(), "Missing target methods:\n${missing.joinToString("\n")}")
    }

    @Test
    fun `all declared reflective roots are backed by covered target classes`() {
        val optionalRootClasses = mapOf(
            "root:project" to "com.intellij.openapi.project.Project",
            "root:application" to "com.intellij.openapi.application.Application",
            "root:actionManager" to "com.intellij.openapi.actionSystem.ActionManager",
            "root:fileEditorManager" to "com.intellij.openapi.fileEditor.FileEditorManager",
            "root:fileDocumentManager" to "com.intellij.openapi.fileEditor.FileDocumentManager",
            "root:psiManager" to "com.intellij.psi.PsiManager",
            "root:psiDocumentManager" to "com.intellij.psi.PsiDocumentManager",
            "root:localFileSystem" to "com.intellij.openapi.vfs.LocalFileSystem",
            "root:dumbService" to "com.intellij.openapi.project.DumbService",
            "root:projectRootManager" to "com.intellij.openapi.roots.ProjectRootManager",
            "root:runManager" to "com.intellij.execution.RunManager",
            "root:daemonCodeAnalyzer" to "com.intellij.codeInsight.daemon.DaemonCodeAnalyzer",
            "root:inspectionProfileManager" to "com.intellij.profile.codeInspection.InspectionProjectProfileManager",
            "root:refactoringFactory" to "com.intellij.refactoring.RefactoringFactory",
        )
        val coveredClasses = targetApiMatrix.map { it.className }.toSet()
        val uncovered = optionalRootClasses.filterValues { it !in coveredClasses }

        assertTrue(uncovered.isEmpty(), "Reflective roots without target API coverage: $uncovered")
    }

    @Test
    fun `reflective bridge tools cover every target api category`() {
        val toolNames = ReflectiveToolCatalog(FakeIdePorts()).toolNames().toSet()
        val required = setOf(
            "ide.observe",
            "ide.capabilities",
            "ide.object.list",
            "ide.object.describe",
            "ide.object.call",
            "ide.object.release",
            "ide.file.open",
            "ide.file.resolve",
            "ide.search.text",
            "ide.plugin.list",
            "ide.plugin.resolve",
            "ide.plugin.enable",
            "ide.plugin.disable",
            "ide.plugin.load",
            "ide.plugin.unload",
            "ide.plugin.install",
            "ide.plugin.uninstall",
            "ide.plugin.self.update",
            "ide.debug.breakpoints",
            "ide.debug.breakpoint.set",
            "ide.debug.breakpoint.remove",
            "com.github.artpar.pragma.jetbrains.reflect.Protocol.describe",
            "com.github.artpar.pragma.jetbrains.reflect.Roots.list",
            "java.lang.Class.forName",
            "java.lang.Class.describe",
            "java.lang.Class.getConstructors",
            "java.lang.reflect.Field.get",
            "java.lang.reflect.Constructor.newInstance",
            "java.lang.reflect.Method.invoke",
            "com.intellij.openapi.application.Application.runReadAction",
            "com.intellij.openapi.command.WriteCommandAction.runWriteCommandAction",
            "com.github.artpar.pragma.jetbrains.reflect.ObjectStore.list",
            "com.github.artpar.pragma.jetbrains.reflect.ObjectStore.get",
            "com.github.artpar.pragma.jetbrains.reflect.ObjectStore.release",
        )

        assertTrue(toolNames.containsAll(required), "Missing reflective tools: ${required - toolNames}")
    }

    private fun findMethod(clazz: Class<*>, spec: TargetMethod): Method? {
        val candidates = clazz.methods.toList() + clazz.declaredMethods.toList()
        return candidates.firstOrNull { method ->
            method.name == spec.name && (
                spec.parameterTypes == null ||
                    method.parameterTypes.map { it.name } == spec.parameterTypes
                )
        }
    }

    private fun loadClass(name: String): Class<*> {
        return when (name) {
            "boolean" -> java.lang.Boolean.TYPE
            "byte" -> java.lang.Byte.TYPE
            "char" -> java.lang.Character.TYPE
            "short" -> java.lang.Short.TYPE
            "int" -> java.lang.Integer.TYPE
            "long" -> java.lang.Long.TYPE
            "float" -> java.lang.Float.TYPE
            "double" -> java.lang.Double.TYPE
            "void" -> java.lang.Void.TYPE
            else -> runCatching { Class.forName(name) }.getOrElse {
                if (name.startsWith("[")) Class.forName(name) else fail("Class not found: $name", it)
            }
        }
    }
}
