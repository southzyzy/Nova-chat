package main

import (
	"fmt"
	"io"
	"time"
	"os"
	"log"
	"bufio"
	"runtime"
	"strings"
	"os/exec"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ChatUI is a Text User Interface (TUI) for a ChatRoom.
// The Run method will draw the UI to the terminal in "fullscreen"
// mode. You can quit with Ctrl-C, or by typing "/quit" into the
// chat prompt.
type ChatUI struct {
	cr        *ChatRoom
	app       *tview.Application
	peersList *tview.TextView

	msgW    io.Writer
	inputCh chan string
	doneCh  chan struct{}
}

// NewChatUI returns a new ChatUI struct that controls the text UI.
// It won't actually do anything until you call Run().
func NewChatUI(cr *ChatRoom) *ChatUI {
	app := tview.NewApplication()

	// make a text view to contain our chat messages
	msgBox := tview.NewTextView()
	msgBox.SetDynamicColors(true)
	msgBox.SetBorder(true)
	msgBox.SetTitle(fmt.Sprintf("Room: %s", cr.roomName))

	// text views are io.Writers, but they don't automatically refresh.
	// this sets a change handler to force the app to redraw when we get
	// new messages to display.
	msgBox.SetChangedFunc(func() {
		app.Draw()
	})

	// an input field for typing messages into
	inputCh := make(chan string, 32)
	input := tview.NewInputField().
		SetLabel(cr.nick + " > ").
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorBlack)

	// the done func is called when the user hits enter, or tabs out of the field
	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			// we don't want to do anything if they just tabbed away
			return
		}
		line := input.GetText()
		if len(line) == 0 {
			// ignore blank lines
			return
		}

		// bail if requested
		if line == "/quit" {
			app.Stop()
			return
		}

		// send the line onto the input chan and reset the field text
		inputCh <- line
		input.SetText("")
	})

	// make a text view to hold the list of peers in the room, updated by ui.refreshPeers()
	peersList := tview.NewTextView()
	peersList.SetBorder(true)
	peersList.SetTitle("Peers")
	peersList.SetChangedFunc(func() { app.Draw() })

	// chatPanel is a horizontal box with messages on the left and peers on the right
	// the peers list takes 20 columns, and the messages take the remaining space
	chatPanel := tview.NewFlex().
		AddItem(msgBox, 0, 1, false).
		AddItem(peersList, 20, 1, false)

	// flex is a vertical box with the chatPanel on top and the input field at the bottom.

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(chatPanel, 0, 1, false).
		AddItem(input, 1, 1, true)

	app.SetRoot(flex, true)

	return &ChatUI{
		cr:        cr,
		app:       app,
		peersList: peersList,
		msgW:      msgBox,
		inputCh:   inputCh,
		doneCh:    make(chan struct{}, 1),
	}
}

// Run starts the chat event loop in the background, then starts
// the event loop for the text UI.
func (ui *ChatUI) Run() error {
	go ui.handleEvents()
	defer ui.end()

	return ui.app.Run()
}

// end signals the event loop to exit gracefully
func (ui *ChatUI) end() {
	ui.doneCh <- struct{}{}
}

// refreshPeers pulls the list of peers currently in the chat room and
// displays the last 8 chars of their peer id in the Peers panel in the ui.
func (ui *ChatUI) refreshPeers() {
	peers := ui.cr.ListPeers()

	// clear is not threadsafe so we need to take the lock.
	ui.peersList.Lock()
	ui.peersList.Clear()
	ui.peersList.Unlock()

	for _, p := range peers {
		fmt.Fprintln(ui.peersList, shortID(p))
	}

	ui.app.Draw()
}

func validIncomingMessage(msg string) bool {
	if strings.Contains(msg, "<img id=\"user-sent-image\"") {
		return false
	} else if checkIfCommand(msg) {
		return false
	}
	return true
}

// displayChatMessage writes a ChatMessage from the room to the message window,
// with the sender's nick highlighted in green.
func (ui *ChatUI) displayChatMessage(cm *ChatMessage) {
	prompt := withColor("green", fmt.Sprintf("<%s>:", cm.SenderNick))
	// Decrypt the cipher text
	plaintext := aesDecrypt(ui.cr.roomName, cm.Message)

	if validIncomingMessage(plaintext) {
		fmt.Fprintf(ui.msgW, "%s %s\n", prompt, plaintext)
	}
}

