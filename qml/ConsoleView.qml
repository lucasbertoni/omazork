// The console screen: variant D of the UI prototype (#8) — variant A's
// phosphor drawer (header, transcript, input line) plus variant C's journal
// sidebar on the right.
import QtQuick

FocusScope {
    id: view
    required property var app
    required property var theme

    function focusInput() { cmdInput.forceActiveFocus() }

    readonly property bool pendingUnmatured:
        view.app.pending !== null && view.app.pendingMinutes > 0

    Keys.onEscapePressed: view.app.requestClose()

    Row {
        anchors.fill: parent

        Column {
            id: main
            width: parent.width - journal.width
            height: parent.height

            // header: room name left, mode hint right
            Item {
                width: parent.width
                height: 34

                Text {
                    anchors { left: parent.left; leftMargin: 22; verticalCenter: parent.verticalCenter }
                    text: view.app.room
                    color: view.theme.textBright
                    font { family: view.theme.mono; pixelSize: 12; bold: true; letterSpacing: 0.5 }
                }
                Text {
                    anchors { right: parent.right; rightMargin: 22; verticalCenter: parent.verticalCenter }
                    text: "score " + view.app.score + "   moves " + view.app.moves + "   " + view.app.mode
                    color: view.theme.textDim
                    font { family: view.theme.mono; pixelSize: 11 }
                }
                Rectangle {
                    anchors.bottom: parent.bottom
                    width: parent.width; height: 1
                    color: view.theme.borderDim
                }
            }

            ListView {
                id: transcriptList
                width: parent.width
                height: parent.height - 34 - pendingBanner.height - inputRow.height
                clip: true
                spacing: 9
                topMargin: 14
                bottomMargin: 14
                leftMargin: 22
                rightMargin: 22
                model: view.app.transcriptModel
                onCountChanged: Qt.callLater(function () {
                    transcriptList.positionViewAtEnd()
                })

                delegate: Text {
                    width: transcriptList.width - transcriptList.leftMargin - transcriptList.rightMargin
                    text: model.text
                    textFormat: Text.PlainText
                    wrapMode: Text.Wrap
                    lineHeight: 1.35
                    color: model.kind === "meta" ? view.theme.textDim : view.theme.text
                    opacity: model.kind === "cmd" ? 0.75 : 1
                    font {
                        family: view.theme.mono
                        pixelSize: 14
                        italic: model.kind === "meta"
                    }
                }
            }

            Rectangle {
                id: pendingBanner
                visible: view.app.pending !== null
                width: parent.width - 44
                anchors.horizontalCenter: parent.horizontalCenter
                height: visible ? bannerColumn.implicitHeight + 18 : 0
                color: "transparent"
                border.color: view.theme.amberBorder
                border.width: 1

                // Wait narration + remaining time, over the progress bar (#43).
                // The narration is the wrapper's; this side only frames it.
                Column {
                    id: bannerColumn
                    anchors { fill: parent; margins: 9; leftMargin: 14; rightMargin: 14 }
                    spacing: 7

                    Text {
                        id: bannerText
                        width: parent.width
                        text: !view.pendingUnmatured
                            ? "⏳ The outcome has matured — your next command reveals it."
                            : view.app.pendingQuiet
                            ? "⏳ " + view.app.pendingNarration + " — " + view.app.pendingWhen
                            : "⏳ " + view.app.pendingNarration + " — resolves "
                              + (view.app.pendingHourPlus ? "at " : "in ") + view.app.pendingWhen
                              + ". No peeking."
                        color: view.theme.amber
                        wrapMode: Text.Wrap
                        font { family: view.theme.mono; pixelSize: 12 }
                    }
                    WaitBar {
                        width: parent.width
                        theme: view.theme
                        progress: view.app.pendingProgress
                    }
                }
            }

            Item {
                id: inputRow
                width: parent.width
                height: 42

                Rectangle {
                    anchors.top: parent.top
                    width: parent.width; height: 1
                    color: view.theme.borderDim
                }
                Row {
                    anchors {
                        fill: parent
                        leftMargin: 22; rightMargin: 22
                        topMargin: 10; bottomMargin: 10
                    }
                    spacing: 8
                    opacity: view.pendingUnmatured ? 0.45 : 1

                    Text {
                        text: ">"
                        color: view.theme.textDim
                        font { family: view.theme.mono; pixelSize: 14 }
                        anchors.verticalCenter: parent.verticalCenter
                    }
                    TextInput {
                        id: cmdInput
                        width: parent.width - 20
                        anchors.verticalCenter: parent.verticalCenter
                        focus: true
                        color: view.theme.text
                        selectionColor: view.theme.borderMid
                        font { family: view.theme.mono; pixelSize: 14 }
                        onAccepted: {
                            view.app.submit(text)
                            text = ""
                        }

                        Text {
                            anchors.fill: parent
                            visible: cmdInput.text === ""
                            text: !view.pendingUnmatured ? ""
                                : view.app.pendingQuiet ? "time is passing — MENU still works"
                                : "an outcome is unfolding — MENU still works"
                            color: view.theme.textDim
                            font: cmdInput.font
                        }
                    }
                }
            }
        }

        Journal {
            id: journal
            width: 240
            height: parent.height
            app: view.app
            theme: view.theme
        }
    }
}
