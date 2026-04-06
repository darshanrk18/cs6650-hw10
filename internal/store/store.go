// Package store holds an in-memory versioned key-value map.
package store

import "sync"

// Entry is a versioned value for a key.
type Entry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

// Store is a thread-safe in-memory KV store with logical versions per key.
type Store struct {
	mu   sync.RWMutex
	data map[string]Entry
}

// New returns an empty Store.
func New() *Store {
	return &Store{data: make(map[string]Entry)}
}

// Get returns the entry and whether the key exists.
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return e, ok
}

// WriteLocal applies a coordinated write on this node and returns the new version.
func (s *Store) WriteLocal(key, value string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.data[key]
	next := cur.Version + 1
	s.data[key] = Entry{Value: value, Version: next}
	return next
}

// PutVersion applies a replicated write from a coordinator (follower / peer).
func (s *Store) PutVersion(key, value string, ver int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.data[key]
	if ver >= cur.Version {
		s.data[key] = Entry{Value: value, Version: ver}
	}
}
