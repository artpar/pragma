package com.github.artpar.pragma.jetbrains.mcp

import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationInfo
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.sun.net.httpserver.HttpServer
import java.net.InetAddress
import java.net.InetSocketAddress
import java.time.Instant
import java.nio.file.Files
import java.nio.file.Path
import java.security.MessageDigest
import java.util.UUID
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

private const val LEASE_TTL_MS = 30_000L
private const val HEARTBEAT_INTERVAL_MS = 5_000L

class ReflectiveMcpProjectService(private val project: Project) : Disposable {
    private var server: HttpServer? = null
    private var executor = Executors.newSingleThreadExecutor()
    private var heartbeatExecutor = Executors.newSingleThreadScheduledExecutor()
    private val ideInstanceId = UUID.randomUUID().toString()
    private val startedAt = Instant.now().toString()

    fun start() {
        if (server != null || project.isDisposed) return

        val port = System.getProperty("pragma.jetbrains.mcp.port")?.toIntOrNull() ?: 0
        val bindAddress = InetAddress.getByName("127.0.0.1")
        val httpServer = HttpServer.create(InetSocketAddress(bindAddress, port), 0)
        val registry = ReflectiveToolCatalog(IntelliJIdePorts(project))
        httpServer.createContext("/", McpHttpHandler(project, registry))
        httpServer.executor = executor
        httpServer.start()
        server = httpServer
        writeStatus(httpServer.address.port, registry)
        heartbeatExecutor.scheduleAtFixedRate(
            { writeStatus(httpServer.address.port, registry) },
            HEARTBEAT_INTERVAL_MS,
            HEARTBEAT_INTERVAL_MS,
            TimeUnit.MILLISECONDS,
        )
    }

    private fun writeStatus(port: Int, registry: ReflectiveToolRegistryView) {
        val home = System.getProperty("user.home") ?: return
        val basePath = project.basePath ?: project.name
        val canonicalProjectPath = Path.of(basePath).toAbsolutePath().normalize().toString()
        val hash = sha256(canonicalProjectPath).take(16)
        val dir = Path.of(home, ".pragma", "jetbrains-mcp")
        val projectsDir = dir.resolve("projects")
        Files.createDirectories(dir)
        Files.createDirectories(projectsDir)
        val appInfo = ApplicationInfo.getInstance()
        val url = "http://127.0.0.1:$port"
        val body = """
            {
              "server": "pragma-jetbrains-reflective-mcp",
              "projectName": ${jsonString(project.name)},
              "projectPath": ${jsonString(project.basePath ?: "")},
              "canonicalProjectPath": ${jsonString(canonicalProjectPath)},
              "ideProduct": ${jsonString(appInfo.versionName)},
              "ideVersion": ${jsonString(appInfo.fullVersion)},
              "ideInstanceId": ${jsonString(ideInstanceId)},
              "pid": ${ProcessHandle.current().pid()},
              "port": $port,
              "url": ${jsonString(url)},
              "startedAt": ${jsonString(startedAt)},
              "updatedAt": ${jsonString(Instant.now().toString())},
              "ttlMs": $LEASE_TTL_MS,
              "mcpConfig": {
                "mcpServers": {
                  "jetbrains-${hash}": {
                    "type": "http",
                    "url": ${jsonString(url)}
                  }
                }
              },
              "toolCount": ${registry.toolNames().size},
              "pluginVersion": ${jsonString(ReflectiveMcpProjectService::class.java.`package`.implementationVersion ?: "")}
            }
        """.trimIndent()
        Files.writeString(dir.resolve("$hash.json"), body)
        Files.writeString(projectsDir.resolve("$hash.json"), body)
        Files.writeString(dir.resolve("latest.json"), body)
    }

    override fun dispose() {
        val home = System.getProperty("user.home")
        val basePath = project.basePath ?: project.name
        val canonicalProjectPath = Path.of(basePath).toAbsolutePath().normalize().toString()
        val hash = sha256(canonicalProjectPath).take(16)
        server?.stop(0)
        server = null
        heartbeatExecutor.shutdownNow()
        executor.shutdownNow()
        if (home != null) {
            val dir = Path.of(home, ".pragma", "jetbrains-mcp")
            Files.deleteIfExists(dir.resolve("$hash.json"))
            Files.deleteIfExists(dir.resolve("projects").resolve("$hash.json"))
            val latest = dir.resolve("latest.json")
            if (Files.exists(latest) && Files.readString(latest).contains("\"ideInstanceId\": ${jsonString(ideInstanceId)}")) {
                Files.deleteIfExists(latest)
            }
        }
    }

    init {
        Disposer.register(project, this)
    }
}

private fun sha256(value: String): String {
    val digest = MessageDigest.getInstance("SHA-256").digest(value.toByteArray())
    return digest.joinToString("") { "%02x".format(it) }
}

internal fun jsonString(value: String): String {
    return buildString {
        append('"')
        value.forEach { ch ->
            when (ch) {
                '\\' -> append("\\\\")
                '"' -> append("\\\"")
                '\n' -> append("\\n")
                '\r' -> append("\\r")
                '\t' -> append("\\t")
                else -> append(ch)
            }
        }
        append('"')
    }
}
