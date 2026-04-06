package kvsrv

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/darshankonnur/cs6650-hw10/internal/store"
)

// LeaderlessConfig wires peer URLs (all replicas are symmetric).
type LeaderlessConfig struct {
	Peers     []string
	NodeIndex int
}

// LeaderlessServer: W=N, R=1 (read returns local copy only).
type LeaderlessServer struct {
	cfg  LeaderlessConfig
	st   *store.Store
	self string
	pc   *PeerClient
}

func NewLeaderless(cfg LeaderlessConfig, st *store.Store) *LeaderlessServer {
	if st == nil {
		st = store.New()
	}
	self := strings.TrimRight(cfg.Peers[cfg.NodeIndex], "/")
	return &LeaderlessServer{
		cfg:  cfg,
		st:   st,
		self: self,
		pc:   NewPeerClient(),
	}
}

func (s *LeaderlessServer) otherPeers() []string {
	var out []string
	for i, p := range s.cfg.Peers {
		if i == s.cfg.NodeIndex {
			continue
		}
		out = append(out, strings.TrimRight(p, "/"))
	}
	return out
}

func (s *LeaderlessServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /kv/set", s.handleSet)
	mux.HandleFunc("GET /kv/get", s.handleGet)
	mux.HandleFunc("GET /kv/local_read", s.handleLocalRead)
	mux.HandleFunc("POST /internal/replicate", s.handleReplicate)
}

func (s *LeaderlessServer) handleSet(w http.ResponseWriter, r *http.Request) {
	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key cannot be empty", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	version := s.st.WriteLocal(req.Key, req.Value)
	for _, peer := range s.otherPeers() {
		if err := s.pc.Replicate(ctx, peer, ReplicateRequest{
			Key: req.Key, Value: req.Value, Version: version,
		}); err != nil {
			log.Printf("replicate to %s: %v", peer, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		select {
		case <-ctx.Done():
			http.Error(w, ctx.Err().Error(), http.StatusInternalServerError)
			return
		case <-time.After(LeaderPauseAfterFollowerMessage):
		}
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": version})
}

func (s *LeaderlessServer) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	e, ok := s.st.Get(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

func (s *LeaderlessServer) handleLocalRead(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	e, ok := s.st.Get(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

func (s *LeaderlessServer) handleReplicate(w http.ResponseWriter, r *http.Request) {
	var req ReplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	time.Sleep(FollowerPauseOnReplicate)
	s.st.PutVersion(req.Key, req.Value, req.Version)
	w.WriteHeader(http.StatusOK)
}
