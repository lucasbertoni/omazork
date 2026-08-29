// The journal sidebar (variant C's contribution to the winning design, #8):
// game + mode, status values, the pending-outcome card, and checkpoints.
// Status values are whatever the backend last sent — during a withheld
// outcome those are the pre-turn values, per the #14 rule, so nothing is
// hidden here.
import QtQuick

Rectangle {
    id: journal
    required property var app
    required property var theme

    color: journal.theme.journalBg

    Rectangle {
        anchors.left: parent.left
        width: 1; height: parent.height
        color: journal.theme.borderDim
    }

    Flickable {
        anchors { fill: parent; leftMargin: 18; rightMargin: 18; topMargin: 14; bottomMargin: 14 }
        contentHeight: content.height
        clip: true

        Column {
            id: content
            width: journal.width - 36
            spacing: 15

            Column {
                width: parent.width
                spacing: 2

                Text {
                    text: (journal.app.currentTitle + " · " + journal.app.mode).toUpperCase()
                    color: journal.theme.textDim
                    font { family: journal.theme.mono; pixelSize: 10; bold: true; letterSpacing: 2 }
                    elide: Text.ElideRight
                    width: parent.width
                }
                Item { width: 1; height: 5 }
                Repeater {
                    model: [
                        { k: "Room", v: journal.app.room },
                        { k: "Score", v: String(journal.app.score) },
                        { k: "Moves", v: String(journal.app.moves) }
                    ]
                    delegate: Item {
                        width: content.width
                        height: 18
                        Text {
                            anchors.left: parent.left
                            text: modelData.k
                            color: journal.theme.journalText
                            font { family: journal.theme.mono; pixelSize: 12 }
                        }
                        Text {
                            anchors.right: parent.right
                            width: parent.width - 52
                            horizontalAlignment: Text.AlignRight
                            text: modelData.v
                            elide: Text.ElideRight
                            color: journal.theme.textBright
                            font { family: journal.theme.mono; pixelSize: 12; bold: true }
                        }
                    }
                }
            }

            Rectangle {
                visible: journal.app.pending !== null
                width: parent.width
                height: pendText.implicitHeight + 20
                radius: 4
                color: journal.theme.amberBg
                border.color: journal.theme.amberBorder
                border.width: 1

                Text {
                    id: pendText
                    anchors { fill: parent; margins: 10 }
                    text: journal.app.pendingMinutes > 0
                        ? "⏳ Outcome pending\nmatures in ~" + journal.app.pendingMinutes
                          + " min\ninput paused · saves wait"
                        : "⏳ Outcome matured\nyour next command\nreveals it"
                    color: journal.theme.amber
                    wrapMode: Text.Wrap
                    lineHeight: 1.3
                    font { family: journal.theme.mono; pixelSize: 12 }
                }
            }

            Column {
                width: parent.width
                spacing: 0

                Text {
                    text: "CHECKPOINTS"
                    color: journal.theme.textDim
                    font { family: journal.theme.mono; pixelSize: 10; bold: true; letterSpacing: 2 }
                }
                Item { width: 1; height: 7 }
                Text {
                    visible: journal.app.checkpoints.length === 0
                    text: "(none yet — type SAVE)"
                    color: journal.theme.checkpointTs
                    font { family: journal.theme.mono; pixelSize: 11; italic: true }
                }
                Repeater {
                    model: journal.app.checkpoints
                    delegate: Column {
                        width: content.width
                        Rectangle {
                            width: parent.width; height: 1
                            color: journal.theme.journalRule
                        }
                        Item { width: 1; height: 5 }
                        Text {
                            width: parent.width
                            text: modelData.room + " · " + modelData.score + " pts"
                            elide: Text.ElideRight
                            color: journal.theme.checkpointText
                            font { family: journal.theme.mono; pixelSize: 12 }
                        }
                        Text {
                            text: journal.app.fmtTs(modelData.createdAt)
                            color: journal.theme.checkpointTs
                            font { family: journal.theme.mono; pixelSize: 10 }
                        }
                        Item { width: 1; height: 5 }
                    }
                }
            }

            Text {
                width: parent.width
                text: "RESTORE lists checkpoints\nMENU returns to the picker\nEsc closes the console"
                color: journal.theme.checkpointTs
                lineHeight: 1.4
                font { family: journal.theme.mono; pixelSize: 10 }
            }
        }
    }
}
