// Binary leader-follower runs the replicated KV service (one leader, four followers).
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
	prof, err := config.QuorumProfile()
	if err != nil {
		log.Fatal(err)
	}
	if len(peers) != 5 {
		log.Printf("warning: expected 5 PEERS for this assignment, got %d", len(peers))
	}

	srv := kvsrv.NewLeaderFollower(kvsrv.LeaderFollowerConfig{
		Peers:     peers,
		NodeIndex: idx,
		Profile:   prof,
	}, nil)

	mux := http.NewServeMux()
	srv.Register(mux)
	addr := ":" + config.Port()
	log.Printf("leader-follower listening on %s (node %d/%d, profile=%s)", addr, idx, len(peers), prof)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
