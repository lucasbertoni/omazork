import QtQuick
import "PluginEntry.js" as PluginEntry

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
        var t = PluginEntry.setting(shell ? shell.shellConfig : null, pluginId, "theme")
        return t === "phosphor" ? "phosphor" : "omarchy"
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

    // Legibility floor. A theme's `muted` is tuned for its own chrome, and
    // on most omarchy themes it sits at 1.5–3:1 against the background —
    // fine for a bar icon, unreadable for 10px console copy — and mixing it
    // further toward the background sank the journal's hints below 2:1.
    // lift() walks a color toward fgExtreme until it clears `min` (WCAG
    // contrast ratio) against the surface it is drawn on. Floors are set so
    // the hierarchy survives: dim < text < bright.
    function luminance(c) {
        function ch(v) { return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4) }
        return 0.2126 * ch(c.r) + 0.7152 * ch(c.g) + 0.0722 * ch(c.b)
    }
    function contrast(a, b) {
        var la = luminance(a), lb = luminance(b)
        return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
    }
    function lift(c, on, min) {
        var out = c
        for (var t = 0; t < 1 && contrast(out, on) < min; t += 0.05)
            out = mix(c, fgExtreme, t)
        return out
    }

    readonly property color drawerTop: adopted ? adopted.background : "#0b120c"
    readonly property color drawerBottom: adopted ? mix(adopted.background, bgExtreme, 0.12) : "#0a0f0a"
    readonly property color borderDim: adopted ? mix(adopted.background, adopted.foreground, 0.15) : "#1e3320"
    readonly property color borderMid: adopted ? mix(adopted.background, adopted.foreground, 0.28) : "#2c4a2e"
    readonly property color selBg: adopted
        ? Qt.rgba(adopted.accent.r, adopted.accent.g, adopted.accent.b, 0.22) : "#143c7840"

    readonly property color text: adopted ? adopted.foreground : "#9fdf9f"
    readonly property color textBright: adopted ? mix(adopted.foreground, fgExtreme, 0.35) : "#cdeccd"
    // textDim is drawn on both surfaces; drawerTop is the stricter one.
    readonly property color textDim: lift(adopted ? adopted.muted : Qt.color("#5f8f62"), drawerTop, 4.5)

    readonly property color journalBg: adopted ? mix(adopted.background, bgExtreme, 0.35) : "#070c08"
    readonly property color journalText: lift(adopted ? mix(adopted.foreground, adopted.background, 0.22) : Qt.color("#82b085"), journalBg, 4.5)
    readonly property color journalRule: adopted ? mix(adopted.background, adopted.foreground, 0.10) : "#142418"
    readonly property color checkpointText: lift(adopted ? mix(textDim, adopted.foreground, 0.25) : Qt.color("#6f9a72"), journalBg, 4.5)
    readonly property color checkpointTs: lift(adopted ? mix(textDim, adopted.background, 0.35) : Qt.color("#4a6b4a"), journalBg, 3.5)

    readonly property color amber: adopted ? adopted.accent : "#e8b04a"
    readonly property color amberBorder: adopted ? mix(adopted.accent, adopted.background, 0.45) : "#7a6a30"
    readonly property color amberBg: adopted ? mix(adopted.background, adopted.accent, 0.15) : "#141004"

    readonly property string mono: adopted && host.style ? host.style.fontFamily : "IBM Plex Mono"
}
