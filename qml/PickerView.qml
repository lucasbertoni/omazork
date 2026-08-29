// The in-console picker (variant A's keyboard-era menu, #8): the three games
// with resume summaries, mode chosen per new game (#5), replace confirmed
// through the backend's confirm-replace round trip (docs/protocol.md).
import QtQuick

FocusScope {
    id: view
    required property var app
    required property var theme

    Keys.onPressed: event => {
        if (event.key === Qt.Key_Escape) {
            if (view.app.pickerStage === "list") view.app.requestClose()
            else view.app.pickerStage = "list"
            event.accepted = true
            return
        }
        if (view.app.pickerStage === "list") {
            switch (event.key) {
            case Qt.Key_Up:
            case Qt.Key_K:
                view.app.pickerIndex = Math.max(0, view.app.pickerIndex - 1); break
            case Qt.Key_Down:
            case Qt.Key_J:
                view.app.pickerIndex = Math.min(view.app.games.length - 1, view.app.pickerIndex + 1); break
            case Qt.Key_1: view.app.activatePicker(0); break
            case Qt.Key_2: view.app.activatePicker(1); break
            case Qt.Key_3: view.app.activatePicker(2); break
            case Qt.Key_Return:
            case Qt.Key_Enter:
                view.app.activatePicker(view.app.pickerIndex); break
            case Qt.Key_N:
                view.app.newGameFlow(view.app.pickerIndex); break
            default:
                return
            }
            event.accepted = true
        } else if (view.app.pickerStage === "mode") {
            switch (event.key) {
            case Qt.Key_Left:
            case Qt.Key_Right:
                view.app.pickerMode = view.app.pickerMode === "casual" ? "classic" : "casual"; break
            case Qt.Key_C:
                view.app.pickerMode = "classic"; break
            case Qt.Key_A:
                view.app.pickerMode = "casual"; break
            case Qt.Key_Return:
            case Qt.Key_Enter:
                view.app.confirmMode(); break
            default:
                return
            }
            event.accepted = true
        } else if (view.app.pickerStage === "confirm") {
            if (event.key === Qt.Key_Y) { view.app.confirmReplace(true); event.accepted = true }
            else if (event.key === Qt.Key_N || event.key === Qt.Key_Return
                     || event.key === Qt.Key_Enter) {
                view.app.confirmReplace(false); event.accepted = true
            }
        }
    }

    function subLine(entry) {
        var pt = entry.playthrough
        if (!pt) return "new game"
        if (pt.finished) return "finished — N for a new game"
        var s = "resume — " + pt.room + ", " + pt.score + " pts, " + pt.mode
        if (pt.pending) s += "  ● outcome pending"
        return s
    }

    Column {
        anchors { top: parent.top; topMargin: 44; horizontalCenter: parent.horizontalCenter }
        spacing: 4
        width: Math.min(620, parent.width - 80)

        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: "U N D E R G R O U N D"
            color: view.theme.textBright
            font { family: view.theme.mono; pixelSize: 15; bold: true; letterSpacing: 6 }
        }
        Item { width: 1; height: 24 }

        Repeater {
            model: view.app.games
            delegate: Rectangle {
                width: parent.width
                height: 42
                color: index === view.app.pickerIndex && view.app.pickerStage === "list"
                    ? view.theme.selBg : "transparent"
                border.color: index === view.app.pickerIndex && view.app.pickerStage === "list"
                    ? view.theme.borderMid : "transparent"
                border.width: 1

                MouseArea {
                    anchors.fill: parent
                    onClicked: view.app.activatePicker(index)
                }
                Text {
                    anchors { left: parent.left; leftMargin: 16; verticalCenter: parent.verticalCenter }
                    text: (index + 1) + ".  " + modelData.title
                    color: view.theme.textBright
                    font { family: view.theme.mono; pixelSize: 14; bold: true }
                }
                Text {
                    anchors { right: parent.right; rightMargin: 16; verticalCenter: parent.verticalCenter }
                    width: parent.width * 0.62
                    horizontalAlignment: Text.AlignRight
                    elide: Text.ElideLeft
                    text: view.subLine(modelData)
                    color: modelData.playthrough && modelData.playthrough.pending
                        ? view.theme.amber : view.theme.textDim
                    font { family: view.theme.mono; pixelSize: 12 }
                }
            }
        }

        Item { width: 1; height: 26 }

        Text {
            visible: view.app.pickerStage === "list"
            anchors.horizontalCenter: parent.horizontalCenter
            text: "Enter resumes · N starts a new game · mode is chosen per new game"
            color: view.theme.textDim
            font { family: view.theme.mono; pixelSize: 12 }
        }

        Column {
            visible: view.app.pickerStage === "mode"
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: 8

            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: "New game — mode?"
                color: view.theme.textBright
                font { family: view.theme.mono; pixelSize: 13; bold: true }
            }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: 10
                Repeater {
                    model: ["casual", "classic"]
                    delegate: Rectangle {
                        width: 110; height: 30
                        color: view.app.pickerMode === modelData ? view.theme.selBg : "transparent"
                        border.color: view.app.pickerMode === modelData
                            ? view.theme.borderMid : view.theme.borderDim
                        border.width: 1
                        MouseArea {
                            anchors.fill: parent
                            onClicked: { view.app.pickerMode = modelData; view.app.confirmMode() }
                        }
                        Text {
                            anchors.centerIn: parent
                            text: modelData
                            color: view.app.pickerMode === modelData
                                ? view.theme.textBright : view.theme.textDim
                            font { family: view.theme.mono; pixelSize: 13 }
                        }
                    }
                }
            }
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: view.app.pickerMode === "casual"
                    ? "outcomes of scoring moves take real time to unfold"
                    : "the game exactly as written — nothing waits"
                color: view.theme.textDim
                font { family: view.theme.mono; pixelSize: 11 }
            }
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: "←/→ switches · Enter starts · Esc back"
                color: view.theme.checkpointTs
                font { family: view.theme.mono; pixelSize: 11 }
            }
        }

        Column {
            visible: view.app.pickerStage === "confirm"
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: 8

            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: "A playthrough already exists — starting over erases it."
                color: view.theme.amber
                font { family: view.theme.mono; pixelSize: 13 }
            }
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: "Y replaces it · N keeps it"
                color: view.theme.textDim
                font { family: view.theme.mono; pixelSize: 12 }
            }
        }
    }
}
