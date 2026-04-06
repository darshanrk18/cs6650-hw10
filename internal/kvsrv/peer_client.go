package kvsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/darshankonnur/cs6650-hw10/internal/store"
)

// PeerClient calls peer internal HTTP APIs.
type PeerClient struct {
	HTTP *http.Client
}

func NewPeerClient() *PeerClient {
	return &PeerClient{
		HTTP: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *PeerClient) Replicate(ctx context.Context, baseURL string, req ReplicateRequest) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/internal/replicate", bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("replicate: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *PeerClient) PeerRead(ctx context.Context, baseURL, key string) (store.Entry, bool, error) {
	u := fmt.Sprintf("%s/internal/peer-read?key=%s", baseURL, url.QueryEscape(key))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return store.Entry{}, false, err
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return store.Entry{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return store.Entry{}, false, fmt.Errorf("peer-read: status %d: %s", resp.StatusCode, string(body))
	}
	var rr ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return store.Entry{}, false, err
	}
	return rr.Entry, rr.Found, nil
}

func (c *PeerClient) ClusterRead(ctx context.Context, leaderBase, key string) (ReadResponse, error) {
	u := fmt.Sprintf("%s/internal/cluster-read?key=%s", leaderBase, url.QueryEscape(key))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ReadResponse{}, err
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ReadResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ReadResponse{}, fmt.Errorf("cluster-read: status %d: %s", resp.StatusCode, string(body))
	}
	var rr ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return ReadResponse{}, err
	}
	return rr, nil
}
