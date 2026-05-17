package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.Gson
import com.google.gson.JsonObject
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class McpProtocolTest {
    private val gson = Gson()

    @Test
    fun `initialize advertises tool capability and server identity`() {
        val response = protocol().json("""{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}""")

        assertEquals("2.0", response["jsonrpc"])
        assertEquals("2025-03-26", response.result()["protocolVersion"])
        assertEquals("pragma-jetbrains-reflective-mcp", response.result().obj("serverInfo")["name"])
        assertTrue(response.result().obj("capabilities").containsKey("tools"))
    }

    @Test
    fun `tools list is generated from reflective registry`() {
        val response = protocol().json("""{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}""")
        val tools = response.result().array("tools")

        assertEquals(1, tools.size)
        assertEquals("com.intellij.example.Source.method", tools[0].asMap()["name"])
        assertEquals("object", tools[0].asMap().obj("inputSchema")["type"])
    }

    @Test
    fun `tool call returns text and structured content`() {
        val response = protocol().json(
            """{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"com.intellij.example.Source.method","arguments":{"value":"abc"}}}"""
        )

        val result = response.result()
        assertFalse(result["isError"] as Boolean)
        assertEquals("abc", result.obj("structuredContent")["echo"])
        val content = result.array("content")[0].asMap()
        assertEquals("text", content["type"])
        assertTrue((content["text"] as String).contains("abc"))
    }

    @Test
    fun `missing tool is returned as tool error not protocol error`() {
        val response = protocol().json(
            """{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"missing","arguments":{}}}"""
        )

        val result = response.result()
        assertTrue(result["isError"] as Boolean)
        assertEquals(true, result.obj("structuredContent")["error"])
    }

    @Test
    fun `initialized notification is accepted without json response`() {
        val response = protocol().handleJson("""{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}""")

        assertTrue(response is McpProtocolResponse.Accepted)
    }

    private fun protocol(): McpProtocol {
        return McpProtocol(FakeRegistry()) {
            mapOf("server" to "test", "tools" to listOf("com.intellij.example.Source.method"))
        }
    }

    private fun McpProtocol.json(request: String): Map<String, Any?> {
        val response = handleJson(request)
        require(response is McpProtocolResponse.Json)
        @Suppress("UNCHECKED_CAST")
        return gson.fromJson(gson.toJson(response.value), Map::class.java) as Map<String, Any?>
    }

    private fun Map<String, Any?>.result(): Map<String, Any?> {
        @Suppress("UNCHECKED_CAST")
        return this["result"] as Map<String, Any?>
    }

    private fun Map<String, Any?>.obj(name: String): Map<String, Any?> {
        @Suppress("UNCHECKED_CAST")
        return this[name] as Map<String, Any?>
    }

    private fun Map<String, Any?>.array(name: String): List<Any> {
        @Suppress("UNCHECKED_CAST")
        return this[name] as List<Any>
    }

    private fun Any.asMap(): Map<String, Any?> {
        @Suppress("UNCHECKED_CAST")
        return this as Map<String, Any?>
    }

    private class FakeRegistry : ReflectiveToolRegistryView {
        private val name = "com.intellij.example.Source.method"
        private val inputSchema = objectSchema(mapOf("value" to stringProp("Echo value.")))

        override fun toolNames(): List<String> = listOf(name)

        override fun toolDefinitions(): List<Map<String, Any>> = listOf(
            mapOf(
                "name" to name,
                "description" to "Reflective test tool",
                "inputSchema" to inputSchema,
                "annotations" to mapOf("readOnlyHint" to true, "destructiveHint" to false),
            )
        )

        override fun call(name: String, arguments: JsonObject): Map<String, Any?> {
            if (name != this.name) {
                return mapOf("error" to true, "message" to "missing")
            }
            return mapOf("source" to name, "echo" to arguments.stringArg("value"))
        }
    }
}
