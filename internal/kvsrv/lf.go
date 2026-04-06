package kvsrv

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/darshankonnur/cs6650-hw10/internal/store"
)

// QuorumProfile selects R/W behavior for leader-follower mode.
type QuorumProfile string

const (
	ProfileW5R1 QuorumProfile = "w5r1"
	ProfileW1R5 QuorumProfile = "w1r5"
	ProfileR3W3 QuorumProfile = "r3w3"
)

// LeaderFollowerConfig is wired from environment / flags.
type LeaderFollowerConfig struct {
	Peers     []string
	NodeIndex int
	Profile   QuorumProfile
}

// LeaderFollowerServer implements the assignment leader-follower database.
type LeaderFollowerServer struct {
	cfg        LeaderFollowerConfig
	st         *store.Store
	peers      []string
	leader     string
	self       string
	isLeader   bool
	pc         *PeerClient
	asyncWg    sync.WaitGroup
	shutdownCh chan struct{}
}

func NewLeaderFollower(cfg LeaderFollowerConfig, st *store.Store) *LeaderFollowerServer {
	if st == nil {
		st = store.New()
	}
	self := cfg.Peers[cfg.NodeIndex]
	leader := cfg.Peers[0]
	return &LeaderFollowerServer{
		cfg:        cfg,
		st:         st,
		peers:      cfg.Peers,
		leader:     leader,
		self:       strings.TrimRight(self, "/"),
		isLeader:   cfg.NodeIndex == 0,
		pc:         NewPeerClient(),
		shutdownCh: make(chan struct{}),
	}
}

func (s *LeaderFollowerServer) Close() {
	close(s.shutdownCh)
	s.asyncWg.Wait()
}

func (s *LeaderFollowerServer) followerURLs() []string {
	var out []string
	for i, p := range s.peers {
		if i == 0 {
			continue
		}
		out = append(out, strings.TrimRight(p, "/"))
	}
	return out
}

func (s *LeaderFollowerServer) replicateToAllFollowers(ctx context.Context, key, value string, version int64, pauseAfterEach bool) error {
	for _, f := range s.followerURLs() {
		if err := s.pc.Replicate(ctx, f, ReplicateRequest{Key: key, Value: value, Version: version}); err != nil {
			return err
		}
		if pauseAfterEach {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(LeaderPauseAfterFollowerMessage):
			}
		}
	}
	return nil
}

func (s *LeaderFollowerServer) replicateToSubset(ctx context.Context, followers []string, key, value string, version int64, need int) error {
	if need <= 0 {
		return nil
	}
	count := 0
	for _, f := range followers {
		if count >= need {
			break
		}
		if err := s.pc.Replicate(ctx, f, ReplicateRequest{Key: key, Value: value, Version: version}); err != nil {
			return err
		}
		count++
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(LeaderPauseAfterFollowerMessage):
		}
	}
	return nil
}

func (s *LeaderFollowerServer) asyncReplicateToFollowers(key, value string, version int64) {
	s.asyncReplicateToPeerList(key, value, version, s.followerURLs())
}

func (s *LeaderFollowerServer) asyncReplicateToPeerList(key, value string, version int64, peers []string) {
	s.asyncWg.Add(1)
	go func(peers []string) {
		defer s.asyncWg.Done()
		ctx := context.Background()
		for _, f := range peers {
			if err := s.pc.Replicate(ctx, f, ReplicateRequest{Key: key, Value: value, Version: version}); err != nil {
				log.Printf("async replicate to %s: %v", f, err)
				return
			}
			time.Sleep(LeaderPauseAfterFollowerMessage)
		}
	}(peers)
}

// -------- HTTP handlers

func (s *LeaderFollowerServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	mux.HandleFunc("POST /kv/set", s.handleSet)
	mux.HandleFunc("GET /kv/get", s.handleGetQuery)
	mux.HandleFunc("GET /kv/local_read", s.handleLocalRead)

	mux.HandleFunc("POST /internal/replicate", s.handleInternalReplicate)
	mux.HandleFunc("GET /internal/peer-read", s.handleInternalPeerRead)
	mux.HandleFunc("GET /internal/cluster-read", s.handleInternalClusterRead)
}

