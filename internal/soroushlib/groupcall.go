package soroushlib

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
)

// ──────────────────────────────────────────────────────────────────────────────
// Group Call MTProto Constructor IDs
// Reverse-engineered from web.splus.ir JS bundle (main.e0f8f960d196367e0c8b.js)
//
// TL Schema (from Soroush's bundled schema):
//   phone.getGroupCall#41845db call:InputGroupCall limit:int = phone.GroupCall
//   phone.joinGroupCall#b132ff7b flags:# muted:flags.0?true video_stopped:flags.2?true
//       call:InputGroupCall join_as:InputPeer invite_hash:flags.1?string params:DataJSON = Updates
//   phone.createGroupCall#48cdc6d8 flags:# rtmp_stream:flags.2?true peer:InputPeer
//       random_id:int title:flags.0?string schedule_date:flags.1?int = Updates
//   phone.leaveGroupCall#500377f9 call:InputGroupCall source:int = Updates
//   phone.discardGroupCall#7a777135 call:InputGroupCall = Updates
//
//   inputGroupCall#d8aa840f id:long access_hash:long = InputGroupCall
//   groupCall#d597650c flags:# ... id:long access_hash:long ... = GroupCall
//   phone.GroupCall#9e727aad call:GroupCall ... = phone.GroupCall
//   updateGroupCallConnection#b783982 flags:# ... params:DataJSON = Update
//   dataJSON#7d748d04 data:string = DataJSON
// ──────────────────────────────────────────────────────────────────────────────

const (
	// Group Call methods
	IDPhoneGetGroupCall  uint32 = 0x041845DB
	IDPhoneJoinGroupCall uint32 = 0xB132FF7B
	IDPhoneCreateGroupCall uint32 = 0x48CDC6D8
	IDPhoneLeaveGroupCall uint32 = 0x500377F9
	IDPhoneDiscardGroupCall uint32 = 0x7A777135

	// Group Call types
	IDInputGroupCall uint32 = 0xD8AA840F
	IDGroupCall      uint32 = 0xD597650C
	IDPhoneGroupCall uint32 = 0x9E727AAD // phone.GroupCall response wrapper
	IDDataJSON       uint32 = 0x7D748D04

	// Group Call updates
	IDUpdateGroupCall  uint32 = 0x14B24500
	IDUpdateGroupCallConnection uint32 = 0x0B783982
)

// GroupCallInfo holds parsed group call state from phone.getGroupCall response
type GroupCallInfo struct {
	ID               int64
	AccessHash       int64
	ParticipantCount int32
	Title            string
	Version          int32
}

// GroupCallToken holds the LiveKit JWT and metadata extracted from
// the updateGroupCallConnection response after phone.joinGroupCall
type GroupCallToken struct {
	JWT       string // LiveKit access_token
	RoomID    string // Extracted from JWT payload (video.room)
	ServerURL string // k.splus.ir:8446
}

// ──────────────────────────────────────────────────────────────────────────────
// TL Builders
// ──────────────────────────────────────────────────────────────────────────────

// BuildGetGroupCallRequest builds phone.getGroupCall TL request
//   phone.getGroupCall#41845db call:InputGroupCall limit:int = phone.GroupCall
func BuildGetGroupCallRequest(groupCallID, accessHash int64) []byte {
	w := NewTLWriter()
	w.WriteUint32(IDPhoneGetGroupCall)
	// call: InputGroupCall
	w.WriteUint32(IDInputGroupCall)
	w.WriteInt64(groupCallID)
	w.WriteInt64(accessHash)
	// limit: int (max participants to return)
	w.WriteInt32(1)
	return w.GetBytes()
}

