package kvsrv_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/darshankonnur/cs6650-hw10/internal/kvsrv"
)

func peerBaseURLs(listeners []net.Listener) []string {
	out := make([]string, len(listeners))
	for i, ln := range listeners {
		out[i] = "http://" + ln.Addr().String()
	}
	return out
}

func startLFServers(t *testing.T, profile kvsrv.QuorumProfile) ([]net.Listener, []string, func()) {
	t.Helper()
	const n = 5
	listeners := make([]net.Listener, n)
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		listeners[i] = ln
	}
	urls := peerBaseURLs(listeners)
	var httpServers []*http.Server
	for i := 0; i < n; i++ {
		mux := http.NewServeMux()
		kvsrv.NewLeaderFollower(kvsrv.LeaderFollowerConfig{
			Peers:     urls,
			NodeIndex: i,
			Profile:   profile,
		}, nil).Register(mux)
		srv := &http.Server{Handler: mux}
		httpServers = append(httpServers, srv)
		go func(ln net.Listener, hs *http.Server) {
			_ = hs.Serve(ln)
		}(listeners[i], srv)
	}
	cleanup := func() {
		for _, hs := range httpServers {
			_ = hs.Close()
		}
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}
	return listeners, urls, cleanup
}

func httpPostJSON(url string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return http.Post(url, "application/json", bytes.NewReader(b))
}

func TestLeaderFollower_W5R1_ConsistentAfterWrite(t *testing.T) {
	_, urls, cleanup := startLFServers(t, kvsrv.ProfileW5R1)
	defer cleanup()
	leader := urls[0]
	follower := urls[1]

	resp, err := httpPostJSON(leader+"/kv/set", map[string]string{"key": "k1", "value": "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("set: %d %s", resp.StatusCode, body)
	}

	for _, base := range []string{leader, follower} {
		r, err := http.Get(base + "/kv/get?key=k1")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("get %s: %d", base, r.StatusCode)
		}
		var got map[string]any
		json.Unmarshal(body, &got)
		if got["value"] != "v1" {
			t.Fatalf("value mismatch: %+v", got)
		}
	}
}

func TestLeaderFollower_LocalReadDuringReplication(t *testing.T) {
	_, urls, cleanup := startLFServers(t, kvsrv.ProfileW1R5)
	defer cleanup()
	leader := urls[0]

	resp, err := httpPostJSON(leader+"/kv/set", map[string]string{"key": "lag", "value": "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("set %d", resp.StatusCode)
	}

	var sawStale bool
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 1; i < 5; i++ {
		wg.Add(1)
		base := urls[i]
		go func() {
			defer wg.Done()
			for j := 0; j < 40; j++ {
				r, err := http.Get(base + "/kv/local_read?key=lag")
				if err != nil {
					return
				}
				body, _ := io.ReadAll(r.Body)
				r.Body.Close()
				if r.StatusCode == http.StatusNotFound {
					mu.Lock()
					sawStale = true
					mu.Unlock()
					return
				}
				var e struct {
					Value string `json:"value"`
				}
				json.Unmarshal(body, &e)
				if e.Value != "x" {
					mu.Lock()
					sawStale = true
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	if !sawStale {
		t.Log("note: did not observe follower lag in this run (timing-dependent)")
	}
}

func startLeaderlessCluster(t *testing.T) ([]net.Listener, []string, func()) {
	t.Helper()
	const n = 5
	listeners := make([]net.Listener, n)
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners[i] = ln
	}
	urls := peerBaseURLs(listeners)
	var httpServers []*http.Server
	for i := 0; i < n; i++ {
		mux := http.NewServeMux()
		kvsrv.NewLeaderless(kvsrv.LeaderlessConfig{Peers: urls, NodeIndex: i}, nil).Register(mux)
		srv := &http.Server{Handler: mux}
		httpServers = append(httpServers, srv)
		go func(ln net.Listener, hs *http.Server) {
			_ = hs.Serve(ln)
		}(listeners[i], srv)
	}
	cleanup := func() {
		for _, hs := range httpServers {
			_ = hs.Close()
		}
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}
	return listeners, urls, cleanup
}

func TestLeaderless_EventualConsistencyWindow(t *testing.T) {
	_, urls, cleanup := startLeaderlessCluster(t)
	defer cleanup()

	coordIdx := 2
	base := urls[coordIdx]

	done := make(chan struct{})
	go func() {
		resp, err := httpPostJSON(base+"/kv/set", map[string]string{"key": "z", "value": "coordinated"})
		if err != nil {
			close(done)
			return
		}
		resp.Body.Close()
		close(done)
	}()

	time.Sleep(5 * time.Millisecond)
	staleReads := 0
	for i := 0; i < len(urls); i++ {
		if i == coordIdx {
			continue
		}
		r, err := http.Get(urls[i] + "/kv/get?key=z")
		if err != nil {
			continue
		}
		if r.StatusCode == http.StatusNotFound {
			staleReads++
		}
		r.Body.Close()
	}
	<-done

	if staleReads == 0 {
		t.Log("note: no stale reads in window (timing-dependent)")
	}

	r, err := http.Get(base + "/kv/get?key=z")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("coordinator read: %d", r.StatusCode)
	}

	other := 0
	if other == coordIdx {
		other = 1
	}
	r2, err := http.Get(urls[other] + "/kv/get?key=z")
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("peer after ack: %d", r2.StatusCode)
	}
}

func TestFollowerWriteRejected(t *testing.T) {
	_, urls, cleanup := startLFServers(t, kvsrv.ProfileW5R1)
	defer cleanup()
	resp, err := httpPostJSON(urls[2]+"/kv/set", map[string]string{"key": "x", "value": "y"})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestEmptyKeyRejected(t *testing.T) {
	_, urls, cleanup := startLFServers(t, kvsrv.ProfileW5R1)
	defer cleanup()
	resp, err := httpPostJSON(urls[0]+"/kv/set", map[string]string{"key": "", "value": "y"})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestR3W3_WriteAndRead(t *testing.T) {
	_, urls, cleanup := startLFServers(t, kvsrv.ProfileR3W3)
	defer cleanup()
	leader := urls[0]
	resp, err := httpPostJSON(leader+"/kv/set", map[string]string{"key": "q", "value": "qvz"})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("set: %d", resp.StatusCode)
	}
	r, err := http.Get(urls[4] + "/kv/get?key=q")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("read via follower: %d", r.StatusCode)
	}
	var e struct {
		Value string `json:"value"`
	}
	json.NewDecoder(r.Body).Decode(&e)
	if e.Value != "qvz" {
		t.Fatalf("got %q", e.Value)
	}
}
