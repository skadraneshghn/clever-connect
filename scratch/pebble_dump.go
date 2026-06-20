//go:build ignore

package main

import (
	"fmt"
	"os"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
)

func main() {
	fmt.Println("=== PebbleDB Dump ===")

	cfg := config.LoadConfig()
	database := db.InitDB(cfg)
	_ = database

	if err := pebble.InitPebble("data/pebble_nodes"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: pebble init: %v\n", err)
		os.Exit(1)
	}
	defer pebble.Close()

	for _, mode := range []string{"masque", "wireguard"} {
		endpoints, err := pebble.GetRankedEndpoints(mode)
		if err != nil {
			fmt.Printf("Error getting %s endpoints: %v\n", mode, err)
			continue
		}
		fmt.Printf("\n[Mode: %s] %d endpoints found\n", mode, len(endpoints))
		for i, ep := range endpoints {
			if i >= 10 {
				fmt.Println("  ...")
				break
			}
			fmt.Printf("  [%d] IP:%s, Port:%d, Latency:%.0fms, Score:%.1f, Restricted:%v, Fails:%d\n",
				i+1, ep.IPAddress, ep.Port, ep.LatencyMs, ep.Score, ep.IsRestricted, ep.FailCount)
		}
	}
}