// BuildJoinGroupCallRequest builds phone.joinGroupCall TL request
//   phone.joinGroupCall#b132ff7b flags:# muted:flags.0?true video_stopped:flags.2?true
//       call:InputGroupCall join_as:InputPeer invite_hash:flags.1?string params:DataJSON = Updates
//
// The params DataJSON must contain an empty JSON object or WebRTC offer.
// For our tunnel we send {"_":"dataJSON","data":""} to signal "data-only" mode.
func BuildJoinGroupCallRequest(groupCallID, accessHash, selfUserID, selfAccessHash int64, muted bool) []byte {
	w := NewTLWriter()
	w.WriteUint32(IDPhoneJoinGroupCall)

	// flags: bit 0 = muted, bit 2 = video_stopped
	flags := int32(0x04) // video_stopped = true (we're data-only, no video)
	if muted {
		flags |= 0x01
	}
	w.WriteInt32(flags)

	// call: InputGroupCall
	w.WriteUint32(IDInputGroupCall)
	w.WriteInt64(groupCallID)
	w.WriteInt64(accessHash)

	// join_as: InputPeer (self)
	w.WriteUint32(IDInputPeerUser)
	w.WriteInt64(selfUserID)
	w.WriteInt64(selfAccessHash)

	// invite_hash is flags.1 — NOT set in our flags, so skip

	// params: DataJSON — empty data signals "give me the token"
	w.WriteUint32(IDDataJSON)
	w.WriteString("") // empty data field

	return w.GetBytes()
}

// BuildLeaveGroupCallRequest builds phone.leaveGroupCall TL request
func BuildLeaveGroupCallRequest(groupCallID, accessHash int64, source int32) []byte {
	w := NewTLWriter()
	w.WriteUint32(IDPhoneLeaveGroupCall)
	w.WriteUint32(IDInputGroupCall)
	w.WriteInt64(groupCallID)
	w.WriteInt64(accessHash)
	w.WriteInt32(source)
	return w.GetBytes()
}

// BuildCreateGroupCallRequest builds phone.createGroupCall TL request
// Used to create a new group call in a chat (admin only)
func BuildCreateGroupCallRequest(chatID int64, accessHash int64) []byte {
	w := NewTLWriter()
	w.WriteUint32(IDPhoneCreateGroupCall)

	// flags = 0 (no rtmp_stream, no title, no schedule)
	w.WriteInt32(0)

	// peer: InputPeer (the chat/group)
	if accessHash != 0 {
		w.WriteUint32(IDInputPeerChannel)
		w.WriteInt64(chatID)
		w.WriteInt64(accessHash)
	} else {
		w.WriteUint32(IDInputPeerChat)
		w.WriteInt64(chatID)
	}

	// random_id
	w.WriteInt32(int32(rand.Int31()))

	return w.GetBytes()
}

// ──────────────────────────────────────────────────────────────────────────────
// Response Parsers
// ──────────────────────────────────────────────────────────────────────────────

// ParseGetGroupCallResponse parses phone.GroupCall response
// Returns the group call info with id and access_hash needed for joinGroupCall
func ParseGetGroupCallResponse(cid uint32, r *TLReader) (*GroupCallInfo, error) {
	if cid == IDRPCError {
		return nil, ParseRPCError(r)
	}

	// phone.GroupCall#9e727aad
	if cid != IDPhoneGroupCall {
		return nil, fmt.Errorf("unexpected response cid=0x%08X, expected phone.GroupCall (0x9E727AAD)", cid)
	}

	// call: GroupCall
	callCID, _ := r.ReadUint32()
	if callCID != IDGroupCall {
		return nil, fmt.Errorf("unexpected call cid=0x%08X, expected groupCall (0xD597650C)", callCID)
	}

	info := &GroupCallInfo{}

	// groupCall#d597650c flags:#
	flags, _ := r.ReadInt32()

	// id: long
	info.ID, _ = r.ReadInt64()
	// access_hash: long
	info.AccessHash, _ = r.ReadInt64()
	// participants_count: int
	info.ParticipantCount, _ = r.ReadInt32()

	// title: flags.3?string
	if flags&(1<<3) != 0 {
		info.Title, _ = r.ReadString()
	}

	// stream_dc_id: flags.4?int
	if flags&(1<<4) != 0 {
		r.ReadInt32()
	}
	// record_start_date: flags.5?int
	if flags&(1<<5) != 0 {
		r.ReadInt32()
	}
	// schedule_date: flags.7?int
	if flags&(1<<7) != 0 {
		r.ReadInt32()
	}
	// unmuted_video_count: flags.10?int
	if flags&(1<<10) != 0 {
		r.ReadInt32()
	}
	// unmuted_video_limit: int
	r.ReadInt32()
	// version: int
	info.Version, _ = r.ReadInt32()

	return info, nil
}

