package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.Gson
import com.intellij.openapi.project.Project
import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpHandler
import java.nio.charset.StandardCharsets

class McpHttpHandler(
    private val project: Project,
    private val registry: ReflectiveToolRegistryView,
) : HttpHandler {
    private val gson = Gson()
    private val protocol = McpProtocol(registry) { serverInfo() }

    override fun handle(exchange: HttpExchange) {
        try {
            when (exchange.requestMethod.uppercase()) {
                "GET" -> writeJson(exchange, 200, serverInfo())
                "POST" -> handlePost(exchange)
                "DELETE" -> writeEmpty(exchange, 202)
                else -> writeJson(exchange, 405, error(null, -32600, "Method not allowed"))
            }
        } catch (t: Throwable) {
            writeJson(exchange, 500, error(null, -32603, t.message ?: t.javaClass.name))
        } finally {
            exchange.close()
        }
    }

    private fun handlePost(exchange: HttpExchange) {
        val body = exchange.requestBody.readBytes().toString(StandardCharsets.UTF_8)
        when (val response = protocol.handleJson(body)) {
            is McpProtocolResponse.Accepted -> writeEmpty(exchange, 202)
            is McpProtocolResponse.Json -> writeJson(exchange, 200, response.value)
        }
    }

    private fun serverInfo(): Map<String, Any?> {
        return mapOf(
            "server" to "pragma-jetbrains-reflective-mcp",
            "projectName" to project.name,
            "projectPath" to project.basePath,
            "tools" to registry.toolNames(),
        )
    }

    private fun error(id: Any?, code: Int, message: String): Map<String, Any?> {
        return mapOf(
            "jsonrpc" to "2.0",
            "id" to id,
            "error" to mapOf("code" to code, "message" to message),
        )
    }

    private fun writeJson(exchange: HttpExchange, status: Int, value: Any) {
        val bytes = gson.toJson(value).toByteArray(StandardCharsets.UTF_8)
        exchange.responseHeaders.set("Content-Type", "application/json")
        exchange.sendResponseHeaders(status, bytes.size.toLong())
        exchange.responseBody.write(bytes)
    }

    private fun writeEmpty(exchange: HttpExchange, status: Int) {
        exchange.sendResponseHeaders(status, -1)
    }
}
