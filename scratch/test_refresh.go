package main

import (
	"fmt"
	"log"

	"clever-connect/internal/db/pebble"
	"clever-connect/internal/models"
)

func main() {
	if err := pebble.InitPebble("data/pebble_nodes"); err != nil {
		log.Fatalf("failed to initialize pebble: %v", err)
	}
	defer pebble.Close()

	// 1. Initial candidates load
	configs, total := pebble.ListClientConfigs(pebble.ConfigFilter{
		SortBy:     "latency",
		PingStatus: "pass",
	}, 0, 0)
	fmt.Printf("Initial configs: %d\n", total)

	// Simulate buildActivePool
	count := 20
	activeNodes := make([]models.V2RayClientConfig, count)
	copy(activeNodes, configs[:count])

	fmt.Printf("Initial active nodes (top 5):\n")
	for i := 0; i < 5; i++ {
		fmt.Printf("  Active[%d]: Address=%s Port=%d\n", i, activeNodes[i].Address, activeNodes[i].Port)
	}

	// Now simulate refreshCandidatePool
	inUse := make(map[string]struct{})
	for _, n := range activeNodes {
		if n.Address != "" {
			inUse[n.Address] = struct{}{}
		}
	}

	fmt.Printf("inUse map size: %d\n", len(inUse))

	filtered := configs[:0]
	for _, c := range configs {
		if _, used := inUse[c.Address]; !used {
			filtered = append(filtered, c)
		}
	}

	fmt.Printf("Filtered pool size: %d\n", len(filtered))
	if len(filtered) > 0 {
		fmt.Printf("Top filtered candidate: Address=%s Port=%d Name=%s\n", filtered[0].Address, filtered[0].Port, filtered[0].Name)
	} else {
		fmt.Println("Filtered pool is empty!")
	}
}
