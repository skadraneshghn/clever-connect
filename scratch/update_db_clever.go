package main

import (
	"fmt"
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type BondingEngineConfig struct {
	ID          uint   `gorm:"primaryKey"`
	CombinerURL string `gorm:"column:combiner_url"`
}

func main() {
	db, err := gorm.Open(sqlite.Open("data/client.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	var cfg BondingEngineConfig
	if err := db.First(&cfg).Error; err != nil {
		log.Fatalf("failed to find config: %v", err)
	}

	oldURL := cfg.CombinerURL
	newURL := "wss://app-a8fead43-36bd-4876-b9cb-103c74487ea5.cleverapps.io/ws/bonding/combiner"
	cfg.CombinerURL = newURL

	if err := db.Save(&cfg).Error; err != nil {
		log.Fatalf("failed to update config: %v", err)
	}

	fmt.Printf("Successfully updated CombinerURL in DB:\nOld: %s\nNew: %s\n", oldURL, newURL)
}
