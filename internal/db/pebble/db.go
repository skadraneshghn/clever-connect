package pebble

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/models"

	"github.com/cockroachdb/pebble"
	"gorm.io/gorm"
)

var (
	DB     *pebble.DB
	nextID uint
	mu     sync.Mutex
)

// InitPebble initializes the PebbleDB instance.
func InitPebble(path string) error {
	var err error
	DB, err = pebble.Open(path, &pebble.Options{})
	if err != nil {
		return err
	}

	// Initialize ID counter by finding the max ID
	nextID = 1
	iter, err := DB.NewIter(nil)
	if err == nil {
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			if id, err := strconv.ParseUint(key, 10, 32); err == nil {
				if uint(id) >= nextID {
					nextID = uint(id) + 1
				}
			}
		}
		iter.Close()
	}

	return nil
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}

// MigrateFromSQLite migrates V2RayClientConfig records from SQLite to PebbleDB and drops the SQLite table.
func MigrateFromSQLite(sqliteDB *gorm.DB) error {
	if sqliteDB == nil || DB == nil {
		return nil
	}

	// Check if table exists
	if !sqliteDB.Migrator().HasTable(&models.V2RayClientConfig{}) {
		return nil
	}

	var configs []models.V2RayClientConfig
	// Read all from SQLite
	if err := sqliteDB.Find(&configs).Error; err != nil {
		return err
	}

	if len(configs) > 0 {
		log.Printf("Migrating %d V2Ray client configs from SQLite to PebbleDB...", len(configs))
		batch := DB.NewBatch()
		
		maxID := nextID
		for _, cfg := range configs {
			key := []byte(fmt.Sprintf("%d", cfg.ID))
			val, err := json.Marshal(cfg)
			if err == nil {
				batch.Set(key, val, pebble.Sync)
			}
			if cfg.ID >= maxID {
				maxID = cfg.ID + 1
			}
		}
		
		if err := batch.Commit(pebble.Sync); err != nil {
			return err
		}
		batch.Close()

		mu.Lock()
		if maxID > nextID {
			nextID = maxID
		}
		mu.Unlock()
		log.Println("PebbleDB migration complete.")
	}

	// Drop SQLite table
	if err := sqliteDB.Migrator().DropTable(&models.V2RayClientConfig{}); err != nil {
		log.Printf("Warning: failed to drop SQLite table V2RayClientConfig: %v", err)
	}

	return nil
}

// SaveClientConfig saves a config. If ID is 0, it creates a new one.
func SaveClientConfig(cfg *models.V2RayClientConfig) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	mu.Lock()
	if cfg.ID == 0 {
		cfg.ID = nextID
		nextID++
		cfg.CreatedAt = time.Now()
	}
	cfg.UpdatedAt = time.Now()
	mu.Unlock()

	key := []byte(fmt.Sprintf("%d", cfg.ID))
	val, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return DB.Set(key, val, pebble.Sync)
}

// SaveClientConfigsBulk saves multiple configs atomically.
func SaveClientConfigsBulk(configs []models.V2RayClientConfig) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	if len(configs) == 0 {
		return nil
	}

	batch := DB.NewBatch()
	
	mu.Lock()
	for i := range configs {
		if configs[i].ID == 0 {
			configs[i].ID = nextID
			nextID++
			configs[i].CreatedAt = time.Now()
		}
		configs[i].UpdatedAt = time.Now()
		
		key := []byte(fmt.Sprintf("%d", configs[i].ID))
		val, _ := json.Marshal(configs[i])
		batch.Set(key, val, pebble.Sync)
	}
	mu.Unlock()

	err := batch.Commit(pebble.Sync)
	batch.Close()
	return err
}

// GetClientConfig retrieves a config by ID.
func GetClientConfig(id uint) (*models.V2RayClientConfig, error) {
	if DB == nil {
		return nil, fmt.Errorf("pebble database is not initialized")
	}
	key := []byte(fmt.Sprintf("%d", id))
	val, closer, err := DB.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var cfg models.V2RayClientConfig
	if err := json.Unmarshal(val, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DeleteClientConfig removes a config by ID.
func DeleteClientConfig(id uint) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	key := []byte(fmt.Sprintf("%d", id))
	return DB.Delete(key, pebble.Sync)
}

// DeleteAllClientConfigs removes all configs.
func DeleteAllClientConfigs() error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	iter, err := DB.NewIter(nil)
	if err != nil {
		return err
	}
	defer iter.Close()

	batch := DB.NewBatch()
	for iter.First(); iter.Valid(); iter.Next() {
		batch.Delete(iter.Key(), pebble.Sync)
	}
	
	err = batch.Commit(pebble.Sync)
	batch.Close()
	
	mu.Lock()
	nextID = 1
	mu.Unlock()
	
	return err
}

// ConfigFilter contains options to filter configurations in PebbleDB
type ConfigFilter struct {
	Search         string   `json:"search"`
	SubscriptionID *uint    `json:"subscription_id"`
	CategoryID     *uint    `json:"category_id"`
	Protocol       string   `json:"protocol"`
	Network        string   `json:"network"`
	Port           int      `json:"port"`
	PingStatus     string   `json:"ping_status"` // "all", "pass", "fail"
	SortBy         string   `json:"sort_by"`     // "priority", "speed" or "latency"
}

