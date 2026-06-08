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
	isFirstTick := true

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
			var currentUp, currentDown int64
			var activeConns int

			if client != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				resp, err := client.QueryStats(ctx, &command.QueryStatsRequest{
					Pattern: "",
					Reset_:  false,
				})
				cancel()

				if err == nil && resp != nil {
					for _, stat := range resp.Stat {
						// aggregate all uplink/downlink values
						if strings.HasSuffix(stat.Name, ">>>uplink") {
							currentUp += stat.Value
						} else if strings.HasSuffix(stat.Name, ">>>downlink") {
							currentDown += stat.Value
						}
					}
				}
			}

			// 2. Calculate speed
			var upSpeed, downSpeed int64
			if !isFirstTick {
				if currentUp >= prevUp {
					upSpeed = currentUp - prevUp
				}
				if currentDown >= prevDown {
					downSpeed = currentDown - prevDown
				}
			}

			if currentUp > 0 || currentDown > 0 {
				isFirstTick = false
				prevUp = currentUp
				prevDown = currentDown
			}

			// 3. Build payload
			stats := RealtimeStats{
				UplinkSpeed:   upSpeed,
				DownlinkSpeed: downSpeed,
				TotalUplink:   currentUp,
				TotalDownlink: currentDown,
				ActiveConns:   activeConns,
			}

			// 4. Stream to React
			if err := conn.WriteJSON(stats); err != nil {
				return // Client disconnected
			}
		}
	}
}
