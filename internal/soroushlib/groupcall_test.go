package soroushlib

import (
	"encoding/binary"
	"testing"
)

func TestScanForInputGroupCall(t *testing.T) {
	// 1. Test scanning for inputGroupCall#d8aa840f
	// Constructor ID: 0xD8AA840F (LE: 0f 84 aa d8)
	buf1 := make([]byte, 30)
	// Add some header padding bytes
	buf1[0] = 0x99
	buf1[1] = 0x88
	
	// Constructor
	buf1[2] = 0x0f
	buf1[3] = 0x84
	buf1[4] = 0xaa
	buf1[5] = 0xd8

	// ID (int64: 12345678)
	binary.LittleEndian.PutUint64(buf1[6:14], uint64(12345678))

	// AccessHash (int64: 98765432)
	binary.LittleEndian.PutUint64(buf1[14:22], uint64(98765432))

	id, hash, found := ScanForInputGroupCall(buf1)
	if !found {
		t.Errorf("Expected to find inputGroupCall, but found was false")
	}
	if id != 12345678 {
		t.Errorf("Expected id 12345678, got %d", id)
	}
	if hash != 98765432 {
		t.Errorf("Expected hash 98765432, got %d", hash)
	}

	// 2. Test scanning for live groupCall#d597650c
	// Constructor ID: 0xD597650C (LE: 0c 65 97 d5)
	buf2 := make([]byte, 40)
	// Add header padding bytes
	buf2[0] = 0xaa
	buf2[1] = 0xbb
	buf2[2] = 0xcc

	// Constructor
	buf2[3] = 0x0c
	buf2[4] = 0x65
	buf2[5] = 0x97
	buf2[6] = 0xd5

	// Flags (int32: 0)
	binary.LittleEndian.PutUint32(buf2[7:11], uint32(0))

	// ID (int64: 87654321)
	binary.LittleEndian.PutUint64(buf2[11:19], uint64(87654321))

	// AccessHash (int64: 23456789)
	binary.LittleEndian.PutUint64(buf2[19:27], uint64(23456789))

	id, hash, found = ScanForInputGroupCall(buf2)
	if !found {
		t.Errorf("Expected to find live groupCall, but found was false")
	}
	if id != 87654321 {
		t.Errorf("Expected id 87654321, got %d", id)
	}
	if hash != 23456789 {
		t.Errorf("Expected hash 23456789, got %d", hash)
	}

	// 3. Test noise data
	buf3 := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	_, _, found = ScanForInputGroupCall(buf3)
	if found {
		t.Errorf("Expected no call found in noise data, but got found=true")
	}
}
