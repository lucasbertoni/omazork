import QtQuick

// The console palette. By default it adopts the active omarchy theme: every
// role derives from the host shell's Color/Style singletons (qs.Commons),
// which the shell hot-reloads on `omarchy theme set` (docs/adr/0001).
// With `"theme": "phosphor"` on the shell.json plugin entry — or outside the
// omarchy-shell host, e.g. a bare `qs` dev run, where qs.Commons does not
// resolve — it falls back to the committed phosphor palette from the winning
// console prototype (variant D, #8).
QtObject {
    id: theme

    // Host contract, handed down by Main: the injected shell object (null in
    // a bare `qs` dev run) and this plugin's manifest id.
    property var shell: null
    property string pluginId: "omazork"

    // "omarchy" (default) follows the active omarchy theme; "phosphor" is
    // the opt-out, set on this plugin's shell.json entry. shellConfig is
    // reactive, so edits to shell.json apply live.
    readonly property string mode: {
        var cfg = shell ? shell.shellConfig : null
        var entries = cfg && cfg.plugins ? cfg.plugins : []
        for (var i = 0; i < entries.length; i++)
            if (entries[i] && entries[i].id === pluginId && entries[i].theme === "phosphor")
                return "phosphor"
        return "omarchy"
    }

    // { colors: Color, style: Style } from qs.Commons, or null outside the
    // host. Resolved at runtime so this file still loads without omarchy.
    property var host: null
    Component.onCompleted: {
        try {
            host = Qt.createQmlObject(
                "import QtQuick\nimport qs.Commons\n"
                + "QtObject { readonly property var colors: Color; "
                + "readonly property var style: Style }",
                theme, "omarchyHost")
        } catch (e) {
            host = null
            console.warn("omazork theme: qs.Commons unavailable, phosphor fallback: " + e)
        }
    }

    // The adopted omarchy palette, or null when opted out / outside the host.
    readonly property var adopted: mode !== "phosphor" && host ? host.colors : null

    // Color exposes no light/dark flag, so derivations flip around the theme
    // background's lightness: light themes yield a genuinely light console.
    // Backgrounds deepen toward bgExtreme (away from the text); text gains
    // emphasis toward fgExtreme.
    readonly property bool isLight: adopted ? adopted.background.hslLightness > 0.5 : false
    readonly property color bgExtreme: isLight ? "#ffffff" : "#000000"
    readonly property color fgExtreme: isLight ? "#000000" : "#ffffff"

    function mix(a, b, t) {
        return Qt.rgba(a.r + (b.r - a.r) * t,
                       a.g + (b.g - a.g) * t,
                       a.b + (b.b - a.b) * t, 1)
    }

    readonly property color drawerTop: adopted ? adopted.background : "#0b120c"
    readonly property color drawerBottom: adopted ? mix(adopted.background, bgExtreme, 0.12) : "#0a0f0a"
    readonly property color borderDim: adopted ? mix(adopted.background, adopted.foreground, 0.15) : "#1e3320"
    readonly property color borderMid: adopted ? mix(adopted.background, adopted.foreground, 0.28) : "#2c4a2e"
    readonly property color selBg: adopted
        ? Qt.rgba(adopted.accent.r, adopted.accent.g, adopted.accent.b, 0.22) : "#143c7840"

    readonly property color text: adopted ? adopted.foreground : "#9fdf9f"
    readonly property color textBright: adopted ? mix(adopted.foreground, fgExtreme, 0.35) : "#cdeccd"
    readonly property color textDim: adopted ? adopted.muted : "#5f8f62"

    readonly property color journalBg: adopted ? mix(adopted.background, bgExtreme, 0.35) : "#070c08"
    readonly property color journalText: adopted ? mix(adopted.foreground, adopted.background, 0.22) : "#82b085"
    readonly property color journalRule: adopted ? mix(adopted.background, adopted.foreground, 0.10) : "#142418"
    readonly property color checkpointText: adopted ? mix(adopted.muted, adopted.foreground, 0.25) : "#6f9a72"
    readonly property color checkpointTs: adopted ? mix(adopted.muted, adopted.background, 0.35) : "#4a6b4a"

    readonly property color amber: adopted ? adopted.accent : "#e8b04a"
    readonly property color amberBorder: adopted ? mix(adopted.accent, adopted.background, 0.45) : "#7a6a30"
    readonly property color amberBg: adopted ? mix(adopted.background, adopted.accent, 0.15) : "#141004"

    readonly property string mono: adopted && host.style ? host.style.fontFamily : "IBM Plex Mono"
}
