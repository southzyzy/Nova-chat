package main

import (
	"context"
	"encoding/json"

	"github.com/libp2p/go-libp2p-core/peer"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"fmt"
  "time"
  "bytes"
  "net/http"
  "html/template"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	// pingPeriod = (pongWait * 9) / 10
	pingPeriod = 10 * time.Second
	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

type TemplateData struct {
	roomName string
	nick string
}

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

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
// Web sockets
// ##################################################################################################################################

func (cr *ChatRoom) readRelay(conn *websocket.Conn) {
  conn.SetReadLimit(maxMessageSize)
  conn.SetReadDeadline(time.Now().Add(pongWait))
  conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

  for {
    _, message, err := conn.ReadMessage()
    if err != nil {
      if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
        fmt.Printf("error: %v", err)
      }
      break
    }

    message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
    fmt.Println("Message from client (web): " + string(message))
		cr.Publish(string(message))
  }
}

func (cr *ChatRoom) writeRelay(conn *websocket.Conn){
  ticker := time.NewTicker(pingPeriod)
  fmt.Printf("[*] Ticker for ping interval initialised every %d seconds\n", pingPeriod/time.Second)

  message := ChatMessage{
    Message: "PingMessage",
    SenderID: "xxxxxxxx",
    SenderNick: "Nova-Chat-Server",
  }

  // Convert object to byte array using json.Marshal
  jsonMsg, err := json.Marshal(message)
	if err != nil {
		panic(err)
	}

	for {
		select {
			case m := <- cr.Messages:
				plaintext := aesDecrypt(cr.roomName, m.Message)
				// fmt.Fprintf(ui.msgW, "%s %s\n", prompt, plaintext)

				fmt.Println("Message from interweb (peers): " + plaintext)
	      if err:= conn.WriteMessage(websocket.TextMessage, []byte(plaintext)); err != nil {
	        panic(err)
	        return
	      }

			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
	        panic(err)
	        return
				}

				peers := cr.ListPeers()
				fmt.Println("Peers: ", peers)

				if err:= conn.WriteMessage(websocket.TextMessage, []byte(jsonMsg)); err != nil {
					panic(err)
					return
				}
		}
	}
}

func (cr *ChatRoom) websocketHandler(w http.ResponseWriter, r *http.Request) {
  // Upgrade connection to websocket connection
  conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

  go cr.readRelay(conn)
  go cr.writeRelay(conn)
}


// Handler to respond to "GET" requests, most likely used to retrieve chat messages
func (cr *ChatRoom) chatHandler(w http.ResponseWriter, r *http.Request) {
	// fmt.Println("chatHandler", string(cr.roomName))

	data := TemplateData{roomName: cr.roomName, nick: cr.nick}
  // Data (Chatroom) to send to webpage (chat/index.html)
  t, _ := template.ParseFiles("chat/index.html")
  t.Execute(w, data)
}
