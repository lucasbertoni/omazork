// omazork bar icon: the brass lantern. Lit while the wrapper runs, dark when
// it is not, with a hollow dot while a Casual wait unfolds and a filled accent
// dot once the matured outcome waits in the Console (docs/action-waits.md §8).
// Click toggles the Console. All state comes off the service singleton
// (docs/adr/0002) as live bindings — no polling, no files.
import QtQuick
import qs.Ui
import qs.Commons

BarWidget {
    id: root
    moduleName: "omazork"

    readonly property var svc: bar && bar.shell ? bar.shell.serviceFor("omazork") : null
    readonly property bool alive: svc ? svc.backendAlive : false
    readonly property bool recapWaiting: svc ? svc.matureRecapWaiting : false
    // in flight: a wait is unfolding and has not matured, either tier
    readonly property bool inFlight: svc ? svc.pendingMaturesAt !== null && !recapWaiting : false

    // Evaluated on hover, since Date.now() is not a binding dependency.
    function pendingWhen() {
        var min = Math.max(0, Math.ceil((svc.pendingMaturesAt.getTime() - Date.now()) / 60000))
        return (svc.pendingHourPlus(min) ? "matures at " : "") + svc.pendingWhen(min)
    }

    // Deliberately spoiler-free: game and mode, never room, score, or what
    // the waiting recap says — the reveal belongs to the Console.
    function tooltipText() {
        var t = "Omazork"
        if (svc && svc.currentTitle !== "")
            t += " — " + svc.currentTitle + " · " + (svc.mode === "casual" ? "Casual" : "Classic")
        if (!alive) t += " · engine not running"
        if (recapWaiting) t += " · recap waiting"
        else if (inFlight) t += (svc.pendingTier === "quiet" ? " · time passes · " : " · unfolding · ") + pendingWhen()
        return t
    }

    implicitWidth: vertical ? barSize : glyph.implicitWidth + Style.space(14)
    implicitHeight: vertical ? glyph.implicitHeight + Style.space(14) : barSize

    Text {
        id: glyph
        anchors.centerIn: parent
        text: "󱀠" // nf-md-coach_lamp (U+F1020): the closest thing to a brass lantern
        color: root.bar
            ? (root.alive ? root.bar.barForeground : Qt.darker(root.bar.barForeground, 1.8))
            : "white"
        font.family: root.bar ? root.bar.fontFamily : "monospace"
        font.pixelSize: Style.font.body
        Behavior on color {
            enabled: !root.bar || root.bar.foregroundAnimationEnabled
            ColorAnimation { duration: 160 }
        }
    }

    Rectangle {
        // recap-waiting dot; deliberately not the urgent color — a matured
        // outcome is calm news, per the whole mediation-layer ethos. While the
        // wait is still unfolding the dot is hollow and dim: something is
        // happening, nothing is owed yet.
        visible: root.recapWaiting || root.inFlight
        width: 5
        height: 5
        radius: 2.5
        color: root.recapWaiting ? Color.accent : "transparent"
        border.width: root.recapWaiting ? 0 : 1
        border.color: root.bar ? Qt.darker(root.bar.barForeground, 1.6) : "gray"
        anchors.left: glyph.right
        anchors.top: glyph.top
        anchors.leftMargin: -1
        anchors.topMargin: -1
    }

    MouseArea {
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: if (root.bar) root.bar.run("omarchy-shell shell toggle omazork")
        onEntered: if (root.bar) root.bar.showTooltip(root, root.tooltipText())
        onExited: if (root.bar) root.bar.hideTooltip(root)
    }
}
