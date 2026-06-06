// Package soroush — this file previously contained AES-GCM encryption functions
// for SDP payloads and the WebRTCTransport yamux wrapper.
//
// With the migration to QUIC over RTP Audio Tracks:
//   - Yamux has been replaced by QUIC's native stream multiplexing
//   - DataChannel transport has been replaced by RtpPacketConn
//   - The HKDF-based handshake and PSK verification functions live in handshake.go
//   - TLS authentication is handled by QUIC's built-in TLS 1.3
package soroush
