package traffic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"

	"github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	prevStats = make(map[string]int64)
	statsMu   sync.Mutex
	running   bool
	stopChan  chan struct{}
)

// StartInterceptor starts the stats polling and quota enforcement loops
func StartInterceptor() {
	statsMu.Lock()
	if running {
		statsMu.Unlock()
		return
	}
	running = true
	stopChan = make(chan struct{})
	statsMu.Unlock()

	logger.Info("V2Ray", "Starting traffic interceptor and quota loops")

	// Stats polling ticker (5 seconds)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if core.IsRunning() {
					if err := PollStats("127.0.0.1:10085"); err != nil {
						logger.Error("V2Ray", "Failed to poll traffic stats", "error", err)
					}
				}
			case <-stopChan:
				return
			}
		}
	}()

	// Quota enforcement loop (60 seconds)
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := EnforceQuotas(); err != nil {
					logger.Error("V2Ray", "Failed to enforce traffic quotas", "error", err)
				}
			case <-stopChan:
				return
			}
		}
	}()
}

// StopInterceptor stops the traffic interceptor and quota loops
func StopInterceptor() {
	statsMu.Lock()
	defer statsMu.Unlock()
	if !running {
		return
	}
	close(stopChan)
	running = false
	logger.Info("V2Ray", "Traffic interceptor stopped")
}

// PollStats queries user traffic stats from the running Xray instance
func PollStats(serverAddr string) error {
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := command.NewStatsServiceClient(conn)
	resp, err := client.QueryStats(context.Background(), &command.QueryStatsRequest{
		Pattern: "user>>>",
		Reset_:  false,
	})
	if err != nil {
		return err
	}

	statsMu.Lock()
	defer statsMu.Unlock()

	tx := db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, stat := range resp.Stat {
		parts := strings.Split(stat.Name, ">>>")
		if len(parts) < 4 {
			continue
		}

		email := parts[1]
		direction := parts[3] // uplink or downlink
		value := stat.Value

		var user models.V2RayUser
		if err := tx.Where("name = ? OR uuid = ?", email, email).First(&user).Error; err != nil {
			continue
		}

		key := fmt.Sprintf("%d_%s", user.ID, direction)
		prevVal := prevStats[key]
		delta := value - prevVal
		if delta < 0 {
			// Xray restarted, reset base reference
			delta = value
		}
		prevStats[key] = value

		if delta > 0 {
			if direction == "uplink" {
				user.UsedUpload += delta
			} else {
				user.UsedDownload += delta
			}
			tx.Save(&user)

			trafficLog := models.V2RayTrafficLog{
				UserID:        user.ID,
				Timestamp:     time.Now(),
				UploadBytes:   condVal(direction == "uplink", delta),
				DownloadBytes: condVal(direction == "downlink", delta),
			}
			tx.Create(&trafficLog)
		}
	}
	tx.Commit()
	return nil
}

func condVal(cond bool, val int64) int64 {
	if cond {
		return val
	}
	return 0
}

// EnforceQuotas checks for users exceeding bandwidth limits or expired dates and deauthorizes them
func EnforceQuotas() error {
	var users []models.V2RayUser
	now := time.Now()

	// Query users that are enabled but should be disabled
	err := db.DB.Where("enabled = ? AND ((traffic_limit > 0 AND (used_upload + used_download) >= traffic_limit) OR (expires_at < ? AND expires_at > ?))", 
		true, now, time.Time{}).Find(&users).Error
	if err != nil {
		return err
	}

	if len(users) == 0 {
		return nil
	}

	logger.Info("V2Ray", "Enforcing quotas: disabling over-limit or expired users", "count", len(users))

	tx := db.DB.Begin()
	for i := range users {
		users[i].Enabled = false
		tx.Save(&users[i])
	}
	tx.Commit()

	// Recompile and hot-reload V2Ray core
	return ReloadCoreConfig()
}

// ReloadCoreConfig recompiles the active database configuration and restarts the Xray core
func ReloadCoreConfig() error {
	var inbounds []models.V2RayInbound
	if err := db.DB.Find(&inbounds).Error; err != nil {
		return err
	}

	var users []models.V2RayUser
	if err := db.DB.Find(&users).Error; err != nil {
		return err
	}

	var rules []models.V2RayRoutingRule
	if err := db.DB.Find(&rules).Error; err != nil {
		return err
	}

	configBytes, err := compiler.CompileServerConfig(inbounds, users, rules)
	if err != nil {
		return fmt.Errorf("failed to compile server config: %w", err)
	}

	// Write compiled config to a temp location
	tempPath := filepath.Join(os.TempDir(), "clever-connect-data", "xray_server.json")
	if err := os.MkdirAll(filepath.Dir(tempPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(tempPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Reload Xray process
	if err := core.StartCore(tempPath); err != nil {
		return fmt.Errorf("failed to restart core process: %w", err)
	}

	return nil
}
