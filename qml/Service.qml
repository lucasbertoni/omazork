// omazork service entry point (docs/adr/0002). Owns the Go wrapper and the
// NDJSON plumbing so wrapper state outlives the Console: the overlay gets
// this instance injected as `service`, the bar icon reaches it through
// bar.shell.serviceFor("omazork"), and both read it via live bindings.
import QtQuick
import Quickshell
import Quickshell.Io

Item {
    id: root

    // ---- host contract ----
    property var shell: null
    property var manifest: null

    readonly property string pluginRoot: {
        var u = Qt.resolvedUrl("..").toString()
        if (u.indexOf("file://") === 0) u = u.substring(7)
        while (u.length > 1 && u.charAt(u.length - 1) === "/")
            u = u.substring(0, u.length - 1)
        return u
    }

    // ---- session identity, pushed by the Console (pushSession in Main.qml) ----
    property string currentGame: ""
    property string currentTitle: ""
    property string mode: ""
    property bool consoleActive: false // the Console shows the game screen, not the picker

    // ---- wrapper-driven state the bar icon binds to ----
    readonly property bool backendAlive: backend.running
    property bool matureRecapWaiting: false
    property var pendingMaturesAt: null

    signal message(var msg) // every parsed wrapper line, after service bookkeeping
    signal backendStarted()

    // ---- engine bootstrap (#12: idempotent, run on every launch) ----
    property string engineState: "bootstrapping" // bootstrapping | ready | error
    property string engineError: ""

    // Services are instantiated synchronously for every enabled plugin at
    // shell startup, so the spawn is deferred off that path (docs/adr/0002).
    Component.onCompleted: Qt.callLater(function() { bootstrap.running = true })

    Process {
        id: bootstrap
        command: ["bash", root.pluginRoot + "/scripts/bootstrap.sh"]
        workingDirectory: root.pluginRoot
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
        root.backendFailures = 0
        bootstrap.running = true
    }

    // ---- wrapper child: NDJSON over stdio ----
    Process {
        id: backend
        command: [root.pluginRoot + "/bin/omazork"]
        stdinEnabled: true
        stdout: SplitParser {
            onRead: data => root.handleLine(data)
        }
        stderr: SplitParser {
            onRead: data => console.warn("omazork backend: " + data)
        }
        onStarted: {
            root.backendStartedAt = Date.now()
            // Durability lives in the backend: after a respawn (plugin
            // rescan, crash) resuming the active game is invisible to the
            // player. The Console re-sends "opened" on backendStarted().
            if (root.consoleActive && root.currentGame !== "")
                root.send({ type: "resume", game: root.currentGame })
            else
                root.send({ type: "picker" })
            root.backendStarted()
        }
        onExited: {
            if (root.engineState !== "ready") return
            // only quick deaths count as failures; a run that lived a while
            // was healthy and resets the budget
            if (Date.now() - root.backendStartedAt > 30000) root.backendFailures = 0
            root.backendFailures++
            if (root.backendFailures > 5) {
                root.engineState = "error"
                root.engineError = "the engine keeps crashing right after launch"
                    + "\n(binary: " + root.pluginRoot + "/bin/omazork)"
            } else {
                respawn.interval = 1000 * Math.pow(2, root.backendFailures - 1)
                respawn.restart()
            }
        }
    }
    property int backendFailures: 0
    property double backendStartedAt: 0
    Timer { id: respawn; interval: 1000; onTriggered: backend.running = true }

    function send(msg) {
        if (!backend.running) return
        backend.write(JSON.stringify(msg) + "\n")
    }

    function handleLine(data) {
        var msg
        try { msg = JSON.parse(data) } catch (e) {
            console.warn("omazork: unparseable backend line: " + data)
            return
        }
        // Bar-icon bookkeeping happens before the Console sees the message,
        // so a reveal clears the dot even while the Console is unloaded.
        if (msg.reveal) {
            root.matureRecapWaiting = false
            root.pendingMaturesAt = null
        }
        switch (msg.type) {
        case "output":
        case "ended":
        case "withheld":
            root.pendingMaturesAt = msg.pending ? new Date(msg.pending.maturesAt) : null
            // no pending and no reveal: the active playthrough owes nothing,
            // so a lit dot is stale (playthrough replaced, or game switched)
            if (!msg.pending && !msg.reveal) root.matureRecapWaiting = false
            break
        case "blocked":
            if (msg.pending) root.pendingMaturesAt = new Date(msg.pending.maturesAt)
            break
        case "picker":
            root.adoptPickerPending(msg.games)
            break
        case "matured":
            root.matureRecapWaiting = true
            Quickshell.execDetached(["notify-send", "-a", "omazork", "omazork",
                "Something has happened in the Great Underground Empire."])
            break
        }
        root.message(msg)
    }

    // With no active session the wrapper cannot announce maturation, so the
    // picker payload carries each playthrough's pendingMaturesAt; adopt the
    // earliest (covers outcomes that matured across a shell restart).
    function adoptPickerPending(games) {
        var earliest = null
        for (var i = 0; games && i < games.length; i++) {
            var pt = games[i].playthrough
            if (pt && pt.pending && pt.pendingMaturesAt) {
                var at = new Date(pt.pendingMaturesAt)
                if (!earliest || at < earliest) earliest = at
            }
        }
        root.pendingMaturesAt = earliest
        if (!earliest) root.matureRecapWaiting = false
        else if (Date.now() >= earliest.getTime()) root.matureRecapWaiting = true
    }

    // The wrapper announces maturation once per outcome; this catches a
    // pending outcome whose wait elapsed while the wrapper was not running.
    Timer {
        interval: 30000; repeat: true
        running: root.pendingMaturesAt !== null && !root.matureRecapWaiting
        onTriggered: {
            if (Date.now() >= root.pendingMaturesAt.getTime())
                root.matureRecapWaiting = true
        }
    }
}
