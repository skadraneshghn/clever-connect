package main

import (
	"fmt"
	"log"

	"clever-connect/internal/db/pebble"
)

func main() {
	if err := pebble.InitPebble("bin/data/pebble_nodes"); err != nil {
		log.Fatalf("failed to initialize pebble: %v", err)
	}
	defer pebble.Close()

	// List passing configs sorted by latency
	configsPass, totalPass := pebble.ListClientConfigs(pebble.ConfigFilter{
		SortBy:     "latency",
		PingStatus: "pass",
	}, 0, 0)
	fmt.Printf("Total passing nodes: %d (listed: %d)\n", totalPass, len(configsPass))
	for i := 0; i < len(configsPass) && i < 20; i++ {
		c := configsPass[i]
		fmt.Printf("Pass [%d]: Name=%s Address=%s Port=%d Latency=%dms\n", i, c.Name, c.Address, c.Port, c.LatencyMs)
	}

	// List all configs
	configsAll, totalAll := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
	fmt.Printf("\nTotal all nodes: %d (listed: %d)\n", totalAll, len(configsAll))
	for i := 0; i < len(configsAll) && i < 5; i++ {
		c := configsAll[i]
		fmt.Printf("All [%d]: Name=%s Address=%s Port=%d Latency=%dms\n", i, c.Name, c.Address, c.Port, c.LatencyMs)
	}
}
