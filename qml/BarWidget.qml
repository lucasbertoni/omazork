// omazork bar icon: the brass lantern. Lit while the wrapper runs, dark when
// it is not, with a dot when a matured Casual outcome waits in the Console.
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

    // Deliberately spoiler-free: game and mode, never room, score, or what
    // the waiting recap says — the reveal belongs to the Console.
    readonly property string tooltipText: {
        var t = "Omazork"
        if (svc && svc.currentTitle !== "")
            t += " — " + svc.currentTitle + " · " + (svc.mode === "casual" ? "Casual" : "Classic")
        if (!alive) t += " · engine not running"
        if (recapWaiting) t += " · recap waiting"
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
        // outcome is calm news, per the whole mediation-layer ethos
        visible: root.recapWaiting
        width: 5
        height: 5
        radius: 2.5
        color: Color.accent
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
        onEntered: if (root.bar) root.bar.showTooltip(root, root.tooltipText)
        onExited: if (root.bar) root.bar.hideTooltip(root)
    }
}
