package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"log"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
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

	// draw the UI
	ui := NewChatUI(cr)
	if err = ui.Run(); err != nil {
		printErr("error running text UI: %s", err)
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
