# QML plugin kind and Go IPC (issue #4)

Research for a slide-from-top console (over all windows, keyboard-driven) built as a
Quickshell QML front end driving a long-lived Go backend.

Verified against a live quattro install (`/usr/share/omarchy`, version `4.0.0.alpha`,
Quickshell 0.3.0 rev `28771c7`), the quattro branch of `basecamp/omarchy`
(`docs/omarchy-shell.md`, `shell/plugins/README.md`), https://omarchy.org/manual/shell-plugins/,
and Quickshell primary sources (v0.3.0 docs + `quickshell-mirror/quickshell` `src/io/process.hpp`,
plus the installed `/usr/lib/qt6/qml/Quickshell/Io/quickshell-io.qmltypes`).

## 1. Plugin kinds

From `basecamp/omarchy` quattro `docs/omarchy-shell.md` (same table on omarchy.org/manual/shell-plugins):

| Kind         | What it is |
|--------------|------------|
| `bar-widget` | Component the active bar drops into a section |
| `bar`        | Full bar option that can replace `omarchy.bar` |
| `panel`      | Floating window (e.g. OSD) |
| `overlay`    | Fullscreen overlay (e.g. clipboard, emojis, image picker) |
| `menu`       | Summoned menu surface |
| `service`    | Headless singleton, no UI |

### The host treats panel / overlay / menu identically

Key finding from `/usr/share/omarchy/shell/shell.qml` (`computePanelEntries`, the panel
`Instantiator`, and `isBarWidgetPanelPlugin`): **`panel`, `overlay`, and `menu` all go through
the exact same Loader path** — one `Loader` per enabled plugin, `active` when
`keepLoaded || openPanelIds[id]`, `summon`/`hide`/`toggle` delivered via the plugin's
`open(payloadJson)` / `close()` functions. The kind picks which `entryPoints` key is loaded
and carries semantic intent; it does **not** set layering, focus, or animation. Those are
entirely plugin-owned, set on the plugin's own `PanelWindow`:

```qml
// overlay example — plugins/clipboard/Clipboard.qml:314-322
PanelWindow {
  visible: root.opened
  anchors { top: true; bottom: true; left: true; right: true }
  color: "transparent"
  WlrLayershell.namespace: "omarchy-clipboard"
  WlrLayershell.layer: WlrLayer.Overlay          // above all windows, incl. fullscreen
  WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive  // grabs typing
  exclusionMode: ExclusionMode.Ignore
}
```

- **Layering**: every summoned first-party surface uses `WlrLayer.Overlay` (clipboard, emojis,
  menu, OSD, notifications). Overlay layer stacks above normal and fullscreen windows.
- **Input focus**: interactive overlays use `WlrKeyboardFocus.Exclusive` (clipboard/emojis/menu);
  passive ones use `None` plus an empty input `mask: Region {}` (OSD, notifications) so clicks
  pass through. A console needs `Exclusive`.
- **Animation**: no host-provided open/close animation exists. Plugins animate inside their own
  (usually transparent, full-screen-anchored) layer surface with plain QML `Behavior`/
  `NumberAnimation` (e.g. `osd/Osd.qml:182-184`, `bar/Bar.qml:1265`). A slide-from-top console
  = transparent full-screen (or top-anchored) `PanelWindow` + a card whose `y` animates from
  `-height` to its margin.
- **`service`** entry points are headless `Item`s created at startup into an invisible
  `serviceHost` (`shell.qml:263-321`), destroyed when disabled/removed. If a plugin declares a
  service *and* a panel/overlay kind, the host injects the service instance into the loaded
  panel item as `item.service` (`shell.qml:637`) — the `omarchy.media` pattern for shared state.
- **`keepLoaded: true`** (top-level manifest key) keeps the Loader active from startup:
  "Services and keep-loaded panels are mounted at startup; other panels, overlays, and menus
  are loaded on demand" (`shell/plugins/README.md`). The image picker uses it "so the
  layer-shell window survives between summons within a single shell session."

### Which kind fits

**`kinds: ["overlay"]` with `keepLoaded: true`.** Rationale:

