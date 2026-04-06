package kvsrv

import "time"

// Simulated network/storage latencies from the assignment.
const (
	LeaderPauseAfterFollowerMessage = 200 * time.Millisecond
	FollowerPauseOnReplicate        = 100 * time.Millisecond
	FollowerPauseOnPeerRead         = 50 * time.Millisecond
)
