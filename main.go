package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"log"
	"path/filepath"
	"time"

	"net/http"
	"github.com/gorilla/mux"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const ( //
	// HTTP port
	http_port = "80"
	// HTTPS port
	https_port = "443"
)


func main() {
	// Set Logging Information
	currentTime := time.Now()
	logname := fmt.Sprintf("%s.txt", currentTime.Format("2006-01-02")) // yyyy-MM-dd

	// Check for log directory
	logpath := filepath.Join(".", "logs")
	err := os.MkdirAll(logpath, os.ModePerm)
	if err != nil {
		panic(err)
	}

	// Open log file for R/W
	f, err := os.OpenFile(filepath.Join(logpath, logname), os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	// Set Log output to file
	log.SetOutput(f)

	// parse some flags to set our nickname and the room to join
	nickFlag := flag.String("nick", "", "nickname to use in chat. will be generated if empty")
	topicFlag := flag.String("topic", "/nova-chat-topic/1.0", "topic to subscribe")
	webuiFlag := flag.Bool("webui", false, "flag to run web ui")
	httpFlag := flag.Bool("http", false, "flag to run web ui in HTTP, default is HTTPS")

	keyFlag := flag.String("key", "", "self-signed certificate")
	certFlag := flag.String("cert", "", "self-signed key")
	flag.Parse()

	ctx := context.Background()

	// Retrieve the topic name
	protoTopicName := *topicFlag

	// Start routed host
	ha, err := makeRoutedHost(ctx, protoTopicName)
	if err != nil {
		panic(err)
	}
	log.Printf("My CID: %s\n", ha.ID().Pretty())

	// create a new PubSub service using the GossipSub router
	ps, err := pubsub.NewGossipSub(ctx, ha)
	if err != nil {
		panic(err)
	}

	// use the nickname from the cli flag, or a default if blank
	nick := *nickFlag
	if len(nick) == 0 {
		nick = defaultNick(ha.ID())
	}

	// join the chat room
	cr, err := JoinChatRoom(ctx, ps, ha.ID(), nick, protoTopicName)
	if err != nil {
		panic(err)
	}

	// Detects if webui is enabled
	if *webuiFlag {
		fmt.Println("[*] Web UI flag detected, running web ui...")

		r := webuiRouter(cr)

		// User specified to run webui in HTTP
		if *httpFlag {
			fmt.Println("[*] Starting server on http://localhost:" + http_port + "/chat")

			http.ListenAndServe(":" + http_port, r)
		} else {
			fmt.Println("[*] Starting server on https://localhost:" + https_port + "/chat")
			// Listen and server http listener solely to redirect requests to https
			// go http.ListenAndServe(":" + http_port, http.HandlerFunc(httpsRedirect))

			// openssl req -new -newkey rsa:2048 -nodes -keyout ssl/localhost.key -out ssl/localhost.csr
			// openssl  x509  -req  -days 365  -in ssl/localhost.csr  -signkey ssl/localhost.key  -out ssl/localhost.crt
			if err := http.ListenAndServeTLS(":" + https_port, "ssl/localhost.crt", "ssl/localhost.key", r); err != nil {
					fmt.Println("[!] Error: ", err)
					fmt.Println("[-] Run the commands: \nopenssl req -new -newkey rsa:2048 -nodes -keyout ssl/localhost.key -out ssl/localhost.csr")
					fmt.Println("[-] and, ")
					fmt.Println("[-] openssl  x509  -req  -days 365  -in ssl/localhost.csr  -signkey ssl/localhost.key  -out ssl/localhost.crt")
			}
		}

	} else {
		fmt.Println("[*] Running terminal UI...")
		// draw the UI
		ui := NewChatUI(cr)
		if err = ui.Run(); err != nil {
			printErr("error running text UI: %s", err)
		}
	}
}

// printErr is like fmt.Printf, but writes to stderr.
func printErr(m string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, m, args...)
}

// defaultNick generates a nickname based on the $USER environment variable and
// the last 8 chars of a peer ID.
func defaultNick(p peer.ID) string {
	return fmt.Sprintf("%s-%s", os.Getenv("USER"), shortID(p))
}

// shortID returns the last 8 chars of a base58-encoded peer id.
func shortID(p peer.ID) string {
	pretty := p.Pretty()
	return pretty[len(pretty)-8:]
}

// httpsRedirect redirects http requests to https
func httpsRedirect(w http.ResponseWriter, r *http.Request) {
    http.Redirect(
        w, r,
        "https://"+r.Host+r.URL.String(),
        http.StatusMovedPermanently,
    )
}

// Router for web ui
func webuiRouter(cr *ChatRoom) *mux.Router {
	r := mux.NewRouter()

  chatFileDirectory := http.Dir("./images/")
  chatFileHandler := http.StripPrefix("/images/", http.FileServer(chatFileDirectory))
  r.PathPrefix("/images/").Handler(chatFileHandler).Methods("GET")

  // Handler for websocket
  r.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		cr.websocketHandler(w, r)
	})

  // Handler for the chat
  r.HandleFunc("/", cr.chatHandler).Methods("GET")
  r.HandleFunc("/chat", cr.chatHandler).Methods("GET")

	return r
}
