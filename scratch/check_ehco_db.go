package main

import (
	"fmt"

	"clever-connect/internal/models"
	sqlite "clever-connect/internal/db/sqlite"
	"gorm.io/gorm"
)

func main() {
	// Query client config
	clientDb, err := gorm.Open(sqlite.Open("data/client.db"), &gorm.Config{})
	if err == nil {
		var clientCfg models.EhcoClientConfig
		if err := clientDb.First(&clientCfg).Error; err == nil {
			fmt.Println("=== Client DB Ehco Config ===")
			fmt.Printf("RemoteURL: %s\n", clientCfg.RemoteURL)
			fmt.Printf("SecondaryURL: %s\n", clientCfg.SecondaryURL)
			fmt.Printf("AuthToken: %s\n", clientCfg.AuthToken)
			fmt.Printf("EnableMux: %t\n", clientCfg.EnableMux)
			fmt.Printf("IsActive: %t\n", clientCfg.IsActive)
		} else {
			fmt.Printf("Client config query error: %v\n", err)
		}
	} else {
		fmt.Printf("Failed to open client DB: %v\n", err)
	}

	// Query server fallback db if exists
	serverDb, err := gorm.Open(sqlite.Open("data/server_fallback.db"), &gorm.Config{})
	if err == nil {
		var serverCfg models.EhcoServerConfig
		if err := serverDb.First(&serverCfg).Error; err == nil {
			fmt.Println("=== Server Fallback DB Ehco Config ===")
			fmt.Printf("ListenPort: %s\n", serverCfg.ListenPort)
			fmt.Printf("AuthToken: %s\n", serverCfg.AuthToken)
			fmt.Printf("IsActive: %t\n", serverCfg.IsActive)
		} else {
			fmt.Printf("Server config query error: %v\n", err)
		}
	} else {
		fmt.Printf("Failed to open server fallback DB: %v\n", err)
	}
}
