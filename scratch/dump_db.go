package main

import (
	"fmt"
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type SoroushAccount struct {
	ID            uint
	PhoneNumber   string
	Name          string
	SoroushUserID int64
	AccessHash    int64
	DisplayName   string
	Role          string
	Status        string
}

type SoroushTunnelConfig struct {
	ID              uint
	GroupChatID     int64
	GroupAccessHash int64
	PSK             string
	ServerHostPhone string
	PairingPIN      string
	LiveKitURL      string
	SocksPort       int
	IsActive        bool
	EngineMode      string
}

func dump(dbPath string) {
	fmt.Println("=========================================")
	fmt.Println("Dumping database:", dbPath)
	fmt.Println("=========================================")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Printf("failed to connect database %s: %v", dbPath, err)
		return
	}

	var accounts []SoroushAccount
	db.Find(&accounts)
	fmt.Println("--- ACCOUNTS ---")
	for _, acct := range accounts {
		fmt.Printf("ID: %d, Phone: %s, Name: %s, UserID: %d, AccessHash: %d, Role: %s, Status: %s\n",
			acct.ID, acct.PhoneNumber, acct.Name, acct.SoroushUserID, acct.AccessHash, acct.Role, acct.Status)
	}

	var configs []SoroushTunnelConfig
	db.Find(&configs)
	fmt.Println("\n--- CONFIGS ---")
	for _, cfg := range configs {
		fmt.Printf("ID: %d, GroupChatID: %d, GroupAccessHash: %d, ServerHostPhone: %s, PairingPIN: %s, LiveKitURL: %s, SocksPort: %d, IsActive: %t, EngineMode: %s\n",
			cfg.ID, cfg.GroupChatID, cfg.GroupAccessHash, cfg.ServerHostPhone, cfg.PairingPIN, cfg.LiveKitURL, cfg.SocksPort, cfg.IsActive, cfg.EngineMode)
	}
}

func main() {
	dump("data/client.db")
	dump("data/server_fallback.db")
}