// ListClientConfigs returns configs with advanced filtering and pagination.
func ListClientConfigs(filter ConfigFilter, offset, limit int) ([]models.V2RayClientConfig, int) {
	var all []models.V2RayClientConfig
	if DB == nil {
		return all, 0
	}
	
	iter, err := DB.NewIter(nil)
	if err != nil {
		return all, 0
	}
	defer iter.Close()

	searchLower := strings.ToLower(strings.TrimSpace(filter.Search))
	protoLower := strings.ToLower(strings.TrimSpace(filter.Protocol))
	netLower := strings.ToLower(strings.TrimSpace(filter.Network))

	for iter.First(); iter.Valid(); iter.Next() {
		var cfg models.V2RayClientConfig
		if err := json.Unmarshal(iter.Value(), &cfg); err == nil {
			// Subscription ID filter
			if filter.SubscriptionID != nil && cfg.SubscriptionID != *filter.SubscriptionID {
				continue
			}
			// Category ID filter
			if filter.CategoryID != nil && cfg.CategoryID != *filter.CategoryID {
				continue
			}
			// Protocol filter
			if protoLower != "" && strings.ToLower(cfg.Protocol) != protoLower {
				continue
			}
			// Network filter
			if netLower != "" && strings.ToLower(cfg.Network) != netLower {
				continue
			}
			// Port filter
			if filter.Port > 0 && cfg.Port != filter.Port {
				continue
			}
			// Ping Status filter
			if filter.PingStatus != "" && filter.PingStatus != "all" {
				if filter.PingStatus == "pass" && cfg.LatencyMs <= 0 {
					continue
				}
				if filter.PingStatus == "fail" && cfg.LatencyMs > 0 {
					continue
				}
			}
			// Generic text search (name, address, uuid)
			if searchLower != "" {
				nameMatch := strings.Contains(strings.ToLower(cfg.Name), searchLower)
				addrMatch := strings.Contains(strings.ToLower(cfg.Address), searchLower)
				uuidMatch := strings.Contains(strings.ToLower(cfg.UUID), searchLower)
				if !nameMatch && !addrMatch && !uuidMatch {
					continue
				}
			}
			all = append(all, cfg)
		}
	}

	// Sort by chosen field
	if filter.SortBy == "speed" || filter.SortBy == "latency" {
		sort.Slice(all, func(i, j int) bool {
			li := all[i].LatencyMs
			lj := all[j].LatencyMs
			if li <= 0 && lj <= 0 {
				return all[i].Name < all[j].Name
			}
			if li <= 0 {
				return false
			}
			if lj <= 0 {
				return true
			}
			if li == lj {
				return all[i].Name < all[j].Name
			}
			return li < lj
		})
	} else {
		// Sort by priority asc, then name asc
		sort.Slice(all, func(i, j int) bool {
			if all[i].Priority == all[j].Priority {
				return all[i].Name < all[j].Name
			}
			return all[i].Priority < all[j].Priority
		})
	}

	total := len(all)

	// Apply pagination
	if limit > 0 {
		if offset >= total {
			return []models.V2RayClientConfig{}, total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		return all[offset:end], total
	}

	return all, total
}

// DeleteFailedClientConfigs deletes all configs with latency_ms < 0 (i.e. -1 for failed)
func DeleteFailedClientConfigs() (int, error) {
	if DB == nil {
		return 0, fmt.Errorf("pebble database is not initialized")
	}
	configs, _ := ListClientConfigs(ConfigFilter{}, 0, 0)
	count := 0
	batch := DB.NewBatch()
	for _, cfg := range configs {
		if cfg.LatencyMs < 0 {
			key := []byte(fmt.Sprintf("%d", cfg.ID))
			batch.Delete(key, pebble.Sync)
			count++
		}
	}
	err := batch.Commit(pebble.Sync)
	batch.Close()
	return count, err
}

// DeleteDiscoveredClientConfigs deletes all configs with name starting with "Discovered-"
func DeleteDiscoveredClientConfigs() (int, error) {
	if DB == nil {
		return 0, fmt.Errorf("pebble database is not initialized")
	}
	configs, _ := ListClientConfigs(ConfigFilter{}, 0, 0)
	count := 0
	batch := DB.NewBatch()
	for _, cfg := range configs {
		if len(cfg.Name) >= 11 && cfg.Name[:11] == "Discovered-" {
			key := []byte(fmt.Sprintf("%d", cfg.ID))
			batch.Delete(key, pebble.Sync)
			count++
		}
	}
	err := batch.Commit(pebble.Sync)
	batch.Close()
	return count, err
}

// ──────────────────────────────────────────────────────────────────────────────
// WARP+ Scan Result KV Helpers (Prefixed Key Namespace)
// ──────────────────────────────────────────────────────────────────────────────

// WarpScanResult represents a scanned Cloudflare edge endpoint.
type WarpScanResult struct {
	IPAddress             string   `json:"ip_address"`
	Port                  int      `json:"port"`
	LatencyMs             float64  `json:"latency_ms"`
	PacketLoss            float64  `json:"packet_loss"`
	ThroughputBytesPerSec float64  `json:"throughput_bps"`
	SupportedALPNs        []string `json:"supported_alpns"`
	LastScanned           string   `json:"last_scanned"`
	IsRestricted          bool     `json:"is_restricted"` // Flagged by trace validator
}

// warpKeyPrefix returns the PebbleDB key prefix for a given transport mode.
func warpKeyPrefix(mode string) string {
	if mode == "wireguard" || mode == "wg" {
		return "cf:node:wg:"
	}
	return "cf:node:masque:"
}

// warpKey builds a full PebbleDB key for a WARP scan result.
func warpKey(mode, ipAddress string, port int) []byte {
	prefix := warpKeyPrefix(mode)
	return []byte(fmt.Sprintf("%s%s:%d", prefix, ipAddress, port))
}

// SaveWarpScanResult saves a scan result to PebbleDB under the appropriate prefix.
func SaveWarpScanResult(mode string, result *WarpScanResult) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	key := warpKey(mode, result.IPAddress, result.Port)
	val, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return DB.Set(key, val, pebble.Sync)
}

