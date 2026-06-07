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

	// Step 1: Fetch dialogs to find the access hash of the chat
	fmt.Printf("\n--- Fetching Dialogs ---\n")
	body := soroushlib.BuildGetDialogsRequest()
	wrapped := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, body)
	_, reader, err := session.SendAndWait(ctx, wrapped, true)
	if err != nil {
		log.Fatalf("getDialogs RPC failed: %v", err)
	}

	raw := reader.GetData()
	targetID := int64(21791372)
	
	var accessHash int64 = 0
	found := false

	// Scan raw bytes for channel constructor: 0x8E87CCD8
	target := []byte{0xd8, 0xcc, 0x87, 0x8e} // 0x8E87CCD8
	for i := 0; i <= len(raw)-32; i++ {
		if raw[i] == target[0] && raw[i+1] == target[1] &&
			raw[i+2] == target[2] && raw[i+3] == target[3] {
			
			cr := soroushlib.NewTLReader(raw[i+4:])
			flags, err1 := cr.ReadInt32()
			flags2, err2 := cr.ReadInt32()
			id, err3 := cr.ReadInt64()
			
			if err1 == nil && err2 == nil && err3 == nil && id == targetID {
				fmt.Printf("Found Channel in Dialogs: ID=%d, Flags=0x%08X, Flags2=0x%08X\n", id, flags, flags2)
				if (flags & (1 << 13)) != 0 {
					hash, err4 := cr.ReadInt64()
					if err4 == nil {
						accessHash = hash
						found = true
						fmt.Printf("  >>> ACCESS HASH = %d (0x%016X) <<<\n", accessHash, uint64(accessHash))
					}
				}
			}
		}
	}

	if !found {
		log.Fatalf("Could not find channel %d in dialogs.", targetID)
	}

	// Step 2: Query channels.getFullChannel
	fmt.Printf("\n--- Querying channels.getFullChannel ---\n")
	wFull := soroushlib.NewTLWriter()
	wFull.WriteUint32(0x08736A09) // channels.getFullChannel
	wFull.WriteUint32(0xf35aec28) // inputChannel
	wFull.WriteInt64(targetID)
	wFull.WriteInt64(accessHash)

	cid, rFull, err := session.SendAndWait(ctx, wFull.GetBytes(), true)
	if err != nil {
		log.Fatalf("getFullChannel RPC failed: %v", err)
	}

	if cid == soroushlib.IDRPCError {
		log.Fatalf("getFullChannel returned RPC error: %v", soroushlib.ParseRPCError(rFull))
	}

	rawFull := rFull.GetData()
	fmt.Printf("getFullChannel succeeded. Response CID: 0x%08X, Response size: %d bytes\n", cid, len(rawFull))
	
	// Dump hex
	fmt.Printf("Raw hex:\n")
	for i := 0; i < len(rawFull); i += 16 {
		end := i + 16
		if end > len(rawFull) {
			end = len(rawFull)
		}
		fmt.Printf("%04d: ", i)
		for _, b := range rawFull[i:end] {
			fmt.Printf("%02x ", b)
		}
		fmt.Printf("\n")
	}

	// List all potential constructor IDs (4-byte alignment or slide by 1 byte)
	fmt.Printf("\nPotential Constructor IDs (sliding):\n")
	for i := 0; i <= len(rawFull)-4; i++ {
		val := uint32(rawFull[i]) | uint32(rawFull[i+1])<<8 | uint32(rawFull[i+2])<<16 | uint32(rawFull[i+3])<<24
		// If it's a known CID or looks like one
		if val == 0xB2264016 {
			fmt.Printf("Found InputConferenceCall (0xB2264016) at offset %d!\n", i)
		} else if val == 0xF36441D5 {
			fmt.Printf("Found ConferenceCall (0xF36441D5) at offset %d!\n", i)
		} else if val == 0x278E3AE9 {
			fmt.Printf("Found conference.conferenceCall (0x278E3AE9) at offset %d!\n", i)
		} else if val == 0xD8AA840F {
			fmt.Printf("Found InputGroupCall (0xD8AA840F) at offset %d!\n", i)
		}
	}
}
