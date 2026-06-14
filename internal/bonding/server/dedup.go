// Package server implements the server-side bonding combiner for the DMB Engine.
// It accepts multiple artery WebSocket connections from clients, deduplicates
// and reorders incoming frames, dials real destinations, and relays responses
// back through all active arteries.
package server

import "sync"

// DedupRing provides per-stream deduplication to prevent cross-stream cache
// pollution under heavy load. Each StreamID maintains its own independent
// sliding window of seen sequence numbers.
//
// DESIGN NOTE (from expert review): A single global ring buffer is vulnerable
// to high-throughput streams evicting dedup records of low-traffic streams,
// allowing late duplicates to slip through. Per-stream isolation guarantees
// that burst traffic on Stream A never corrupts dedup state for Stream B.
type DedupRing struct {
	mu      sync.Mutex
	streams map[uint32]*streamDedup // StreamID → per-stream dedup state
	perCap  int                     // capacity per stream
	maxStreams int                  // max tracked streams (GC threshold)
}

// streamDedup tracks deduplication state for a single stream.
type streamDedup struct {
	entries map[uint64]struct{} // seen Seq numbers
	ring    []uint64            // eviction ring
	size    int
	idx     int
}

// NewDedupRing creates a dedup ring with the given per-stream capacity.
func NewDedupRing(perStreamCapacity int) *DedupRing {
	if perStreamCapacity <= 0 {
		perStreamCapacity = 512
	}
	return &DedupRing{
		streams:    make(map[uint32]*streamDedup),
		perCap:     perStreamCapacity,
		maxStreams:  4096,
	}
}

// Check returns true if this (StreamID, Seq) is new (first arrival).
// Returns false if it's a duplicate. Thread-safe.
func (d *DedupRing) Check(streamID uint32, seq uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	sd, ok := d.streams[streamID]
	if !ok {
		// First frame for this stream — create per-stream dedup
		sd = &streamDedup{
			entries: make(map[uint64]struct{}, d.perCap),
			ring:    make([]uint64, d.perCap),
			size:    d.perCap,
		}
		d.streams[streamID] = sd

		// GC: if too many streams, prune empty/old ones
		if len(d.streams) > d.maxStreams {
			d.pruneStreamsLocked()
		}
	}

	// Check duplicate within this stream's ring
	if _, exists := sd.entries[seq]; exists {
		return false // duplicate
	}

	// Evict oldest entry in this stream's ring
	old := sd.ring[sd.idx]
	if old != 0 || sd.idx > 0 { // handle zero-seq edge case
		delete(sd.entries, old)
	}

	// Record new entry
	sd.ring[sd.idx] = seq
	sd.entries[seq] = struct{}{}
	sd.idx = (sd.idx + 1) % sd.size

	return true // new
}

// RemoveStream cleans up dedup state for a closed stream.
func (d *DedupRing) RemoveStream(streamID uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.streams, streamID)
}

// Reset clears all dedup state.
func (d *DedupRing) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.streams = make(map[uint32]*streamDedup)
}

// pruneStreamsLocked removes streams with the fewest entries to stay under maxStreams.
// Must be called with d.mu held.
func (d *DedupRing) pruneStreamsLocked() {
	// Simple strategy: remove streams that have used < 10% of their ring capacity
	for id, sd := range d.streams {
		if len(sd.entries) < d.perCap/10 {
			delete(d.streams, id)
		}
		if len(d.streams) <= d.maxStreams/2 {
			break
		}
	}
}

// Stats returns dedup statistics.
type DedupStats struct {
	TrackedStreams int `json:"tracked_streams"`
	PerStreamCap  int `json:"per_stream_cap"`
}

func (d *DedupRing) Stats() DedupStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return DedupStats{
		TrackedStreams: len(d.streams),
		PerStreamCap:  d.perCap,
	}
}
