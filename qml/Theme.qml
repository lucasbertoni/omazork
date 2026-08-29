import QtQuick

// Phosphor palette from the winning console prototype (variant D, #8).
// Committed single-theme: the console is a green-on-black terminal drawer.
QtObject {
    readonly property color drawerTop: "#0b120c"
    readonly property color drawerBottom: "#0a0f0a"
    readonly property color borderDim: "#1e3320"
    readonly property color borderMid: "#2c4a2e"
    readonly property color selBg: "#143c7840"

    readonly property color text: "#9fdf9f"
    readonly property color textBright: "#cdeccd"
    readonly property color textDim: "#5f8f62"

    readonly property color journalBg: "#070c08"
    readonly property color journalText: "#82b085"
    readonly property color journalRule: "#142418"
    readonly property color checkpointText: "#6f9a72"
    readonly property color checkpointTs: "#4a6b4a"

    readonly property color amber: "#e8b04a"
    readonly property color amberBorder: "#7a6a30"
    readonly property color amberBg: "#141004"

    readonly property string mono: "IBM Plex Mono"
}