func (s *LeaderFollowerServer) handleSet(w http.ResponseWriter, r *http.Request) {
	if !s.isLeader {
		http.Error(w, "writes must go to the leader", http.StatusForbidden)
		return
	}
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

	switch s.cfg.Profile {
	case ProfileW5R1:
		if err := s.replicateToAllFollowers(ctx, req.Key, req.Value, version, true); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case ProfileW1R5:
		s.asyncReplicateToFollowers(req.Key, req.Value, version)
	case ProfileR3W3:
		followers := s.followerURLs()
		sort.Strings(followers)
		if err := s.replicateToSubset(ctx, followers, req.Key, req.Value, version, 2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(followers) > 2 {
			s.asyncReplicateToPeerList(req.Key, req.Value, version, followers[2:])
		}
	default:
		http.Error(w, "unknown quorum profile", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": version})
}

func (s *LeaderFollowerServer) handleGetQuery(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	if s.isLeader {
		e, ok := s.clusterReadLocal(r.Context(), key)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(e)
		return
	}
	rr, err := s.pc.ClusterRead(r.Context(), s.leader, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !rr.Found {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rr.Entry)
}

func (s *LeaderFollowerServer) handleLocalRead(w http.ResponseWriter, r *http.Request) {
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

func (s *LeaderFollowerServer) handleInternalReplicate(w http.ResponseWriter, r *http.Request) {
	var req ReplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	time.Sleep(FollowerPauseOnReplicate)
	s.st.PutVersion(req.Key, req.Value, req.Version)
	w.WriteHeader(http.StatusOK)
}

func (s *LeaderFollowerServer) handleInternalPeerRead(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	time.Sleep(FollowerPauseOnPeerRead)
	e, ok := s.st.Get(key)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ReadResponse{Found: ok, Entry: e})
}

func (s *LeaderFollowerServer) handleInternalClusterRead(w http.ResponseWriter, r *http.Request) {
	if !s.isLeader {
		http.Error(w, "only leader", http.StatusForbidden)
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	e, ok := s.clusterReadLocal(r.Context(), key)
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		_ = json.NewEncoder(w).Encode(ReadResponse{Found: false})
		return
	}
	_ = json.NewEncoder(w).Encode(ReadResponse{Found: true, Entry: e})
}

func (s *LeaderFollowerServer) clusterReadLocal(ctx context.Context, key string) (store.Entry, bool) {
	switch s.cfg.Profile {
	case ProfileW5R1:
		e, ok := s.st.Get(key)
		return e, ok
	case ProfileW1R5:
		return s.readFromPeers(ctx, key, len(s.peers))
	case ProfileR3W3:
		return s.readFromPeers(ctx, key, 3)
	default:
		e, ok := s.st.Get(key)
		return e, ok
	}
}

// readFromPeers reads key from the first `degree` nodes in peer order (index 0 is leader).
// Returns the entry with the highest logical version among nodes that have the key.
// If no node reports the key, returns (Entry{}, false).
func (s *LeaderFollowerServer) readFromPeers(ctx context.Context, key string, degree int) (store.Entry, bool) {
	if degree > len(s.peers) {
		degree = len(s.peers)
	}
	type nodeResp struct {
		e store.Entry
		f bool
	}
	var wg sync.WaitGroup
	ch := make(chan nodeResp, degree)
	for i := 0; i < degree; i++ {
		base := strings.TrimRight(s.peers[i], "/")
		if base == s.self {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e, ok := s.st.Get(key)
				ch <- nodeResp{e: e, f: ok}
			}()
			continue
		}
		wg.Add(1)
		go func(base string) {
			defer wg.Done()
			e, ok, err := s.pc.PeerRead(ctx, base, key)
			if err != nil {
				log.Printf("peer read %s: %v", base, err)
				ch <- nodeResp{f: false}
				return
			}
			ch <- nodeResp{e: e, f: ok}
		}(base)
	}
	wg.Wait()
	close(ch)
	var best store.Entry
	var found bool
	for nr := range ch {
		if nr.f {
			if !found || nr.e.Version > best.Version {
				best = nr.e
				found = true
			}
		}
	}
	if !found {
		return store.Entry{}, false
	}
	return best, true
}
