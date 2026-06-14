// Package server implements the server-side bonding combiner for the DMB Engine.
// It accepts multiple artery WebSocket connections from clients, deduplicates
// and reorders incoming frames, dials real destinations, and relays responses
// back through all active arteries.
package server

import "sync"

// DedupRing is a fast deduplication check for (StreamID, Seq) tuples.
// It uses a bounded ring buffer to track recently seen frame identifiers
// and reject duplicates before they reach the session layer.
type DedupRing struct {
	mu      sync.Mutex
	entries map[dedupKey]struct{}
	ring    []dedupKey
	size    int
	idx     int
}

type dedupKey struct {
	StreamID uint32
	Seq      uint64
}

// NewDedupRing creates a dedup ring with the given capacity.
// The ring evicts the oldest entry when full.
func NewDedupRing(capacity int) *DedupRing {
	if capacity <= 0 {
		capacity = 8192
	}
	return &DedupRing{
		entries: make(map[dedupKey]struct{}, capacity),
		ring:    make([]dedupKey, capacity),
		size:    capacity,
	}
}

// Check returns true if this (StreamID, Seq) is new (first arrival).
// Returns false if it's a duplicate. Thread-safe.
func (d *DedupRing) Check(streamID uint32, seq uint64) bool {
	key := dedupKey{StreamID: streamID, Seq: seq}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.entries[key]; exists {
		return false // duplicate
	}

	// Evict the oldest entry at ring[idx] before overwriting
	old := d.ring[d.idx]
	if old != (dedupKey{}) {
		delete(d.entries, old)
	}

	// Record new entry
	d.ring[d.idx] = key
	d.entries[key] = struct{}{}
	d.idx = (d.idx + 1) % d.size

	return true // new
}

// Reset clears the dedup ring.
func (d *DedupRing) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries = make(map[dedupKey]struct{}, d.size)
	d.ring = make([]dedupKey, d.size)
	d.idx = 0
}
