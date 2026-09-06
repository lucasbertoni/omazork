// The console screen: variant D of the UI prototype (#8) — variant A's
// phosphor drawer (header, transcript, input line) plus variant C's journal
// sidebar on the right.
import QtQuick

FocusScope {
    id: view
    required property var app
    required property var theme

    // Blocked input (CONTEXT.md): while a pending outcome is unmatured the
    // input row gives way to the wait row and the view itself holds focus so
    // the single-key shortcuts (M menu, Esc close) work. The wrapper's
    // blocked rule is unchanged; this side just stops inviting the typing.
    function focusInput() {
        if (!view.visible) return // never steal focus from the picker
        if (view.pendingUnmatured) view.forceActiveFocus()
        else cmdInput.forceActiveFocus()
    }

    readonly property bool pendingUnmatured: view.app.pendingUnmatured
    readonly property bool pendingMatured: view.app.pendingMatured

    onPendingUnmaturedChanged: Qt.callLater(view.focusInput)

    Keys.onEscapePressed: view.app.requestClose()
    Keys.onPressed: event => {
        if (!view.pendingUnmatured) return
        if (event.key === Qt.Key_M) { view.app.toPicker(); event.accepted = true }
    }

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
                height: parent.height - 34 - maturedBanner.height - bottomRow.height
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
                id: maturedBanner
                visible: view.pendingMatured
                width: parent.width - 44
                anchors.horizontalCenter: parent.horizontalCenter
                height: visible ? maturedText.implicitHeight + 18 : 0
                color: "transparent"
                border.color: view.theme.amberBorder
                border.width: 1

                Text {
                    id: maturedText
                    anchors { fill: parent; margins: 9; leftMargin: 14; rightMargin: 14 }
                    text: "⏳ The outcome has matured — your next command reveals it."
                    color: view.theme.amber
                    wrapMode: Text.Wrap
                    font { family: view.theme.mono; pixelSize: 12 }
                }
            }

            // Bottom row: the wait row during blocked input, the input row
            // otherwise. Only one is ever visible.
            Item {
                id: bottomRow
                width: parent.width
                height: view.pendingUnmatured ? waitRow.implicitHeight + 20 : 42

                Rectangle {
                    anchors.top: parent.top
                    width: parent.width; height: 1
                    color: view.theme.borderDim
                }

                // Wait row (#43 narration + progress bar): what the player is
                // doing and how long it has left. The narration is the
                // wrapper's; this side only frames it.
                Column {
                    id: waitRow
                    visible: view.pendingUnmatured
                    anchors {
                        left: parent.left; right: parent.right
                        leftMargin: 22; rightMargin: 22
                        verticalCenter: parent.verticalCenter
                    }
                    spacing: 7

                    Text {
                        width: parent.width
                        text: view.app.pendingQuiet
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

                Row {
                    visible: !view.pendingUnmatured
                    anchors {
                        fill: parent
                        leftMargin: 22; rightMargin: 22
                        topMargin: 10; bottomMargin: 10
                    }
                    spacing: 8

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
                        // Hidden items keep scope focus, so release it while
                        // blocked or the M shortcut never reaches the view.
                        focus: !view.pendingUnmatured
                        color: view.theme.text
                        selectionColor: view.theme.borderMid
                        font { family: view.theme.mono; pixelSize: 14 }
                        onAccepted: {
                            view.app.submit(text)
                            text = ""
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
