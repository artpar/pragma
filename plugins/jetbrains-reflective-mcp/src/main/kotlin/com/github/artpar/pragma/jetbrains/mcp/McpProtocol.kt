package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.Gson
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser

class McpProtocol(
    private val registry: ReflectiveToolRegistryView,
    private val serverInfo: () -> Map<String, Any?>,
) {
    private val gson = Gson()

    fun handleJson(requestJson: String): McpProtocolResponse {
        val request = JsonParser.parseString(requestJson).asJsonObject
        val id = request.get("id")
        return when (val method = request.get("method")?.asString) {
            "initialize" -> McpProtocolResponse.Json(response(id, initializeResult()))
            "notifications/initialized" -> McpProtocolResponse.Accepted
            "ping" -> McpProtocolResponse.Json(response(id, mapOf<String, Any>()))
            "tools/list" -> McpProtocolResponse.Json(response(id, mapOf("tools" to registry.toolDefinitions())))
            "tools/call" -> McpProtocolResponse.Json(response(id, callTool(request)))
            else -> McpProtocolResponse.Json(error(id, -32601, "Unknown MCP method: $method"))
        }
    }

    fun describeServer(): Map<String, Any?> = serverInfo()

    private fun initializeResult(): Map<String, Any> {
        return mapOf(
            "protocolVersion" to "2025-03-26",
            "capabilities" to mapOf(
                "tools" to mapOf("listChanged" to false),
            ),
            "serverInfo" to mapOf(
                "name" to "pragma-jetbrains-reflective-mcp",
                "version" to "0.1.0",
            ),
        )
    }

    private fun callTool(request: JsonObject): Map<String, Any> {
        val params = request.getAsJsonObject("params")
        val name = params.get("name").asString
        val args = params.getAsJsonObject("arguments") ?: JsonObject()
        val result = registry.call(name, args)
        val text = gson.toJson(result)
        return mapOf(
            "content" to listOf(mapOf("type" to "text", "text" to text)),
            "structuredContent" to result,
            "isError" to (result["error"] == true),
        )
    }

    private fun response(id: JsonElement?, result: Any): Map<String, Any?> {
        return mapOf("jsonrpc" to "2.0", "id" to id, "result" to result)
    }

    private fun error(id: JsonElement?, code: Int, message: String): Map<String, Any?> {
        return mapOf(
            "jsonrpc" to "2.0",
            "id" to id,
            "error" to mapOf("code" to code, "message" to message),
        )
    }
}

sealed class McpProtocolResponse {
    data class Json(val value: Any) : McpProtocolResponse()
    data object Accepted : McpProtocolResponse()
}

interface ReflectiveToolRegistryView {
    fun toolNames(): List<String>
    fun toolDefinitions(): List<Map<String, Any>>
    fun call(name: String, arguments: JsonObject): Map<String, Any?>
}