func containsInList(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func checkIfCommand(raw_msg string) bool {
	arrayOfCommands := []string{"/help", "/send", "/exit"} 
	return containsInList(arrayOfCommands, raw_msg)
}

func getPathOfUserSelectedFile() string {

	// create a bash script that opens file dialog for user to select a file
	bash_script_name := "script.sh"
	select_file_name := "selected.txt"
	bash_command := `#!/bin/sh
FileToUpload="$(osascript -l JavaScript -e 'a=Application.currentApplication();a.includeStandardAdditions=true;a.chooseFile({withPrompt:"Please select a file to process:"}).toString()')"
echo $FileToUpload > selected.txt`

	// bash script is created
    f, err := os.Create(bash_script_name)

    if err != nil {
        log.Fatal(err)
    }

    defer f.Close()

    // command is written to the bash script
    _, err2 := f.WriteString(bash_command)

    if err2 != nil {
        log.Fatal(err2)
    }

    // give the file execution permissions
    out, err := exec.Command("chmod", "+x", bash_script_name).Output()

    // execute the bash script
    out, err = exec.Command("/bin/sh", bash_script_name).Output()

    // delete get_file.sh
    out, err = exec.Command("rm", bash_script_name).Output()
	
	_ = out 

    // get the file that the user selected
    f, err = os.Open(select_file_name)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()

	path := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		path = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln(err)
	}

	// delete selected.txt
    out, err = exec.Command("rm", select_file_name).Output()

    return path
}

func getLinkToIPFSFileAfterUpload(filepath string) string {
	out, err := exec.Command("ipfs", "add", filepath).Output()

    if err != nil {
        log.Fatal(err)
    }

    hash := strings.Fields(string(out))[1]

    url_of_file := "https://ipfs.io/ipfs/"+hash

	return url_of_file
}

func executeCommands(command string) (bool, string) {
	if runtime.GOOS == "windows" {
		return true, "Cannot run commands in windows at the moment."
	}
	isSend := false
	msg := ""
	if command == "/help" {
		msg = `/help
=== HELP MENU ===
/help -> get help
/send -> select and upload a file to ipfs and send link
/exit -> exit the program`
	}
	if command == "/send" {
		isSend = true
		msg = "File was uploaded to: " + getLinkToIPFSFileAfterUpload(getPathOfUserSelectedFile())
	}
	if command == "/exit" {
		os.Exit(3)
	}
	return isSend, msg
}

// displaySelfMessage writes a message from ourself to the message window,
// with our nick highlighted in yellow.
func (ui *ChatUI) displaySelfMessage(msg string) {
	// check if user entered a command
	prompt := withColor("yellow", fmt.Sprintf("<%s>:", ui.cr.nick))
	if checkIfCommand(msg) {
		isSend, msg := executeCommands(msg)
		if isSend {
			ui.cr.Publish(msg)
			fmt.Fprintf(ui.msgW, "%s %s\n", prompt, msg)
		}
	} else {
		fmt.Fprintf(ui.msgW, "%s %s\n", prompt, msg)
	}
}

// handleEvents runs an event loop that sends user input to the chat room
// and displays messages received from the chat room. It also periodically
// refreshes the list of peers in the UI.
func (ui *ChatUI) handleEvents() {
	peerRefreshTicker := time.NewTicker(time.Second)
	defer peerRefreshTicker.Stop()

	for {
		select {
		case input := <-ui.inputCh:
			// when the user types in a line, publish it to the chat room and print to the message window
			// check if its a command, if so, dont send it out
			if !checkIfCommand(input) {
				err := ui.cr.Publish(input)
				if err != nil {
					printErr("publish error: %s", err)
				}
			}
			ui.displaySelfMessage(input)

		case m := <-ui.cr.Messages:
			// when we receive a message from the chat room, print it to the message window
			ui.displayChatMessage(m)

		case <-peerRefreshTicker.C:
			// refresh the list of peers in the chat room periodically
			ui.refreshPeers()

		case <-ui.cr.ctx.Done():
			return

		case <-ui.doneCh:
			return
		}
	}
}

// withColor wraps a string with color tags for display in the messages text box.
func withColor(color, msg string) string {
	return fmt.Sprintf("[%s]%s[-]", color, msg)
}
