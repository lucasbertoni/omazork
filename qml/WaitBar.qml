// The action-wait progress bar (docs/action-waits.md §8, #43): a thin amber
// track showing how much of a pending wait has elapsed. Out-of-game surfaces
// only — the transcript never sees it. progress < 0 means "unknown" (a save
// from before the wait's start was recorded) and hides the bar.
import QtQuick

Item {
    id: bar
    required property var theme
    property real progress: -1 // 0..1, or < 0 when unknown

    width: parent ? parent.width : 100
    height: visible ? 3 : 0
    visible: progress >= 0

    Rectangle {
        anchors.fill: parent
        radius: 1.5
        color: bar.theme.amberBorder
    }
    Rectangle {
        width: parent.width * Math.min(1, Math.max(0, bar.progress))
        height: parent.height
        radius: 1.5
        color: bar.theme.amber
        Behavior on width { NumberAnimation { duration: 400; easing.type: Easing.OutQuad } }
    }
}
