package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/darshankonnur/cs6650-hw10/internal/kvsrv"
)

// LoadPeers parses PEERS as comma-separated base URLs (no trailing paths).
func LoadPeers() ([]string, error) {
	raw := os.Getenv("PEERS")
	if raw == "" {
		return nil, fmt.Errorf("PEERS is required")
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("PEERS is empty")
	}
	return out, nil
}

func NodeIndex() (int, error) {
	s := os.Getenv("NODE_INDEX")
	if s == "" {
		return 0, fmt.Errorf("NODE_INDEX is required")
	}
	i, err := strconv.Atoi(s)
	if err != nil || i < 0 {
		return 0, fmt.Errorf("invalid NODE_INDEX")
	}
	return i, nil
}

func QuorumProfile() (kvsrv.QuorumProfile, error) {
	s := strings.TrimSpace(strings.ToLower(os.Getenv("QUORUM_PROFILE")))
	switch s {
	case "w5r1":
		return kvsrv.ProfileW5R1, nil
	case "w1r5":
		return kvsrv.ProfileW1R5, nil
	case "r3w3", "w3r3":
		return kvsrv.ProfileR3W3, nil
	default:
		return "", fmt.Errorf("QUORUM_PROFILE must be w5r1|w1r5|r3w3, got %q", s)
	}
}

func Port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}
