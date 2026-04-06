// Binary leaderless runs the symmetric KV service (W=N, R=1).
package main

import (
	"log"
	"net/http"

	"github.com/darshankonnur/cs6650-hw10/internal/config"
	"github.com/darshankonnur/cs6650-hw10/internal/kvsrv"
)

func main() {
	peers, err := config.LoadPeers()
	if err != nil {
		log.Fatal(err)
	}
	idx, err := config.NodeIndex()
	if err != nil {
		log.Fatal(err)
	}
	if idx >= len(peers) {
		log.Fatalf("NODE_INDEX %d out of range for %d peers", idx, len(peers))
	}

	srv := kvsrv.NewLeaderless(kvsrv.LeaderlessConfig{
		Peers:     peers,
		NodeIndex: idx,
	}, nil)

	mux := http.NewServeMux()
	srv.Register(mux)
	addr := ":" + config.Port()
	log.Printf("leaderless listening on %s (node %d/%d)", addr, idx, len(peers))
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
