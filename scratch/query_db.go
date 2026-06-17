package main

import (
	"fmt"
	"log"

	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	sqlite "clever-connect/internal/db/sqlite"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/core"
	"gorm.io/gorm"
)

func main() {
	gormDb, err := gorm.Open(sqlite.Open("data/client.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	db.DB = gormDb

	var settings []models.V2RayClientSetting
	gormDb.Find(&settings)
	fmt.Println("--- DB Settings ---")
	for _, s := range settings {
		fmt.Printf("%s: %s\n", s.Key, s.Value)
	}

	selectedCore := core.GetSelectedCoreName()
	fmt.Printf("\nSelected Core Name: %s\n", selectedCore)

	binPath, err := core.GetBinPathForCore(selectedCore)
	fmt.Printf("Bin Path for selected core: %s (err: %v)\n", binPath, err)

	// Let's also check pebble configs
	pebble.InitPebble("data/pebble_nodes")
	defer pebble.Close()

	configs, count := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
	fmt.Printf("\nPebble client configs count: %d\n", count)
	vlessCount := 0
	for _, c := range configs {
		if c.Protocol == "vless" {
			vlessCount++
			fmt.Printf("ID: %d, Name: %s, Address: %s, Port: %d, Latency: %d, TLSSettings: %s\n",
				c.ID, c.Name, c.Address, c.Port, c.LatencyMs, c.TLSSettings)
		}
	}
	fmt.Printf("Total VLESS configs in PebbleDB: %d\n", vlessCount)
}
