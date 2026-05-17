package com.github.artpar.pragma.jetbrains.mcp

import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.StartupActivity

class McpStartupActivity : StartupActivity.DumbAware {
    override fun runActivity(project: Project) {
        project.getService(ReflectiveMcpProjectService::class.java).start()
    }
}
