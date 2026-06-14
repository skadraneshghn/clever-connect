package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"clever-connect/internal/v2ray/core"
)

var statsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type RealtimeStats struct {
	UplinkSpeed   int64   `json:"uplinkSpeed"`   // Bytes per second
	DownlinkSpeed int64   `json:"downlinkSpeed"` // Bytes per second
	TotalUplink   int64   `json:"totalUplink"`
	TotalDownlink int64   `json:"totalDownlink"`
	ActiveConns   int     `json:"activeConns"`
	CPUUsage      float64 `json:"cpuUsage"`
	MemoryUsage   int64   `json:"memoryUsage"`
}

// HandleStatsStream upgrades the connection and pumps data to the client
func HandleStatsStream(c *gin.Context) {
	conn, err := statsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Previous values to calculate speed (bytes per second)
	var prevUp, prevDown int64
	var prevUpSet bool

	// Dial gRPC once (it handles reconnection automatically)
	grpcConn, err := grpc.Dial("127.0.0.1:10085", grpc.WithTransportCredentials(insecure.NewCredentials()))
	var client command.StatsServiceClient
	if err == nil {
		defer grpcConn.Close()
		client = command.NewStatsServiceClient(grpcConn)
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			// 1. Get wrapper stats (used for single client, selector, and bonding)
			tx, rx, conns := core.GetClientTraffic()
			currentUp := tx
			currentDown := rx
			activeConns := conns

			// 2. If wrapper traffic is zero (engine might be starting), also try gRPC stats
			if client != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				resp, grpcErr := client.QueryStats(ctx, &command.QueryStatsRequest{
					Pattern: "",
					Reset_:  false,
				})
				cancel()

				if grpcErr == nil && resp != nil {
					var grpcUp, grpcDown int64
					for _, stat := range resp.Stat {
						// aggregate all uplink/downlink values
						if strings.HasSuffix(stat.Name, ">>>uplink") {
							grpcUp += stat.Value
						} else if strings.HasSuffix(stat.Name, ">>>downlink") {
							grpcDown += stat.Value
						}
					}
					// Use whichever source has higher values (they may count different things)
					if grpcUp > currentUp {
						currentUp = grpcUp
					}
					if grpcDown > currentDown {
						currentDown = grpcDown
					}
				}
			}

			// 3. Calculate speed delta
			var upSpeed, downSpeed int64
			if prevUpSet {
				if currentUp >= prevUp {
					upSpeed = currentUp - prevUp
				}
				if currentDown >= prevDown {
					downSpeed = currentDown - prevDown
				}
			}
			prevUp = currentUp
			prevDown = currentDown
			prevUpSet = true

			// 4. Build payload
			stats := RealtimeStats{
				UplinkSpeed:   upSpeed,
				DownlinkSpeed: downSpeed,
				TotalUplink:   currentUp,
				TotalDownlink: currentDown,
				ActiveConns:   activeConns,
			}

			// 5. Stream to React
			if err := conn.WriteJSON(stats); err != nil {
				return // Client disconnected
			}
		}
	}
}
