// Package frame implements the DMB Engine wire protocol for multipath bonding.
//
// Frame layout (per artery, after the artery's own TLS):
//
//	┌────────┬────────┬───────────┬──────────┬──────────┬───────────────┐
//	│ Ver(1) │ Type(1)│ StreamID(4)│ Seq(8)   │ Len(2)   │ Payload(Len)  │
//	└────────┴────────┴───────────┴──────────┴──────────┴───────────────┘
//
// Total header size: 16 bytes.
// Maximum payload size: 65535 bytes (2^16 - 1).
//
// The frame header is always unencrypted at this layer (each artery already
// runs TLS 1.3). Optional end-to-end AEAD is applied at the session layer.
package frame

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Protocol version
const Version uint8 = 1

// HeaderSize is the fixed byte size of every frame header.
const HeaderSize = 16 // 1 + 1 + 4 + 8 + 2

// MaxPayloadSize is the maximum allowed payload per frame (64KB - 1).
const MaxPayloadSize = 65535

// Frame types
const (
	TypeOPEN   uint8 = 0 // payload = "host:port" target address
	TypeDATA   uint8 = 1 // payload = stream bytes
	TypeFIN    uint8 = 2 // half-close this StreamID
	TypeRST    uint8 = 3 // abort this StreamID (payload = optional reason byte)
	TypePING   uint8 = 4 // keepalive / RTT probe (payload = echo nonce + send-timestamp)
	TypeWINDOW uint8 = 5 // flow-control credit update (payload = new window)
)

// TypeName returns a human-readable name for a frame type.
func TypeName(t uint8) string {
	switch t {
	case TypeOPEN:
		return "OPEN"
	case TypeDATA:
		return "DATA"
	case TypeFIN:
		return "FIN"
	case TypeRST:
		return "RST"
	case TypePING:
		return "PING"
	case TypeWINDOW:
		return "WINDOW"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", t)
	}
}

// Frame represents a single DMB wire protocol frame.
type Frame struct {
	Version  uint8  // protocol version (always 1 for now)
	Type     uint8  // frame type (OPEN/DATA/FIN/RST/PING/WINDOW)
	StreamID uint32 // identifies a user connection (4B, monotonic, low-bit = initiator parity)
	Seq      uint64 // connection-level sequence number, independent of path
	Payload  []byte // variable-length data (max 65535 bytes)
}

// Errors
var (
	ErrFrameTooShort    = errors.New("frame: data too short for header")
	ErrPayloadTooLarge  = errors.New("frame: payload exceeds maximum size")
	ErrVersionMismatch  = errors.New("frame: unsupported protocol version")
	ErrInvalidType      = errors.New("frame: invalid frame type")
	ErrPayloadTruncated = errors.New("frame: payload length exceeds available data")
)

// Encode serializes a Frame into its binary wire format.
// Returns a byte slice containing the complete frame (header + payload).
func (f *Frame) Encode() ([]byte, error) {
	payloadLen := len(f.Payload)
	if payloadLen > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	buf := make([]byte, HeaderSize+payloadLen)

	// Header
	buf[0] = f.Version
	buf[1] = f.Type
	binary.BigEndian.PutUint32(buf[2:6], f.StreamID)
	binary.BigEndian.PutUint64(buf[6:14], f.Seq)
	binary.BigEndian.PutUint16(buf[14:16], uint16(payloadLen))

	// Payload
	if payloadLen > 0 {
		copy(buf[HeaderSize:], f.Payload)
	}

	return buf, nil
}

// Decode parses a binary wire format into a Frame.
// The input must contain at least HeaderSize bytes.
func Decode(data []byte) (*Frame, error) {
	if len(data) < HeaderSize {
		return nil, ErrFrameTooShort
	}

	ver := data[0]
	if ver != Version {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrVersionMismatch, ver, Version)
	}

	frameType := data[1]
	if frameType > TypeWINDOW {
		return nil, fmt.Errorf("%w: %d", ErrInvalidType, frameType)
	}

	streamID := binary.BigEndian.Uint32(data[2:6])
	seq := binary.BigEndian.Uint64(data[6:14])
	payloadLen := binary.BigEndian.Uint16(data[14:16])

	totalLen := HeaderSize + int(payloadLen)
	if len(data) < totalLen {
		return nil, ErrPayloadTruncated
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		copy(payload, data[HeaderSize:totalLen])
	}

	return &Frame{
		Version:  ver,
		Type:     frameType,
		StreamID: streamID,
		Seq:      seq,
		Payload:  payload,
	}, nil
}

