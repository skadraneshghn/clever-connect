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

	fmt.Printf("Connected successfully. Warming up session...\n")
	if err := session.WarmUpSession(ctx); err != nil {
		log.Fatalf("failed to warm up Soroush session: %v", err)
	}

	chatID := int64(21791372)

	// Step 1: Call messages.getFullChat
	fmt.Printf("\n--- Method 2: messages.getFullChat ---\n")
	bodyFullChat := soroushlib.BuildGetFullGroupRequest(chatID, 0)
	wrappedFChat := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, bodyFullChat)
	cidFChat, readerFChat, err := session.SendAndWait(ctx, wrappedFChat, true)
	if err == nil {
		fmt.Printf("getFullChat succeeded. CID: 0x%08X, size: %d bytes\n", cidFChat, len(readerFChat.GetData()))
		dumpBytesAndInts(readerFChat.GetData())
	} else {
		fmt.Printf("getFullChat failed: %v\n", err)
	}

	// Step 2: Fetch Chat History as chat (accessHash = 0)
	fmt.Printf("\n--- Method 3: messages.getHistory (chat mode) ---\n")
	historyBody := soroushlib.BuildGetHistoryRequest(chatID, 0, 0, 0, 0, 100)
	wrappedHist := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, historyBody)
	cidHist, readerHist, err := session.SendAndWait(ctx, wrappedHist, true)
	if err == nil {
		fmt.Printf("getHistory succeeded. CID: 0x%08X, size: %d bytes\n", cidHist, len(readerHist.GetData()))
		parseAndPrintCall(readerHist.GetData())
	} else {
		fmt.Printf("getHistory failed: %v\n", err)
	}
}

func dumpBytesAndInts(data []byte) {
	fmt.Printf("--- Raw data offsets, uint32 and int64 values ---\n")
	for i := 0; i+4 <= len(data); i += 4 {
		val := uint32(data[i]) | uint32(data[i+1])<<8 | uint32(data[i+2])<<16 | uint32(data[i+3])<<24
		
		// If 8-byte aligned, print as int64 too
		var int64str string
		if i%8 == 0 && i+8 <= len(data) {
			val64 := int64(uint64(data[i]) | uint64(data[i+1])<<8 | uint64(data[i+2])<<16 | uint64(data[i+3])<<24 |
				uint64(data[i+4])<<32 | uint64(data[i+5])<<40 | uint64(data[i+6])<<48 | uint64(data[i+7])<<56)
			int64str = fmt.Sprintf(" | int64: %d (0x%016X)", val64, uint64(val64))
		}
		
		// Also show hex representation of the 4 bytes
		hexBytes := fmt.Sprintf("%02x %02x %02x %02x", data[i], data[i+1], data[i+2], data[i+3])
		fmt.Printf("Offset %3d: hex: %s | uint32: 0x%08X (%d)%s\n", i, hexBytes, val, val, int64str)
	}
}

func parseAndPrintCall(raw []byte) {
	target := []byte{0x0f, 0x84, 0xaa, 0xd8} // InputGroupCall (d8aa840f)
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

	gcTarget := []byte{0x0c, 0x65, 0x97, 0xd5} // groupCall (d597650c)
	for i := 0; i <= len(raw)-20; i++ {
		if raw[i] == gcTarget[0] && raw[i+1] == gcTarget[1] && raw[i+2] == gcTarget[2] && raw[i+3] == gcTarget[3] {
			cr := soroushlib.NewTLReader(raw[i+4:])
			flags, _ := cr.ReadInt32()
			id, _ := cr.ReadInt64()
			accessHash, _ := cr.ReadInt64()
			fmt.Printf("  >>> FOUND groupCall OBJECT: ID=%d, AccessHash=%d, Flags=0x%08X <<<\n", id, accessHash, flags)
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
		if len(cids) > 10 {
			fmt.Printf("  Potential Constructor IDs (first 10): %v\n", cids[:10])
		} else if len(cids) > 0 {
			fmt.Printf("  Potential Constructor IDs: %v\n", cids)
		}
	}
}