// ParseJoinGroupCallResponse parses the Updates response from phone.joinGroupCall.
// The LiveKit JWT token is embedded inside an updateGroupCallConnection update
// within the Updates wrapper. The token is in params.data as a raw JWT string.
func ParseJoinGroupCallResponse(cid uint32, r *TLReader) (*GroupCallToken, error) {
	if cid == IDRPCError {
		return nil, ParseRPCError(r)
	}

	// The response is Updates#74ae4240 or similar Updates wrapper
	// We need to find updateGroupCallConnection inside the updates vector
	//
	// Walk through the entire remaining response looking for the
	// updateGroupCallConnection constructor ID (0x0B783982)
	data := r.GetData()
	token := scanForGroupCallConnectionToken(data)
	if token != nil {
		return token, nil
	}

	return nil, fmt.Errorf("updateGroupCallConnection not found in joinGroupCall response (cid=0x%08X)", cid)
}

// scanForGroupCallConnectionToken scans raw TL data for the
// updateGroupCallConnection constructor and extracts the DataJSON params.
//
// updateGroupCallConnection#b783982 flags:# presentation:flags.0?true params:DataJSON
// DataJSON#7d748d04 data:string
//
// The data string contains either:
//   - A raw JWT token string
//   - A JSON object like {"token":"...","room":"..."}
func scanForGroupCallConnectionToken(data []byte) *GroupCallToken {
	// Scan for the 4-byte constructor ID 0x0B783982 (LE: 82 39 78 0b)
	target := []byte{0x82, 0x39, 0x78, 0x0b}

	for i := 0; i <= len(data)-4; i++ {
		if data[i] == target[0] && data[i+1] == target[1] &&
			data[i+2] == target[2] && data[i+3] == target[3] {

			// Found updateGroupCallConnection at offset i
			r := NewTLReader(data[i+4:]) // skip the constructor ID

			// flags: #
			flags, err := r.ReadInt32()
			if err != nil {
				continue
			}
			_ = flags

			// params: DataJSON
			dataJSONCID, err := r.ReadUint32()
			if err != nil || dataJSONCID != IDDataJSON {
				continue
			}

			// data: string (the JWT or JSON payload)
			tokenStr, err := r.ReadString()
			if err != nil || tokenStr == "" {
				continue
			}

			return parseGroupCallTokenString(tokenStr)
		}
	}

	return nil
}

// parseGroupCallTokenString extracts the JWT from the DataJSON data string.
// The token may be:
//   - A raw JWT string (starts with "eyJ")
//   - A JSON object with "token" field
func parseGroupCallTokenString(s string) *GroupCallToken {
	token := &GroupCallToken{
		ServerURL: "wss://k.splus.ir:8446",
	}

	// Try raw JWT first
	if len(s) > 3 && s[:3] == "eyJ" {
		token.JWT = s
		token.RoomID = extractRoomFromJWT(s)
		return token
	}

	// Try JSON
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(s), &obj); err == nil {
		if jwt, ok := obj["token"].(string); ok {
			token.JWT = jwt
		} else if jwt, ok := obj["access_token"].(string); ok {
			token.JWT = jwt
		}
		if room, ok := obj["room"].(string); ok {
			token.RoomID = room
		}
		if token.JWT != "" {
			if token.RoomID == "" {
				token.RoomID = extractRoomFromJWT(token.JWT)
			}
			return token
		}
	}

	return nil
}