// ReadFrame reads exactly one frame from a reader (streaming decode).
// It first reads the fixed header, then the variable-length payload.
func ReadFrame(r io.Reader) (*Frame, error) {
	headerBuf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, headerBuf); err != nil {
		return nil, fmt.Errorf("frame: failed to read header: %w", err)
	}

	ver := headerBuf[0]
	if ver != Version {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrVersionMismatch, ver, Version)
	}

	frameType := headerBuf[1]
	if frameType > TypeWINDOW {
		return nil, fmt.Errorf("%w: %d", ErrInvalidType, frameType)
	}

	streamID := binary.BigEndian.Uint32(headerBuf[2:6])
	seq := binary.BigEndian.Uint64(headerBuf[6:14])
	payloadLen := binary.BigEndian.Uint16(headerBuf[14:16])

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("frame: failed to read payload (%d bytes): %w", payloadLen, err)
		}
	}

	return &Frame{
		Version:  ver,
		Type:     frameType,
		StreamID: streamID,
		Seq:      seq,
		Payload:  payload,
	}, nil
}

// WriteFrame serializes and writes a frame to a writer.
func WriteFrame(w io.Writer, f *Frame) error {
	data, err := f.Encode()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// NewDataFrame creates a DATA frame with the given stream ID, sequence, and payload.
func NewDataFrame(streamID uint32, seq uint64, payload []byte) *Frame {
	return &Frame{
		Version:  Version,
		Type:     TypeDATA,
		StreamID: streamID,
		Seq:      seq,
		Payload:  payload,
	}
}

// NewOpenFrame creates an OPEN frame with the target address as payload.
func NewOpenFrame(streamID uint32, seq uint64, targetAddr string) *Frame {
	return &Frame{
		Version:  Version,
		Type:     TypeOPEN,
		StreamID: streamID,
		Seq:      seq,
		Payload:  []byte(targetAddr),
	}
}

// NewFinFrame creates a FIN (half-close) frame.
func NewFinFrame(streamID uint32, seq uint64) *Frame {
	return &Frame{
		Version:  Version,
		Type:     TypeFIN,
		StreamID: streamID,
		Seq:      seq,
	}
}

// NewRstFrame creates a RST (abort) frame with an optional reason byte.
func NewRstFrame(streamID uint32, seq uint64, reason byte) *Frame {
	return &Frame{
		Version:  Version,
		Type:     TypeRST,
		StreamID: streamID,
		Seq:      seq,
		Payload:  []byte{reason},
	}
}

// NewPingFrame creates a PING frame with a nonce and timestamp for RTT measurement.
func NewPingFrame(nonce uint32, sendTimeNs uint64) *Frame {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], nonce)
	binary.BigEndian.PutUint64(payload[4:12], sendTimeNs)
	return &Frame{
		Version:  Version,
		Type:     TypePING,
		StreamID: 0, // PING is session-level, not stream-level
		Seq:      0,
		Payload:  payload,
	}
}

// NewWindowFrame creates a WINDOW frame announcing available receive credit.
func NewWindowFrame(streamID uint32, windowSize uint32) *Frame {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, windowSize)
	return &Frame{
		Version:  Version,
		Type:     TypeWINDOW,
		StreamID: streamID,
		Seq:      0,
		Payload:  payload,
	}
}

// String returns a human-readable representation of the frame for debugging.
func (f *Frame) String() string {
	return fmt.Sprintf("Frame{ver=%d type=%s stream=%d seq=%d payload=%d}",
		f.Version, TypeName(f.Type), f.StreamID, f.Seq, len(f.Payload))
}
