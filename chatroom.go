package main

import (
	"context"
	"errors"
	"encoding/json"
	"encoding/base64"

	"github.com/libp2p/go-libp2p-core/peer"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/microcosm-cc/bluemonday"

	"fmt"
	"log"
    "time"
    "bytes"
    "strings"
    "net/http"
    "html/template"
    "github.com/gorilla/websocket"
    "os/exec"
    "image"
    _ "image/gif"
    _ "image/jpeg"
    _ "image/png"
    "io/ioutil"
)

const (
	nova_chat_peerlist_indicator = "NOVA-PEERLIST"

	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	peerlistInterval = 10 * time.Second
	// Maximum message size allowed from peer.
	maxMessageSize = 256000
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
	// Declare policy for sanitising user input
	sanitisation_policy = bluemonday.UGCPolicy()
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

var (
    ErrBucket       = errors.New("Invalid bucket!")
    ErrSize         = errors.New("Invalid size!")
    ErrInvalidImage = errors.New("Invalid image!")
)

// ChatRoomBufSize is the number of incoming messages to buffer for each topic.
const ChatRoomBufSize = 128

// ChatRoom represents a subscription to a single PubSub topic. Messages
// can be published to the topic with ChatRoom.Publish, and received
// messages are pushed to the Messages channel.
type ChatRoom struct {
	// Messages is a channel of messages received from other peers in the chat room
	Messages chan *ChatMessage

	ctx   context.Context
	ps    *pubsub.PubSub
	topic *pubsub.Topic
	sub   *pubsub.Subscription

	roomName string
	self     peer.ID
	nick     string
}

// ChatMessage gets converted to/from JSON and sent in the body of pubsub messages.
type ChatMessage struct {
	Message    string
	SenderID   string
	SenderNick string
	MessageType string
}

// JoinChatRoom tries to subscribe to the PubSub topic for the room name, returning
// a ChatRoom on success.
func JoinChatRoom(ctx context.Context, ps *pubsub.PubSub, selfID peer.ID, nickname string, roomName string) (*ChatRoom, error) {
	// join the pubsub topic
	topic, err := ps.Join(topicName(roomName))
	if err != nil {
		return nil, err
	}

	// and subscribe to it
	// sub, err := topic.Subscribe()
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}

	cr := &ChatRoom{
		ctx:      ctx,
		ps:       ps,
		topic:    topic,
		sub:      sub,
		self:     selfID,
		nick:     nickname,
		roomName: roomName,
		Messages: make(chan *ChatMessage, ChatRoomBufSize),
	}

	// start reading messages from the subscription in a loop
	go cr.readLoop()
	return cr, nil
}

// Publish sends a message to the pubsub topic.
func (cr *ChatRoom) Publish(message string) error {
	// Encrypt the plaintext message
	ciphertext := aesEncrypt(cr.roomName, message)

	m := ChatMessage{
		Message:    ciphertext,
		SenderID:   cr.self.Pretty(),
		SenderNick: cr.nick,
	}

	msgBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return cr.topic.Publish(cr.ctx, msgBytes)
}

func (cr *ChatRoom) ListPeers() []peer.ID {
	return cr.ps.ListPeers(topicName(cr.roomName))
}

// readLoop pulls messages from the pubsub topic and pushes them onto the Messages channel.
func (cr *ChatRoom) readLoop() {
	for {
		msg, err := cr.sub.Next(cr.ctx)
		if err != nil {
			close(cr.Messages)
			return
		}

		// only forward messages delivered by others
		if msg.ReceivedFrom == cr.self {
			continue
		}

		cm := new(ChatMessage)
		err = json.Unmarshal(msg.Data, cm)
		if err != nil {
			continue
		}
		// send valid messages onto the Messages channel
		cr.Messages <- cm
	}
}

func topicName(roomName string) string {
	return "nova-chat-room:" + roomName
}


// ##################################################################################################################################
// Web sockets stuffs
// ##################################################################################################################################

func (cr *ChatRoom) readFromSocket(conn *websocket.Conn) {

  conn.SetReadLimit(maxMessageSize)
  conn.SetReadDeadline(time.Now().Add(pongWait))
  conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

  for {
		// Read websocket message
    _, websocket_message, err := conn.ReadMessage()
    if err != nil {
      if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
        fmt.Printf("Error reading websocket message: %v", err)
      }
      break
    }

		// Parse the message
    websocket_message = bytes.TrimSpace(bytes.Replace(websocket_message, newline, space, -1))

		// Sanitise the message with bluemonday
		sanitised_message := sanitisation_policy.Sanitize(string(websocket_message))

		// fmt.Println(sanitised_message)
		if strings.Contains(string(websocket_message), "<img id='user-sent-image'") {
				// get the image url
				image_url := upload_get_user_sent_image_url(string(websocket_message))
				b64_image := extract_b64(string(websocket_message))
				fmt.Println(string(craft_image_msg(image_url, b64_image)))
				// send the url back to the client
				conn.WriteMessage(websocket.TextMessage, []byte(craft_image_msg(image_url, b64_image)))
		}

		// Detect if there is a change after sanitisation
		if string(websocket_message) != string(sanitised_message) {
			fmt.Println("[..] Caught unsanitised message: ", string(websocket_message))
		} else {
			// Prints sanitised message
			fmt.Println("[..] Message from client: " + string(sanitised_message))
		}

		// If sanitised_message not empty
		if len(sanitised_message) > 0 {
			// Publish message to pubsub topic
			cr.Publish(string(sanitised_message))
		}

  }
}

