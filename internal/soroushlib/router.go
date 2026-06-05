package soroushlib

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
)

// UpdateMessage represents a raw MTProto update packet
type UpdateMessage struct {
	CID  uint32
	Data []byte
}

// MessageRouter coordinates reading from one MTProto session and broadcasting to subscribers
type MessageRouter struct {
	session          *MTProtoSession
	mu               sync.Mutex
	subsUpdate       []chan UpdateMessage
	subsText         []chan IncomingMessage
	running          bool
	done             chan struct{}
	userAccessHashes map[int64]int64
	accessHashMu     sync.Mutex
}

// NewMessageRouter creates a new MessageRouter
func NewMessageRouter(session *MTProtoSession) *MessageRouter {
	return &MessageRouter{
		session:          session,
		done:             make(chan struct{}),
		userAccessHashes: make(map[int64]int64),
	}
}

// SubscribeUpdate returns a channel receiving raw MTProto updates
func (mr *MessageRouter) SubscribeUpdate() chan UpdateMessage {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	ch := make(chan UpdateMessage, 500)
	mr.subsUpdate = append(mr.subsUpdate, ch)
	return ch
}

// UnsubscribeUpdate removes an update subscription
func (mr *MessageRouter) UnsubscribeUpdate(ch chan UpdateMessage) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	for i, c := range mr.subsUpdate {
		if c == ch {
			mr.subsUpdate = append(mr.subsUpdate[:i], mr.subsUpdate[i+1:]...)
			// Don't close here — Run()'s defer handles cleanup to avoid double-close panic
			break
		}
	}
}

// SubscribeText returns a channel receiving parsed text messages
func (mr *MessageRouter) SubscribeText() chan IncomingMessage {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	ch := make(chan IncomingMessage, 500)
	mr.subsText = append(mr.subsText, ch)
	return ch
}

// UnsubscribeText removes a text subscription
func (mr *MessageRouter) UnsubscribeText(ch chan IncomingMessage) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	for i, c := range mr.subsText {
		if c == ch {
			mr.subsText = append(mr.subsText[:i], mr.subsText[i+1:]...)
			// Don't close here — Run()'s defer handles cleanup to avoid double-close panic
			break
		}
	}
}

// Run starts the read loop from the MTProto session and broadcasts to subscribers
func (mr *MessageRouter) Run(ctx context.Context) error {
	mr.mu.Lock()
	if mr.running {
		mr.mu.Unlock()
		return fmt.Errorf("router already running")
	}
	mr.running = true
	mr.mu.Unlock()

	defer func() {
		mr.mu.Lock()
		mr.running = false
		// Close all subscribers
		for _, ch := range mr.subsUpdate {
			select {
			case <-ch:
			default:
			}
			close(ch)
		}
		mr.subsUpdate = nil
		for _, ch := range mr.subsText {
			select {
			case <-ch:
			default:
			}
			close(ch)
		}
		mr.subsText = nil
		mr.mu.Unlock()
	}()

	mr.session.StartReader(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-mr.session.updateCh:
			if !ok {
				if mr.session.readerErr != nil {
					return mr.session.readerErr
				}
				return fmt.Errorf("session update channel closed")
			}

			cid := msg.CID
			rawBytes := msg.Data
			if rawBytes != nil {
				mr.ScanAndCacheAccessHashes(rawBytes)
			}
			var reader *TLReader
			if rawBytes != nil {
				reader = NewTLReader(rawBytes)
			}

			// Dispatch raw update
			mr.mu.Lock()
			for _, ch := range mr.subsUpdate {
				select {
				case ch <- msg:
				default:
				}
			}
			mr.mu.Unlock()

			// Also check if we can process it as text message updates
			if reader != nil {
				processUpdate(cid, reader, mr.session, func(m IncomingMessage) {
					mr.mu.Lock()
					for _, ch := range mr.subsText {
						select {
						case ch <- m:
						default:
						}
					}
					mr.mu.Unlock()
				})
			}
		}
	}
}

// GetUserAccessHash returns the cached access hash of a Soroush user ID
func (mr *MessageRouter) GetUserAccessHash(userID int64) int64 {
	mr.accessHashMu.Lock()
	defer mr.accessHashMu.Unlock()
	return mr.userAccessHashes[userID]
}

// ScanAndCacheAccessHashes scans raw bytes for user constructor signatures and caches access hashes.
// It scans for Soroush user constructors 0x274DB727 and 0x6A2179DD (layout: CID + flags + flags2 + id + accessHash)
func (mr *MessageRouter) ScanAndCacheAccessHashes(raw []byte) {
	mr.accessHashMu.Lock()
	defer mr.accessHashMu.Unlock()

	targetCIDs := []uint32{0x274DB727, 0x6A2179DD}
	for _, targetCID := range targetCIDs {
		for i := 0; i+28 <= len(raw); i++ {
			cid := binary.LittleEndian.Uint32(raw[i : i+4])
			if cid == targetCID {
				flags := binary.LittleEndian.Uint32(raw[i+4 : i+8])
				// layout: CID (4) + flags (4) + flags2 (4) + id (8) + access_hash (8, if flags bit 0 is set)
				if flags&(1<<0) != 0 {
					if i+28 <= len(raw) {
						id := int64(binary.LittleEndian.Uint64(raw[i+12 : i+20]))
						accessHash := int64(binary.LittleEndian.Uint64(raw[i+20 : i+28]))
						if id > 0 && accessHash != 0 {
							mr.userAccessHashes[id] = accessHash
							if mr.session != nil && mr.session.Logger != nil {
								mr.session.Logger(fmt.Sprintf("[AccessHashCache] Cached user %d -> accessHash=%d (CID 0x%08X)", id, accessHash, cid), "info")
							}
						}
					}
				}
			}
		}
	}
}

// CacheUserAccessHash manually inserts a user access hash into the cache
func (mr *MessageRouter) CacheUserAccessHash(userID int64, accessHash int64) {
	mr.accessHashMu.Lock()
	defer mr.accessHashMu.Unlock()
	mr.userAccessHashes[userID] = accessHash
}