// extractRoomFromJWT decodes the JWT payload (without verification) to get the room name
func extractRoomFromJWT(jwt string) string {
	// JWT format: header.payload.signature
	// Split by '.' and decode payload (index 1)
	parts := splitJWT(jwt)
	if len(parts) < 2 {
		return ""
	}

	// Base64url decode the payload
	decoded, err := base64URLDecode(parts[1])
	if err != nil {
		return ""
	}

	var payload struct {
		Video struct {
			Room string `json:"room"`
		} `json:"video"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return ""
	}

	return payload.Video.Room
}

func splitJWT(jwt string) []string {
	var parts []string
	start := 0
	for i, c := range jwt {
		if c == '.' {
			parts = append(parts, jwt[start:i])
			start = i + 1
		}
	}
	parts = append(parts, jwt[start:])
	return parts
}

func base64URLDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// ScanForInputGroupCall scans raw TL response bytes for InputGroupCall constructor (0xD8AA840F)
// and returns the group call ID and access hash if found.
func ScanForInputGroupCall(data []byte) (int64, int64, bool) {
	// Little-endian representation of 0xD8AA840F
	target := []byte{0x0f, 0x84, 0xaa, 0xd8}
	for i := 0; i <= len(data)-20; i++ {
		if data[i] == target[0] && data[i+1] == target[1] &&
			data[i+2] == target[2] && data[i+3] == target[3] {
			
			r := NewTLReader(data[i+4:])
			id, err1 := r.ReadInt64()
			accessHash, err2 := r.ReadInt64()
			if err1 == nil && err2 == nil {
				return id, accessHash, true
			}
		}
	}
	return 0, 0, false
}

// ResolveGroupCall fetches the full chat/channel info and scans it to extract
// the active group call ID and access hash.
func ResolveGroupCall(ctx context.Context, session *MTProtoSession, chatID int64, chatAccessHash int64) (int64, int64, error) {
	body := BuildGetFullGroupRequest(chatID, chatAccessHash)
	wrapped := WrapInitConnection(SoroushAppID, body)

	cid, reader, err := session.SendAndWait(ctx, wrapped, true)
	if err != nil {
		return 0, 0, fmt.Errorf("getFullGroup RPC failed: %w", err)
	}

	if cid == IDRPCError {
		return 0, 0, ParseRPCError(reader)
	}

	raw := reader.GetData()
	callID, callAccessHash, found := ScanForInputGroupCall(raw)
	if !found {
		// Debug logging for reverse engineering Soroush's channelFull response
		hexStr := ""
		for i := 0; i < len(raw) && i < 256; i++ {
			hexStr += fmt.Sprintf("%02x ", raw[i])
		}
		fmt.Printf("[DEBUG] ResolveGroupCall raw response length: %d bytes\n", len(raw))
		fmt.Printf("[DEBUG] ResolveGroupCall raw hex (first 256 bytes): %s\n", hexStr)
		
		// Also scan and list potential constructor IDs (4-byte boundaries)
		var cids []string
		for i := 0; i+4 <= len(raw); i += 4 {
			val := uint32(raw[i]) | uint32(raw[i+1])<<8 | uint32(raw[i+2])<<16 | uint32(raw[i+3])<<24
			if val > 0x01000000 && val < 0xffffff00 {
				cids = append(cids, fmt.Sprintf("offset %d: 0x%08X", i, val))
			}
		}
		fmt.Printf("[DEBUG] Potential Constructor IDs in response: %v\n", cids)

		return 0, 0, fmt.Errorf("no active group call found in chat %d", chatID)
	}

	return callID, callAccessHash, nil
}

// CreateGroupCall creates a new group call in the given chat/channel.
func CreateGroupCall(ctx context.Context, session *MTProtoSession, chatID int64, chatAccessHash int64) error {
	body := BuildCreateGroupCallRequest(chatID, chatAccessHash)
	wrapped := WrapInitConnection(SoroushAppID, body)

	cid, reader, err := session.SendAndWait(ctx, wrapped, true)
	if err != nil {
		return fmt.Errorf("createGroupCall RPC failed: %w", err)
	}

	if cid == IDRPCError {
		return ParseRPCError(reader)
	}

	return nil
}
