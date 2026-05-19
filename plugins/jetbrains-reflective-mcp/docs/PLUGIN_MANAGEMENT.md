# Plugin Management

The MCP plugin exposes IntelliJ plugin management as part of the agent object interface. This lets an agent inspect installed plugins, resolve descriptors, enable or disable plugins, dynamically load or unload plugins where IntelliJ allows it, and install or uninstall plugins through IntelliJ's own plugin APIs.

## Tools

```text
ide.plugin.list
ide.plugin.resolve
ide.plugin.enable
ide.plugin.disable
ide.plugin.load
ide.plugin.unload
ide.plugin.install
ide.plugin.uninstall
ide.plugin.self.update
```

`ide.plugin.list`, `ide.plugin.resolve`, and `plugin.info()` are read-only. Everything else is marked destructive in MCP annotations because it can change IDE state, project behavior, classloaders, or files under the IDE plugin directory.

## Plugin Objects

Plugin tools return `Plugin` objects:

```json
{
  "alias": "plugin1",
  "ref": "plugin1",
  "type": "Plugin",
  "metadata": {
    "pluginId": "org.jetbrains.kotlin",
    "name": "Kotlin",
    "version": "241.14494.240",
    "enabled": true,
    "bundled": true,
    "valid": true
  },
  "methods": [
    {"name":"info","signature":"info(): PluginInfo","readOnly":true},
    {"name":"disable","signature":"disable(): PluginMutationResult","readOnly":false,"destructive":true}
  ]
}
```

Agents should prefer object methods after a plugin has been resolved:

```json
{"name":"ide.plugin.resolve","arguments":{"pluginId":"org.jetbrains.kotlin"}}
```

```json
{"name":"ide.object.call","arguments":{"object":"plugin1","method":"info"}}
```

## Listing

`ide.plugin.list` accepts:

```json
{
  "query": "kotlin",
  "includeBundled": true,
  "includeDisabled": true,
  "limit": 50
}
```

The query matches plugin id and plugin name. Results are returned as live `Plugin` objects, not only plain records.

## Resolving

`ide.plugin.resolve` accepts a concrete IntelliJ plugin id:

```json
{"pluginId":"org.jetbrains.kotlin"}
```

It returns one `Plugin` object and a plugin info record. If the id is not installed or known to the runtime plugin set, the plugin returns `plugin_not_found`.

## Enable And Disable

`ide.plugin.enable` and `ide.plugin.disable` call IntelliJ `PluginEnabler`.

The result includes:

- `changed`: whether IntelliJ accepted the state change.
- `plugin`: current descriptor information.
- `restartRequired`: whether IntelliJ reports a restart is required.

When the target is this MCP plugin itself, the operation is allowed. The plugin marks restart required and returns `selfMutation: true`, `staged: true`, and `agentMustReconnectAfterRestart: true` so the harness can restart IntelliJ and reconnect cleanly.

## Dynamic Load And Unload

`ide.plugin.load` and `ide.plugin.unload` use IntelliJ `DynamicPlugins`.

Dynamic unload is first checked with `DynamicPlugins.checkCanUnloadWithoutRestart`. If IntelliJ reports a blocker, the MCP response is:

```json
{
  "ok": false,
  "code": "restart_required",
  "summary": "Plugin ... cannot be unloaded dynamically: ...",
  "nextBestActions": ["ide.plugin.disable"]
}
```

This is intentional. The agent should treat the IDE as the source of truth and not assume classloader operations are always possible.

## Install

`ide.plugin.install` supports two input shapes:

Install from Marketplace:

```json
{"pluginId":"some.marketplace.plugin.id"}
```

Install from a local artifact path:

```json
{"path":"/absolute/path/plugin.zip"}
```

For Marketplace installs, the plugin uses `RepositoryHelper` and `PluginDownloader`. It first prepares the download, then tries `installDynamically`. If dynamic install is not possible, it falls back to a normal install and reports that a restart may be required.

For local artifacts, the plugin reads the descriptor from the artifact through IntelliJ's plugin descriptor loader and then calls `PluginInstaller.installAndLoadDynamicPlugin`.

If the local artifact is this MCP plugin itself, the plugin does not try to unload and replace its own classloader while serving the request. It stages the update with `PluginInstaller.installAfterRestart`, reports `restartRequired: true`, and leaves the MCP server alive until the IDE is restarted.

## Bootstrap Install

The first install cannot be initiated through this MCP server because the server is not available until the plugin is already loaded. A harness should install the ZIP into IntelliJ using the IDE plugin install path or startup configuration, start IntelliJ, discover the MCP endpoint, and then hand control to the agent.

After bootstrap, the agent can manage the plugin through the IDE surface:

- `ide.plugin.self.update` stages a replacement ZIP for the next restart.
- `ide.plugin.disable` against this plugin stages removal from the enabled plugin set.
- `ide.plugin.unload` against this plugin stages disable-after-restart instead of dropping the active classloader mid-request.
- `ide.plugin.uninstall` against this plugin stages uninstall after restart.

Each self lifecycle result returns `selfMutation: true`, `staged: true`, `restartRequired: true`, and `agentMustReconnectAfterRestart: true`. That is the contract a harness needs in order to stop/start IntelliJ and reconnect the agent.

## Self Update

Use `ide.plugin.self.update` to upgrade the MCP plugin itself from a local plugin ZIP:

```json
{"filePath":"/absolute/path/pragma-jetbrains-reflective-mcp-0.1.0.zip"}
```

The artifact descriptor must have plugin id `com.github.artpar.pragma.jetbrains.reflectiveMcp`. If the id differs, the plugin returns `plugin_id_mismatch`.

Self update is always staged for restart:

- It validates the ZIP descriptor.
- It schedules IntelliJ startup action commands to replace the current plugin path.
- It marks restart required.
- It does not unload, disable, or uninstall the running MCP plugin.

Direct self disable, unload, and uninstall are supported as staged lifecycle operations. They do not remove the running MCP classloader during the active request; they return a restart/reconnect contract that an external harness can execute.

## Uninstall

`ide.plugin.uninstall` uses `PluginInstaller.uninstallDynamicPlugin` when possible. If IntelliJ cannot uninstall dynamically, it calls `PluginInstaller.prepareToUninstall` and reports that the uninstall is scheduled after restart.

When uninstalling this MCP plugin itself, the plugin always stages uninstall after restart and reports `selfMutation: true`, `staged: true`, `dynamic: false`, and `agentMustReconnectAfterRestart: true`.

## Safety Model

Plugin management is powerful and can change the running IDE process. The interface follows these rules:

- Never fake success. If IntelliJ requires restart, return `restart_required` or a result with `restartRequired: true`.
- Self-mutation is staged. Do not unload the active MCP classloader mid-request; return the restart/reconnect contract.
- Return concrete plugin ids, names, versions, paths, enabled state, loaded state, bundled state, and dependencies.
- Use IntelliJ APIs only. Do not delete plugin files with direct filesystem operations.

## Target API Coverage

The plugin management surface is covered in `TargetApiMatrix.kt` under the `plugin-management` category. The test suite verifies the IntelliJ classes and method signatures used for:

- `PluginManagerCore`
- `PluginEnabler`
- `DynamicPlugins`
- `PluginInstaller`
- `RepositoryHelper`
- `PluginDownloader`
- `PluginId`
- `IdeaPluginDescriptor`

Run:

```bash
gradle --no-daemon -p plugins/jetbrains-reflective-mcp test
```