func craft_image_msg(link string, base64_image string) []byte {
	// ChatMessage object
	message := ChatMessage{
		Message: link,
		SenderID: "placeholderID",
		SenderNick: "placeholderNick",
		MessageType: "image_link",
	}

	// Convert message object to byte array using json.Marshal
  	jsonMsg, err := json.Marshal(message)
	if err != nil {
		panic(err)
	}

	return jsonMsg
}

func extract_b64(raw_msg string) string {
	return raw_msg[31:len(raw_msg)-86]
}

func upload_get_user_sent_image_url(raw_msg string) string {
	fmt.Println("Uploading user sent image.")
	b64_image := extract_b64(raw_msg)
	filepath, err := saveImageToDisk("./images/uploads/uploaded", b64_image)

	out, err := exec.Command("ipfs", "add", filepath).Output()

    if err != nil {
        log.Fatal(err)
    }

    hash := strings.Fields(string(out))[1]

    url_of_file := "https://ipfs.io/ipfs/"+hash
    fmt.Println(url_of_file)

	return url_of_file
}

func saveImageToDisk(fileNameBase, data string) (string, error) {
    idx := strings.Index(data, ";base64,")
    if idx < 0 {
        return "", ErrInvalidImage
    }
    reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(data[idx+8:]))
    buff := bytes.Buffer{}
    _, err := buff.ReadFrom(reader)
    if err != nil {
        return "", err
    }
    imgCfg, fm, err := image.DecodeConfig(bytes.NewReader(buff.Bytes()))
    if err != nil {
        return "", err
    }

    if imgCfg.Width > 5000 || imgCfg.Height > 5000 {
        return "", ErrSize
    }

    filePath := fileNameBase + "." + fm
    ioutil.WriteFile(filePath, buff.Bytes(), 0644)

    return filePath, err
}

func (cr *ChatRoom) writeToSocket(conn *websocket.Conn){
	// Initialise timer for ping-pong
  ticker := time.NewTicker(pingPeriod)
	peerlistTicker := time.NewTicker(peerlistInterval)
  fmt.Printf("[*] Ticker for ping-pong interval initialised for every %d seconds\n", pingPeriod/time.Second)
	fmt.Printf("[*] Ticker for peerlist interval initialised for every %d seconds\n", peerlistInterval/time.Second)

	var message ChatMessage
	var peerlist []peer.ID
	var peerlist_string string

	for {
		select {
			// Upon a new message received
			case m := <- cr.Messages:
				// Decrypt message
				plaintext := aesDecrypt(cr.roomName, m.Message)

				// Sanitise the message with bluemonday
				sanitised_plaintext := sanitisation_policy.Sanitize(string(plaintext))

				// Detect if there is a change after sanitisation
				if string(plaintext) != string(sanitised_plaintext) {
					fmt.Println("[.] Caught unsanitised message: (" + m.SenderNick + ") [" + m.SenderID + "]: ", string(plaintext))
				} else {
					// Prints sanitised message
					fmt.Println("[.] Message from interweb (" + m.SenderNick + ") [" + m.SenderID + "]: " + string(sanitised_plaintext))
				}

				// ChatMessage object
				message = ChatMessage{
					Message: plaintext,
					SenderID: m.SenderID,
					SenderNick: m.SenderNick,
					MessageType: "message",
				}

				// Convert message object to byte array using json.Marshal
			  jsonMsg, err := json.Marshal(message)
				if err != nil {
					panic(err)
				}

				// Write message to websocket connection (send to web ui)
	      if err:= conn.WriteMessage(websocket.TextMessage, []byte(jsonMsg)); err != nil {
	        panic(err)
	        return
	      }

			// Upon peerlist interval
			case <-peerlistTicker.C:
				curr_peers := cr.ListPeers()

				// Only send updates to websocket if there is a change in peers
				if Equal(peerlist, curr_peers) == false {
						fmt.Println("Peer list updated: ", curr_peers)

						// Converts peer.ID[] to string
						for _, p := range curr_peers {
							peerlist_string += shortID(p)
						}

						// ChatMessage object
						message = ChatMessage{
							Message: peerlist_string,
							SenderID: nova_chat_peerlist_indicator,
							SenderNick: nova_chat_peerlist_indicator,
							MessageType: "message",
						}

						// Convert message object to byte array using json.Marshal
						jsonMsg, err := json.Marshal(message)
						if err != nil {
							panic(err)
						}

						// Write message to websocket connection (send to web ui)
						if err:= conn.WriteMessage(websocket.TextMessage, []byte(jsonMsg)); err != nil {
							panic(err)
							return
						}

						peerlist = curr_peers
				}

			// Upon ping-pong interval
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					fmt.Println("[!] Err: Websocket connection closed!")
	        panic(err)
	        return
				}
		}
	}
}

// Handler for websocket connection
func (cr *ChatRoom) websocketHandler(w http.ResponseWriter, r *http.Request) {
  // Upgrade connection to websocket connection
  conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Execute goroutines for both read and write socket connection operations
  go cr.readFromSocket(conn)
  go cr.writeToSocket(conn)
}

// Handler for main/chat page
func (cr *ChatRoom) chatHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("[*] Web UI initialised, roomname: ", string(cr.roomName))

	data := map[string]interface{}{"roomName" : cr.roomName, "nick": cr.nick, "nova_img" : "images/nova_chat.png"}

  // Data (Chatroom) to send to webpage (chat/index.html)
  t, _ := template.ParseFiles("chat/index.html")
  t.Execute(w, data)
}

// Helper function to determine if 2 []peer.ID arrays are equal
func Equal(a, b []peer.ID) bool {
    if len(a) != len(b) {
        return false
    }
    for i, v := range a {
        if v != b[i] {
            return false
        }
    }
    return true
}
