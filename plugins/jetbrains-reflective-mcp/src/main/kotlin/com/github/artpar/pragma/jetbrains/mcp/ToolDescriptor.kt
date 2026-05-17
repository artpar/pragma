package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonObject

class ToolDescriptor<I : Any>(
    val name: String,
    val source: String,
    val inputSchema: Map<String, Any> = objectSchema(),
    val readOnly: Boolean = true,
    val destructive: Boolean = false,
    private val decode: (JsonObject) -> I,
    private val execute: (I, IdePorts) -> Any?,
) {
    val description: String =
        "Reflective IntelliJ Platform bridge for $source. The MCP tool name mirrors the platform API/action surface; responses include the originating source when possible."

    fun definition(): Map<String, Any> {
        return mapOf(
            "name" to name,
            "description" to description,
            "inputSchema" to inputSchema,
            "annotations" to mapOf(
                "readOnlyHint" to readOnly,
                "destructiveHint" to destructive,
            ),
        )
    }

    fun call(arguments: JsonObject, ports: IdePorts): Map<String, Any?> {
        val input = decode(arguments)
        val output = execute(input, ports)
        @Suppress("UNCHECKED_CAST")
        return mapOf(
            "source" to source,
            "result" to output.toBoundaryValue(),
        )
    }
}

class ReflectiveToolCatalog(
    private val ports: IdePorts,
    private val descriptors: List<ToolDescriptor<*>> = defaultToolDescriptors(),
) : ReflectiveToolRegistryView {
    private val byName = descriptors.associateBy { it.name }

    override fun toolNames(): List<String> = descriptors.map { it.name }

    override fun toolDefinitions(): List<Map<String, Any>> = descriptors.map { it.definition() }

    override fun call(name: String, arguments: JsonObject): Map<String, Any?> {
        val descriptor = byName[name] ?: return mapOf(
            "error" to true,
            "message" to "No IntelliJ Platform tool registered for $name",
            "registeredTools" to byName.keys.sorted(),
        )

        return try {
            descriptor.call(arguments, ports)
        } catch (t: Throwable) {
            mapOf(
                "error" to true,
                "tool" to name,
                "message" to (t.message ?: t.javaClass.name),
            )
        }
    }
}
