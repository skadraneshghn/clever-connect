package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/models"
	"clever-connect/internal/soroushlib"
	sqlite "clever-connect/internal/db/sqlite"
	"gorm.io/gorm"
)

func main() {
	dbPath := "data/client.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	gormDb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database %s: %v", dbPath, err)
	}

	db.DB = gormDb

	var acct models.SoroushAccount
	if err := gormDb.First(&acct).Error; err != nil {
		log.Fatalf("failed to query Soroush account: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	session, transport := soroushlib.RestoreSession(acct.AuthKey, acct.AuthKeyID, acct.ServerSalt)
	if err := transport.Connect(ctx); err != nil {
		log.Fatalf("failed to connect to Soroush: %v", err)
	}
	defer transport.Disconnect()

	// Capture updates by intercepting unsolicited updates in our own loops
	fmt.Printf("Connected successfully. Warming up session...\n")
	if err := session.WarmUpSession(ctx); err != nil {
		log.Fatalf("failed to warm up Soroush session: %v", err)
	}

	// Start a goroutine to read from session updates or print incoming raw updates
	fmt.Printf("\n--- Listening for unsolicited updates for 20 seconds ---\n")
	
	// Let's call updates.getState first
	getStateBody := []byte{0xc7, 0x7c, 0xc1, 0xed} // updates.getState#edd17cc7
	wrapped := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, getStateBody)
	_, reader, err := session.SendAndWait(ctx, wrapped, true)
	if err == nil {
		fmt.Printf("updates.getState succeeded. Response CID: 0x%08X, size: %d bytes\n",
			binaryReaderCID(reader.GetData()), len(reader.GetData()))
		parseAndPrintCall(reader.GetData())
	} else {
		fmt.Printf("updates.getState failed: %v\n", err)
	}

	// Keep connection alive and print any incoming packets
	startTime := time.Now()
	for time.Since(startTime) < 20*time.Second {
		// Read a frame from transport
		frame, err := transport.Recv(ctx)
		if err != nil {
			fmt.Printf("Recv error: %v\n", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(frame) < 8 {
			continue
		}
		
		cid := binaryReaderCID(frame)
		fmt.Printf("[%s] Received raw frame: size=%d, CID=0x%08X\n",
			time.Now().Format("15:04:05.000"), len(frame), cid)
		
		parseAndPrintCall(frame)
	}
}

func binaryReaderCID(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}
	// Try offsets to find a valid MTProto CID
	for _, offset := range []int{0, 8, 16, 24, 32} {
		if offset+4 <= len(data) {
			val := uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
			if val == 0x74AE4240 || val == 0x1CB5C415 || val == 0x780E4B66 || val == 0x90C87230 || val == 0xD597650C || val == 0xD8AA840F || val == 0x0B783982 || val == 0x14B24500 {
				return val
			}
		}
	}
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
}

func parseAndPrintCall(raw []byte) {
	// Look for InputGroupCall constructor: 0xD8AA840F (LE: 0f 84 aa d8)
	target := []byte{0x0f, 0x84, 0xaa, 0xd8}
	found := false
	for i := 0; i <= len(raw)-20; i++ {
		if raw[i] == target[0] && raw[i+1] == target[1] && raw[i+2] == target[2] && raw[i+3] == target[3] {
			cr := soroushlib.NewTLReader(raw[i+4:])
			id, err1 := cr.ReadInt64()
			accessHash, err2 := cr.ReadInt64()
			if err1 == nil && err2 == nil {
				fmt.Printf("  >>> FOUND ACTIVE CALL IN RESPONSE: ID=%d, AccessHash=%d <<<\n", id, accessHash)
				found = true
			}
		}
	}

	// Also look for GroupCall constructor: 0xD597650C (LE: 0c 65 97 d5)
	gcTarget := []byte{0x0c, 0x65, 0x97, 0xd5}
	for i := 0; i <= len(raw)-20; i++ {
		if raw[i] == gcTarget[0] && raw[i+1] == gcTarget[1] && raw[i+2] == gcTarget[2] && raw[i+3] == gcTarget[3] {
			cr := soroushlib.NewTLReader(raw[i+4:])
			flags, _ := cr.ReadInt32()
			id, _ := cr.ReadInt64()
			accessHash, _ := cr.ReadInt64()
			fmt.Printf("  >>> FOUND groupCall OBJECT IN RESPONSE: ID=%d, AccessHash=%d, Flags=0x%08X <<<\n", id, accessHash, flags)
			found = true
		}
	}

	// Also look for updateGroupCall: 0x14B24500 (LE: 00 45 b2 14)
	ugcTarget := []byte{0x00, 0x45, 0xb2, 0x14}
	for i := 0; i <= len(raw)-20; i++ {
		if ugcTarget[0] == raw[i] && ugcTarget[1] == raw[i+1] && ugcTarget[2] == raw[i+2] && ugcTarget[3] == raw[i+3] {
			fmt.Printf("  >>> FOUND updateGroupCall CONSTRUCTOR AT OFFSET %d <<<\n", i)
			found = true
		}
	}

	// Also look for updateGroupCallConnection: 0x0B783982 (LE: 82 39 78 0b)
	ugccTarget := []byte{0x82, 0x39, 0x78, 0x0b}
	for i := 0; i <= len(raw)-20; i++ {
		if ugccTarget[0] == raw[i] && ugccTarget[1] == raw[i+1] && ugccTarget[2] == raw[i+2] && ugccTarget[3] == raw[i+3] {
			fmt.Printf("  >>> FOUND updateGroupCallConnection CONSTRUCTOR AT OFFSET %d <<<\n", i)
			found = true
		}
	}

	if !found {
		// Scan and list potential constructor IDs
		var cids []string
		for i := 0; i+4 <= len(raw); i += 4 {
			val := uint32(raw[i]) | uint32(raw[i+1])<<8 | uint32(raw[i+2])<<16 | uint32(raw[i+3])<<24
			if val > 0x01000000 && val < 0xffffff00 {
				cids = append(cids, fmt.Sprintf("offset %d: 0x%08X", i, val))
			}
		}
		if len(cids) > 0 {
			fmt.Printf("  Potential Constructor IDs: %v\n", cids)
		}
	}
}
