package kvsrv

import "github.com/darshankonnur/cs6650-hw10/internal/store"

// ReplicateRequest is the internal replication payload.
type ReplicateRequest struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

// ReadResponse is returned by peer read endpoints.
type ReadResponse struct {
	Found   bool         `json:"found"`
	Entry   store.Entry  `json:"entry"`
	Key     string       `json:"key,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// SetRequest is the public set body.
type SetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
