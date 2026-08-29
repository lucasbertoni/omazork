// omazork overlay entry point (#15). The shell host loads this via
// manifest entryPoints.overlay, injects `manifest`, and drives summon/hide
// through open()/close(). Layering, keyboard focus, and the slide animation
// are plugin-owned (#4); the Go backend owns all game state and durability,
// reached over NDJSON on stdio (docs/protocol.md).
import QtQuick
import Quickshell
import Quickshell.Io
import Quickshell.Wayland

Item {
    id: root

    // ---- host contract ----
    property var manifest: null
    property var shell: null // injected by the host; null in a bare `qs` dev run
    property bool opened: false

    function open(payloadJson) {
        root.opened = true
        if (root.engineState !== "ready") return
        if (root.screen === "console") root.send({ type: "opened" })
        else root.send({ type: "picker" })
    }

    function close() {
        if (root.opened && root.engineState === "ready" && root.screen === "console")
            root.send({ type: "closed" })
        closeGrace.restart()
        root.opened = false
    }

    readonly property string pluginId: root.manifest && root.manifest.id
        ? root.manifest.id : "omazork"

    // Esc: route through the host so its open-state stays in sync; the local
    // close keeps a bare `qs` dev run working without the host.
    function requestClose() {
        root.close()
        Quickshell.execDetached(["omarchy-shell", "shell", "hide", root.pluginId])
    }

    // Theme adoption (docs/adr/0001): follows the active omarchy theme unless
    // the shell.json plugin entry opts out with `"theme": "phosphor"`.
    Theme { id: consoleTheme; shell: root.shell; pluginId: root.pluginId }

    readonly property string pluginRoot: {
        var u = Qt.resolvedUrl("..").toString()
        if (u.indexOf("file://") === 0) u = u.substring(7)
        while (u.length > 1 && u.charAt(u.length - 1) === "/")
            u = u.substring(0, u.length - 1)
        return u
    }

    // ---- engine bootstrap (#12: idempotent, run on every launch) ----
    property string engineState: "bootstrapping" // bootstrapping | ready | error
    property string engineError: ""

    Process {
        id: bootstrap
        command: ["bash", root.pluginRoot + "/scripts/bootstrap.sh"]
        workingDirectory: root.pluginRoot
        running: true
        stderr: StdioCollector { id: bootstrapErr }
        onExited: exitCode => {
            if (exitCode === 0) {
                root.engineState = "ready"
                backend.running = true
            } else {
                root.engineState = "error"
                root.engineError =
                    ("bootstrap failed (exit " + exitCode + ")\n" + bootstrapErr.text).trim()
            }
        }
    }

    function retryBootstrap() {
        if (root.engineState !== "error") return
        root.engineState = "bootstrapping"
        root.engineError = ""
        bootstrap.running = true
    }

    // ---- backend child: NDJSON over stdio ----
    Process {
        id: backend
        command: [root.pluginRoot + "/bin/omazork"]
        stdinEnabled: true
        stdout: SplitParser {
            onRead: data => root.handleMessage(data)
        }
        stderr: SplitParser {
            onRead: data => console.warn("omazork backend: " + data)
        }
        onStarted: {
            // Durability lives in the backend: after a respawn (plugin rescan,
            // crash) resuming the active game is invisible to the player.
            if (root.screen === "console" && root.currentGame !== "") {
                transcript.clear()
                root.send({ type: "resume", game: root.currentGame })
                if (root.opened) root.send({ type: "opened" })
            } else {
                root.send({ type: "picker" })
            }
        }
        onExited: {
            if (root.engineState === "ready") respawn.restart()
        }
    }
    Timer { id: respawn; interval: 1000; onTriggered: backend.running = true }

    function send(msg) {
        if (!backend.running) return
        backend.write(JSON.stringify(msg) + "\n")
    }

    // ---- session / ui state (display mirror of the backend's state) ----
    property string screen: "picker" // picker | console
    property var games: []
    property int pickerIndex: 0
    property string pickerStage: "list" // list | mode | confirm
    property int pickerGameIndex: -1
    property string pickerMode: "casual"
    property bool awaitingEnter: false

    property string currentGame: ""
    property string currentTitle: ""
    property string mode: ""
    property string room: ""
    property int score: 0
    property int moves: 0
    property bool finished: false
    property var pending: null // { maturesAt: Date }
    property int pendingMinutes: 0
    property var checkpoints: []
    property var restoreChoices: null

    ListModel { id: transcript }
    property alias transcriptModel: transcript

    function cleanOutput(text) {
        // the Z-machine ends its output with the next "\n>" prompt; the
        // console draws its own input line, so strip it
        return String(text).replace(/\s+$/, "").replace(/\n?>$/, "").replace(/\s+$/, "")
    }

    function appendLine(kind, text) {
        if (text === undefined || text === null) return
        var t = cleanOutput(text)
        if (t === "") return
        transcript.append({ kind: kind, text: t })
    }

    function rebuildTranscript(lines) {
        transcript.clear()
        if (!lines) return
        for (var i = 0; i < lines.length; i++) {
            var t = lines[i]
            var kind = t.indexOf("> ") === 0 ? "cmd"
                     : t.indexOf("[") === 0 ? "meta" : "out"
            t = cleanOutput(t)
            if (t !== "") transcript.append({ kind: kind, text: t })
        }
    }

    function applyStatus(st) {
        if (!st) return
        root.room = st.room
        root.score = st.score
        root.moves = st.moves
    }

    function setPending(p) {
        root.pending = p ? { maturesAt: new Date(p.maturesAt) } : null
        root.updatePendingMinutes()
    }
    function updatePendingMinutes() {
        if (!root.pending) { root.pendingMinutes = 0; return }
        root.pendingMinutes = Math.max(0,
            Math.ceil((root.pending.maturesAt.getTime() - Date.now()) / 60000))
    }
    Timer {
        interval: 30000; repeat: true
        running: root.pending !== null
        onTriggered: root.updatePendingMinutes()
    }

    function fmtTs(iso) {
        return Qt.formatDateTime(new Date(iso), "MMM d hh:mm")
    }

    function showUnlocked(list) {
        if (!list) return
        for (var i = 0; i < list.length; i++)
            appendLine("meta", "[Achievement unlocked: " + list[i].name
                + (list[i].description ? " — " + list[i].description : "") + "]")
    }

    function showReveal(r) {
        appendLine("meta", "— While you were away… —")
        appendLine("cmd", "> " + r.command)
        appendLine("out", r.output)
        if (r.delta)
            appendLine("meta", "[Your score changed by "
                + (r.delta > 0 ? "+" : "") + r.delta + ".]")
        showUnlocked(r.unlocked)
        root.pending = null
        root.updatePendingMinutes()
    }

    function endedLine(how) {
        if (how === "won")
            return "[The story has ended in victory. Type MENU for the picker.]"
        if (how === "died")
            return "[The story has ended. Type MENU for the picker.]"
        if (how === "over")
            return "[This playthrough is finished. Type MENU for the picker.]"
        return "[The story has ended. Your progress is saved — resume any time.]"
    }

    function handleTurn(msg) {
        if (msg.transcript) rebuildTranscript(msg.transcript)
        if (msg.reveal) showReveal(msg.reveal)
        appendLine("out", msg.output)
        applyStatus(msg.status)
        showUnlocked(msg.unlocked)
        setPending(msg.pending || null)
    }

    function handleMessage(data) {
        var msg
        try { msg = JSON.parse(data) } catch (e) {
            console.warn("omazork: unparseable backend line: " + data)
            return
        }
        switch (msg.type) {
        case "picker":
            root.games = msg.games || []
            if (root.pickerIndex >= root.games.length) root.pickerIndex = 0
            break
        case "output":
            if (root.awaitingEnter) {
                root.awaitingEnter = false
                root.screen = "console"
                root.pickerStage = "list"
                root.send({ type: "opened" }) // start the playtime clock
            }
            handleTurn(msg)
            break
        case "ended":
            handleTurn(msg)
            appendLine("meta", endedLine(msg.ended))
            if (msg.ended === "won" || msg.ended === "died") root.finished = true
            break
        case "withheld":
            appendLine("out", msg.output)
            applyStatus(msg.status) // pre-turn values, per the #14 rule
            setPending(msg.pending)
            break
        case "blocked":
            appendLine("meta", msg.output
                || "The outcome of your last action is still unfolding...")
            if (msg.pending) setPending(msg.pending)
            break
        case "checkpoint":
            appendLine("out", msg.output)
            applyStatus(msg.status)
            showUnlocked(msg.unlocked)
            root.checkpoints = root.checkpoints.concat([{
                id: "", room: root.room, score: root.score,
                createdAt: new Date().toISOString()
            }])
            break
        case "checkpoints":
            if (msg.checkpoints && msg.checkpoints.length > 0) {
                root.checkpoints = msg.checkpoints
                root.restoreChoices = msg.checkpoints
                var lines = ["Restore which checkpoint? (enter a number; anything else cancels)"]
                for (var i = 0; i < msg.checkpoints.length; i++) {
                    var c = msg.checkpoints[i]
                    lines.push("  " + (i + 1) + ". " + c.room + " — score " + c.score
                        + " (" + fmtTs(c.createdAt) + ")")
                }
                appendLine("meta", lines.join("\n"))
            } else {
                appendLine("meta", msg.output || "There are no saved checkpoints yet.")
            }
            break
        case "confirm-replace":
            root.awaitingEnter = false
            root.pickerStage = "confirm"
            break
        case "matured":
            Quickshell.execDetached(["notify-send", "-a", "omazork", "omazork",
                "Something has happened in the Great Underground Empire."])
            if (root.opened && root.screen === "console")
                root.send({ type: "opened" }) // reveal now: the console is open
            break
        case "error":
            appendLine("meta", "[error: " + msg.message + "]")
            break
        case "closed":
        case "quiet":
        case "stats":
        case "achievements":
            break
        }
    }

    // ---- player input ----
    function submit(raw) {
        var text = raw.trim()
        if (text === "") return
        var lower = text.toLowerCase()
        if (lower === "menu" || lower === "games") { toPicker(); return }
        if (root.restoreChoices) {
            var choices = root.restoreChoices
            root.restoreChoices = null
            var n = parseInt(text, 10)
            if (!isNaN(n) && String(n) === text && n >= 1 && n <= choices.length) {
                appendLine("cmd", "> restore " + n)
                root.send({ type: "restore-checkpoint", checkpoint: choices[n - 1].id })
                return
            }
            appendLine("meta", "(restore cancelled)")
        }
        appendLine("cmd", "> " + text)
        root.send({ type: "input", text: text })
    }

    // ---- picker flow ----
    function pickerEntry(i) {
        return (root.games && i >= 0 && i < root.games.length) ? root.games[i] : null
    }

    function activatePicker(i) {
        var e = pickerEntry(i)
        if (!e) return
        root.pickerIndex = i
        if (e.playthrough && !e.playthrough.finished) resumeGame(e)
        else newGameFlow(i)
    }

    function newGameFlow(i) {
        if (!pickerEntry(i)) return
        root.pickerIndex = i
        root.pickerGameIndex = i
        root.pickerMode = "casual"
        root.pickerStage = "mode"
    }

    function resumeGame(e) {
        enterGame(e.game, e.title, e.playthrough ? e.playthrough.mode : "")
        root.send({ type: "resume", game: e.game })
    }

    function confirmMode() {
        var e = pickerEntry(root.pickerGameIndex)
        if (!e) return
        enterGame(e.game, e.title, root.pickerMode)
        root.send({ type: "new", game: e.game, mode: root.pickerMode })
    }

    function confirmReplace(yes) {
        if (!yes) { root.pickerStage = "list"; return }
        var e = pickerEntry(root.pickerGameIndex)
        if (!e) return
        enterGame(e.game, e.title, root.pickerMode)
        root.send({ type: "new", game: e.game, mode: root.pickerMode, replace: true })
    }

    function enterGame(game, title, mode) {
        root.currentGame = game
        root.currentTitle = title
        root.mode = mode
        root.finished = false
        root.pending = null
        root.checkpoints = []
        root.restoreChoices = null
        transcript.clear()
        root.awaitingEnter = true
    }

    function toPicker() {
        root.screen = "picker"
        root.pickerStage = "list"
        root.restoreChoices = null
        root.send({ type: "picker" })
    }

    // ---- focus ----
    onOpenedChanged: Qt.callLater(root.syncFocus)
    onScreenChanged: Qt.callLater(root.syncFocus)
    onEngineStateChanged: Qt.callLater(root.syncFocus)
    onPickerStageChanged: Qt.callLater(root.syncFocus)

    function syncFocus() {
        if (!root.opened) return
        if (root.engineState !== "ready") engineStatus.forceActiveFocus()
        else if (root.screen === "console") consoleView.focusInput()
        else pickerView.forceActiveFocus()
    }

    Timer { id: closeGrace; interval: 280 }

    // ---- the window: transparent full-screen layer, drawer slides in ----
    PanelWindow {
        id: panel
        visible: root.opened || closeGrace.running
        anchors { top: true; bottom: true; left: true; right: true }
        color: "transparent"
        exclusionMode: ExclusionMode.Ignore
        WlrLayershell.namespace: "omazork"
        WlrLayershell.layer: WlrLayer.Overlay
        WlrLayershell.keyboardFocus: root.opened
            ? WlrKeyboardFocus.Exclusive : WlrKeyboardFocus.None

        // click outside the drawer closes
        MouseArea {
            anchors.fill: parent
            onClicked: root.requestClose()
        }

        Rectangle {
            id: drawer
            width: parent.width
            height: Math.max(380, Math.round(panel.height * 0.58))
            y: root.opened ? 0 : -height
            Behavior on y {
                NumberAnimation { duration: 220; easing.type: Easing.OutCubic }
            }
            gradient: Gradient {
                GradientStop { position: 0.0; color: consoleTheme.drawerTop }
                GradientStop { position: 1.0; color: consoleTheme.drawerBottom }
            }
            border.color: consoleTheme.borderMid
            border.width: 0

            Rectangle { // bottom edge line
                anchors.bottom: parent.bottom
                width: parent.width; height: 1
                color: consoleTheme.borderMid
            }

            MouseArea { anchors.fill: parent } // swallow clicks inside the drawer

            FocusScope {
                id: engineStatus
                anchors.fill: parent
                visible: root.engineState !== "ready"

                Keys.onPressed: event => {
                    if (event.key === Qt.Key_Escape) { root.requestClose(); event.accepted = true }
                    else if (event.key === Qt.Key_R && root.engineState === "error") {
                        root.retryBootstrap(); event.accepted = true
                    }
                }

                Column {
                    anchors.centerIn: parent
                    spacing: 18
                    width: Math.min(640, parent.width - 80)

                    Text {
                        anchors.horizontalCenter: parent.horizontalCenter
                        text: "O M A Z O R K"
                        color: consoleTheme.textBright
                        font { family: consoleTheme.mono; pixelSize: 15; bold: true; letterSpacing: 6 }
                    }
                    Text {
                        anchors.horizontalCenter: parent.horizontalCenter
                        visible: root.engineState === "bootstrapping"
                        text: "installing engine…"
                        color: consoleTheme.textDim
                        font { family: consoleTheme.mono; pixelSize: 13 }
                    }
                    Text {
                        width: parent.width
                        visible: root.engineState === "error"
                        text: root.engineError + "\n\npress R to retry · Esc to close"
                        color: consoleTheme.amber
                        wrapMode: Text.Wrap
                        horizontalAlignment: Text.AlignHCenter
                        font { family: consoleTheme.mono; pixelSize: 12 }
                    }
                }
            }

            PickerView {
                id: pickerView
                anchors.fill: parent
                visible: root.engineState === "ready" && root.screen === "picker"
                app: root
                theme: consoleTheme
            }

            ConsoleView {
                id: consoleView
                anchors.fill: parent
                visible: root.engineState === "ready" && root.screen === "console"
                app: root
                theme: consoleTheme
            }
        }
    }
}