- Mechanically any of panel/overlay/menu would work (same loader), but omarchy's own taxonomy
  puts full-screen, keyboard-exclusive, summoned surfaces under `overlay` (clipboard, emojis,
  image picker); `panel` is exemplified by the OSD, a non-interactive floating readout.
- No `service` combo needed. A `keepLoaded` overlay is mounted at startup exactly like a
  service, so the plugin item (and any child Process / session state) already lives across
  summon/hide. A separate service entry point would only add value if headless state had to
  outlive the *window object*, which `keepLoaded` already prevents.

## 2. Spawning and driving a long-lived child process from QML

Quickshell's `Quickshell.Io` module (confirmed in the installed
`quickshell-io.qmltypes`) provides everything needed:

- **`Process`** — `command` (argv list, no shell), `running`, `workingDirectory`,
  `environment`, `stdinEnabled` + `write(data)`, `signal(int)`, `stdout`/`stderr` accepting a
  `DataStreamParser` (`SplitParser` for line/record framing, `StdioCollector` for
  run-to-completion capture), `started`/`exited` signals, `startDetached()`.
- **`Socket`** — client for a Unix domain socket: `path`, `connected`, `write()`, `flush()`,
  same `DataStream` parser interface for reads, `error` signal.
- **`SocketServer`** — listens on a Unix socket path with a per-connection `handler` component
  (this is what backs the shell's own IPC).

First-party precedent for a stdin-driven child: `plugins/panels/network/Panel.qml:777-786`
(`stdinEnabled: true`, `write(secret + "\n")` on `onStarted` — "The password goes over stdin,
never argv"). Long-running watcher children with auto-restart: `plugins/clipboard/Clipboard.qml`
(`textWatchProc`/`imageWatchProc` + a restart `Timer`). Line-framed stdout consumption via
`SplitParser` is used throughout the bar widgets.

### Process lifetime (the critical constraint)

From Quickshell primary sources (`src/io/process.hpp` doc comments; v0.3.0 docs):

- "The process will be killed when quickshell dies."
- A tracked process is killed when its `Process` object is destroyed or the configuration is
  reloaded — "See `startDetached()` to prevent the process from being killed by Quickshell if
  Quickshell is killed or the configuration is reloaded."
- `startDetached()`: "The subprocess will not be tracked, `running` will be false, and the
  subprocess will not be killed by Quickshell" — but a detached child has **no stdio pipes**
  back to QML, so stdio IPC and detachment are mutually exclusive.

## 3. What survives which reload

Four distinct teardown levels, verified in `shell.qml` and plugin sources:

| Event | Plugin QML object | Child `Process` | `PersistentProperties` | On-disk state |
|---|---|---|---|---|
| hide → summon (with `keepLoaded: true`) | survives (Loader stays active) | survives | n/a | survives |
| plugin rescan (`omarchy-shell shell rescanPlugins`; automatic on save under `~/.config/omarchy/plugins/`) | destroyed + recreated (`unloadPanels`/`unloadPluginServices` + `Qt.clearComponentCache`, `shell.qml:741-756`) | **SIGTERMed** (tracked child dies with its Process object) | not restored (this path bypasses Quickshell's reload machinery) | survives |
| Quickshell config reload (editing files under `$OMARCHY_PATH/shell`) | destroyed + recreated | killed unless detached | **restored** — "PersistentProperties holds properties declared in it across a reload" (Quickshell docs); used with `reloadableId` in `notifications/Service.qml:65` and `services/battery/Service.qml:16` | survives |
| shell restart (`omarchy-restart-shell`) / logout | gone | killed (unless detached/external) | gone | survives |

The notifications service states the idiom outright (`Service.qml:61-64`):
"PersistentProperties handles in-process QML reloads. The on-disk notifications.json file is
the cross-restart backstop." The agents plugin likewise keeps all durable data as JSON files
under `$XDG_STATE_HOME/omarchy/...` watched with `FileView`.

**Conclusion:** nothing in-process is guaranteed across a plugin rescan or shell restart.
Durable state must live on disk; the Go backend must be restartable and able to rehydrate.

## 4. IPC design options: stdio vs Unix socket

**A. Plugin-owned child over stdio (recommended).**
`Process { command: ["omazork-backend"]; stdinEnabled: true; stdout: SplitParser { ... } }`,
NDJSON request/response + events, one JSON object per line.

- Pros: zero lifecycle management — the shell starts it (keepLoaded → at login), kills it with
  the shell, no orphans, no socket path/cleanup/permissions, no reconnect state machine.
  Matches existing first-party patterns (stdin writes, SplitParser reads, restart Timer).
- Cons: the Go process dies on plugin rescan / shell restart. Mitigate in the backend, where it
  belongs anyway for a Z-machine: autosave (Quetzal snapshot) after every turn and/or on
  SIGTERM, restore on start. A reload then costs a respawn + restore, invisible to the user.

**B. Independent Go daemon + Unix socket.**
Backend listens on `$XDG_RUNTIME_DIR/omazork.sock`; QML connects with `Socket`, spawns the
daemon on demand via `Quickshell.execDetached` / `Process.startDetached()` when the connect
fails, and reconnects with a retry timer.

- Pros: game session survives plugin rescans and shell restarts in live memory.
- Cons: orphan management (daemon outlives logout unless it self-terminates), stale-socket
  cleanup, version skew between a long-running daemon and reloaded QML, a reconnect/handshake
  state machine in QML — all to protect state that a Z-machine can checkpoint to disk in
  milliseconds anyway.

**Recommendation: A.** `kinds: ["overlay"]`, `keepLoaded: true`, one `Process` child speaking
line-framed NDJSON over stdio, with the Go side owning durability (autosave every turn, save on
SIGTERM, restore on start). Keep the protocol transport-agnostic (a `net.Conn`-shaped reader/
writer pair in Go) so option B remains a drop-in upgrade if reload-survival in live memory ever
matters.

Sketch:

```qml
// overlay entry point (excerpt)
Process {
  id: backend
  command: [root.manifest.__sourceDir + "/bin/omazork-backend"]
  stdinEnabled: true
  running: true                       // keepLoaded => spawned at shell startup
  stdout: SplitParser {               // one NDJSON message per line
    onRead: data => root.handleMessage(JSON.parse(data))
  }
  onExited: respawnTimer.restart()    // clipboard-watcher pattern
}
function send(msg) { backend.write(JSON.stringify(msg) + "\n") }
```

```qml
PanelWindow {
  visible: root.opened
  anchors { top: true; left: true; right: true; bottom: true }
  color: "transparent"
  WlrLayershell.layer: WlrLayer.Overlay
  WlrLayershell.keyboardFocus: root.opened ? WlrKeyboardFocus.Exclusive : WlrKeyboardFocus.None
  exclusionMode: ExclusionMode.Ignore
  Item {  // the console card slides in
    y: root.opened ? 0 : -height
    Behavior on y { NumberAnimation { duration: 180; easing.type: Easing.OutCubic } }
  }
}
```

## Sources

- `/usr/share/omarchy/shell/shell.qml` — panel/overlay/menu loader, service host, summon/hide,
  reloadPlugins (`4.0.0.alpha`)
- `/usr/share/omarchy/shell/plugins/README.md`, `.../services/PluginRegistry.qml` — kinds,
  manifest schema, keepLoaded, first-party enablement
- `/usr/share/omarchy/shell/plugins/{clipboard/Clipboard.qml,osd/Osd.qml,emojis/Emojis.qml,menu/Menu.qml,notifications/Service.qml,services/battery/Service.qml,panels/network/Panel.qml,agents/Main.qml}`
- `basecamp/omarchy` quattro branch: `docs/omarchy-shell.md` (kinds table, IPC method table)
- https://omarchy.org/manual/shell-plugins/ (kinds, manifest, live-reload of `~/.config/omarchy/plugins/`)
- Quickshell v0.3.0 docs: `Quickshell.Io.Process`, `Quickshell.Io.Socket`,
  `Quickshell.PersistentProperties`; `quickshell-mirror/quickshell` `src/io/process.hpp`
- `/usr/lib/qt6/qml/Quickshell/Io/quickshell-io.qmltypes` (Process/Socket/SocketServer/SplitParser
  API surface as installed)