// SaveWarpScanResultsBulk saves multiple scan results atomically.
func SaveWarpScanResultsBulk(mode string, results []WarpScanResult) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	if len(results) == 0 {
		return nil
	}

	batch := DB.NewBatch()
	for i := range results {
		key := warpKey(mode, results[i].IPAddress, results[i].Port)
		val, err := json.Marshal(results[i])
		if err != nil {
			continue
		}
		batch.Set(key, val, pebble.Sync)
	}

	err := batch.Commit(pebble.Sync)
	batch.Close()
	return err
}

// ListWarpScanResults returns all scan results for a given transport mode,
// sorted by latency ascending (best first). Non-restricted only by default.
func ListWarpScanResults(mode string) ([]WarpScanResult, error) {
	if DB == nil {
		return nil, fmt.Errorf("pebble database is not initialized")
	}

	prefix := []byte(warpKeyPrefix(mode))
	upperBound := func(b []byte) []byte {
		end := make([]byte, len(b))
		copy(end, b)
		for i := len(end) - 1; i >= 0; i-- {
			end[i]++
			if end[i] != 0 {
				break
			}
		}
		return end
	}

	iter, err := DB.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []WarpScanResult
	for iter.First(); iter.Valid(); iter.Next() {
		var r WarpScanResult
		if err := json.Unmarshal(iter.Value(), &r); err == nil {
			results = append(results, r)
		}
	}

	// Sort by latency ascending (lowest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].LatencyMs < results[j].LatencyMs
	})

	return results, nil
}

// GetBestWarpEndpoint returns the lowest-latency endpoint.
// It first prefers endpoints where QUIC/UDP is available (IsRestricted=false).
// If none are available (e.g. ISP blocks UDP), it falls back to TCP-only
// (IsRestricted=true) endpoints so the engine can still attempt a connection.
func GetBestWarpEndpoint(mode string) (*WarpScanResult, error) {
	results, err := ListWarpScanResults(mode)
	if err != nil {
		return nil, err
	}

	// Prefer QUIC-capable endpoints first
	for _, r := range results {
		if !r.IsRestricted {
			return &r, nil
		}
	}

	// Fall back to TCP-only (restricted) endpoints
	for _, r := range results {
		return &r, nil // already sorted by latency; pick lowest
	}

	return nil, fmt.Errorf("no available WARP endpoints found for mode %s (run a scan first)", mode)
}

// MarkWarpEndpointRestricted flags a specific endpoint as restricted in PebbleDB.
func MarkWarpEndpointRestricted(mode, ipAddress string, port int) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	key := warpKey(mode, ipAddress, port)
	val, closer, err := DB.Get(key)
	if err != nil {
		return err
	}

	var r WarpScanResult
	if err := json.Unmarshal(val, &r); err != nil {
		closer.Close()
		return err
	}
	closer.Close()

	r.IsRestricted = true
	newVal, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return DB.Set(key, newVal, pebble.Sync)
}

// DeleteWarpScanResults purges all WARP scan entries (both masque and wg prefixes).
func DeleteWarpScanResults() error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}

	prefixes := []string{"cf:node:masque:", "cf:node:wg:"}
	batch := DB.NewBatch()

	for _, pfx := range prefixes {
		prefix := []byte(pfx)
		upperBound := func(b []byte) []byte {
			end := make([]byte, len(b))
			copy(end, b)
			for i := len(end) - 1; i >= 0; i-- {
				end[i]++
				if end[i] != 0 {
					break
				}
			}
			return end
		}

		iter, err := DB.NewIter(&pebble.IterOptions{
			LowerBound: prefix,
			UpperBound: upperBound(prefix),
		})
		if err != nil {
			continue
		}
		for iter.First(); iter.Valid(); iter.Next() {
			batch.Delete(iter.Key(), pebble.Sync)
		}
		iter.Close()
	}

	err := batch.Commit(pebble.Sync)
	batch.Close()
	return err
}
