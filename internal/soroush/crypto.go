package soroush

// This file previously contained AES-GCM encryption functions for SDP payloads
// exchanged over MTProto text messages. With the migration to LiveKit SFU
// DataChannels, all SDP signaling has been removed. The HKDF-based handshake
// and PSK verification functions live in handshake.go.
//
// The AES-GCM functions (EncryptPayload, DecryptPayload, EncryptSDP, DecryptSDP)
// have been deleted as they are no longer referenced by any production code.
