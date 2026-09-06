// Console height (CONTEXT.md): the share of the screen the Console occupies,
// 20..100 percent, default 25, stepped in fives by Console shortcuts. The
// value is local state written through to this plugin's shell.json entry as
// `"height"` (a preference, like theme), so it survives shell restarts and a
// hand edit of the file applies live. Fullscreen is a bookmark: Ctrl+F jumps
// to 100 and remembers where it left; pressing again returns there, and any
// ordinary step while fullscreen forgets the bookmark. The bookmark itself
// is never persisted — after a restart at 100, Ctrl+F returns to the default.
import QtQuick
import Quickshell
import "PluginEntry.js" as PluginEntry

QtObject {
    id: consoleHeight

    property var shell: null
    property string pluginId: "omazork"

    readonly property int minimum: 20
    readonly property int maximum: 100
    readonly property int fallback: 25
    readonly property int step: 5

    property int percent: fallback
    // where fullscreen will return to; 0 when there is nothing to return to
    property int returnTo: 0
    readonly property bool fullscreen: percent === maximum

    // A hand-edited value is kept exactly when in range (27 stays 27), out
    // of range clamps, and anything non-numeric falls back to the default.
    function sanitize(v) {
        var n = typeof v === "number" ? v
              : typeof v === "string" && v.trim() !== "" ? Number(v) : NaN
        if (isNaN(n) || !isFinite(n)) return fallback
        return Math.min(maximum, Math.max(minimum, Math.round(n)))
    }

    function configured() {
        return sanitize(PluginEntry.setting(shell ? shell.shellConfig : null, pluginId, "height"))
    }

    // Steps land on the grid of fives after one press: 27 → 30, not 32.
    function expand() { stepTo(Math.min(maximum, Math.floor(percent / step) * step + step)) }
    function contract() { stepTo(Math.max(minimum, Math.ceil(percent / step) * step - step)) }

    function toggleFullscreen() {
        if (fullscreen) { stepTo(returnTo > 0 ? returnTo : fallback); return }
        var from = percent
        if (apply(maximum)) returnTo = from
    }

    // An ordinary height change forgets the fullscreen bookmark; a step that
    // changes nothing (expand at 100) also forgets nothing.
    function stepTo(value) {
        if (apply(value)) returnTo = 0
    }

    // Moves the drawer now and schedules the write; false when unchanged.
    function apply(value) {
        var v = sanitize(value)
        if (v === percent) return false
        percent = v
        if (shell) writer.restart() // bare `qs` dev run: session-only
        return true
    }

    // Write-through via the shell's CLI, which only knows entries in
    // bar.layout; an entry still in `plugins` does not persist (README).
    // Debounced so a run of presses becomes one write and the processes
    // cannot race each other to the file.
    property Timer writer: Timer {
        interval: 250
        onTriggered: {
            consoleHeight.settling.restart()
            Quickshell.execDetached(["omarchy", "bar", "set", consoleHeight.pluginId,
                "height", String(consoleHeight.percent), "--json"])
        }
    }

    // shellConfig is reactive: adopt external edits, but not the echo of our
    // own writes still landing (two quick presses would otherwise flicker).
    property Timer settling: Timer { interval: 1500 }
    property Connections config: Connections {
        target: consoleHeight.shell
        ignoreUnknownSignals: true
        function onShellConfigChanged() {
            if (consoleHeight.writer.running || consoleHeight.settling.running) return
            var v = consoleHeight.configured()
            if (v !== consoleHeight.percent) { consoleHeight.percent = v; consoleHeight.returnTo = 0 }
        }
    }

    onShellChanged: percent = configured()
    Component.onCompleted: percent = configured()
}
