package main

import (
	"context"
	"fmt"
	"log"
	// "sync"
	"crypto/rand"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peerstore"
	
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
	discovery "github.com/libp2p/go-libp2p-discovery"
)

// Func to make routed host 
func makeRoutedHost(ctx context.Context, serviceName string) (host.Host, error) {
	r := rand.Reader

	// Generate a key pair for this host. We will use it at least
	// to obtain a valid host ID.
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		panic(err)
	}

	opts := []libp2p.Option{
		// Random Source Port
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/0")),
		// Use the randomly generated keypair
		libp2p.Identity(priv),
		// support TLS connections
		libp2p.DefaultSecurity,
		// support datastore
		libp2p.DefaultMuxers,
		// Attempt to open ports using uPNP for NATed hosts
		libp2p.NATPortMap(),
		// This service is highly rate-limited and should not cause any
		// performance issues.
		libp2p.EnableNATService(),
	}

	// create a new libp2p Host that listens on a random TCP port
	// h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
	h, err := libp2p.New(opts...)
	if err != nil {
		panic(err)
	}

	// Construct a datastore (needed by the DHT). This is just a simple, in-memory thread-safe datastore.
	dstore := dsync.MutexWrap(ds.NewMapDatastore())

	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kaddht := dht.NewDHT(ctx, h, dstore)
	
	// Make the routed host
	routedHost := rhost.Wrap(h, kaddht)

	// connect to the chosen ipfs nodes
	err = bootstrapConnect(ctx, routedHost, dht.GetDefaultBootstrapPeerAddrInfos())
	if err != nil {
		return nil, err
	}

	// Bootstrap the DHT. In the default configuration, this spawns a Background
	// thread that will refresh the peer table every five minutes.
	if err = kaddht.Bootstrap(ctx); err != nil {
		panic(err)
	}

	// Setup Peer Discovery with DHT
	log.Println("Annoucing with DHT ...")
	routingDiscovery := discovery.NewRoutingDiscovery(kaddht)
	discovery.Advertise(ctx, routingDiscovery, serviceName)
	log.Println("Successfully accounced with DHT !")

	// Now, look for others who have announced
	// This is like your friend telling you the location to meet you.
	log.Println("Searching for other peers...")
	connectedPeers := make([]peer.ID, 128)
	// Find peers in loop and store them in the peerstore
	go func() {
		for {
			peerChan, err := routingDiscovery.FindPeers(ctx, serviceName)
			if err != nil {
				panic(err)
			}
			for p := range peerChan {
				if p.ID == routedHost.ID() || containsID(connectedPeers, p.ID) {
					continue
				}
				//Store addresses in the peerstore and connect to the peer found (and localize it)
				if len(p.Addrs) > 0 {
					routedHost.Peerstore().AddAddrs(p.ID, p.Addrs, peerstore.ConnectedAddrTTL)
					err := routedHost.Connect(ctx, p)
					if err == nil {
						log.Println("- Connected to", p)
						connectedPeers = append(connectedPeers, p.ID)
					} else {
						log.Printf("- Error connecting to %s: %s\n", p, err)
						return
					}
				}
			}
		}
	}()

	// Build host multiaddress
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", routedHost.ID().Pretty()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	// addr := routedHost.Addrs()[0]
	addrs := routedHost.Addrs()
	log.Println("I can be reached at:")
	for _, addr := range addrs {
		log.Println(addr.Encapsulate(hostAddr))
	}

	return routedHost, nil
}

//Check if a peer id is in the list given
func containsID(list []peer.ID, p peer.ID) bool {
	for _, pid := range list {
		if pid == p {
			return true
		}
	}
	return false
}