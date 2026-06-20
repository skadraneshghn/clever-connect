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

// ──────────────────────────────────────────────────────────────────────────────
// WARP Edge Endpoint Storage — Perfect Scoring Algorithm
//
// Key namespace per transport mode:
//   masque    → cf:node:masque:
//   masque_h2 → cf:node:h2:
//   wireguard → cf:node:wg:
//
// This ensures scan results from different modes are completely isolated.
// An endpoint that works for masque_h2 (TCP) may not work for wireguard (UDP)
// and vice versa.
//
// Scoring algorithm (higher = better, try first):
//
//   score = baseScore - latencyPenalty - failPenalty + quicBonus
//
//   baseScore    = 1000.0
//   latencyPenalty = latencyMs * 0.5   (linear; 200ms = -100 pts)
//   failPenalty  = failCount * 150.0   (each failure costs 150 pts)
//   quicBonus    = 50.0 if QUIC is available (IsRestricted=false)
//
// Score is recomputed on every read so it stays accurate as failures accumulate.
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
	IsRestricted          bool     `json:"is_restricted"` // true when QUIC/UDP is ISP-blocked
	FailCount             int      `json:"fail_count"`    // connection failures since last scan
	LastFailed            string   `json:"last_failed"`   // RFC3339 of last failure
	Score                 float64  `json:"score"`         // computed ranking score (higher=better)
}

// endpointScore computes a ranking score for an endpoint.
// Called on every read so the score always reflects the current state.
func endpointScore(r *WarpScanResult) float64 {
	const (
		baseScore    = 1000.0
		latencyScale = 0.5   // 200ms latency → -100 pts
		failPenalty  = 150.0 // each failure → -150 pts
		quicBonus    = 50.0  // QUIC available → +50 pts
	)
	s := baseScore - (r.LatencyMs * latencyScale) - (float64(r.FailCount) * failPenalty)
	if !r.IsRestricted {
		s += quicBonus
	}
	return s
}

// warpKeyPrefix returns the PebbleDB key prefix for a given transport mode.
// Each mode has its own namespace so results are completely isolated.
func warpKeyPrefix(mode string) string {
	switch mode {
	case "wireguard", "wg":
		return "cf:node:wg:"
	case "masque_h2", "h2":
		return "cf:node:h2:"
	default: // "masque"
		return "cf:node:masque:"
	}
}

// warpKey builds a full PebbleDB key for a WARP scan result.
func warpKey(mode, ipAddress string, port int) []byte {
	prefix := warpKeyPrefix(mode)
	return []byte(fmt.Sprintf("%s%s:%d", prefix, ipAddress, port))
}

// prefixUpperBound returns the exclusive upper bound for a prefix range scan.
func prefixUpperBound(prefix []byte) []byte {
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			break
		}
	}
	return end
}

// SaveWarpScanResult saves a scan result to PebbleDB under the appropriate prefix.
// The score is computed and stored so the UI can display it without recomputing.
func SaveWarpScanResult(mode string, result *WarpScanResult) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	// Preserve existing fail count if there's already a stored result
	existing, err := getWarpScanResult(mode, result.IPAddress, result.Port)
	if err == nil && existing != nil {
		result.FailCount = existing.FailCount
		result.LastFailed = existing.LastFailed
	}
	result.Score = endpointScore(result)

	key := warpKey(mode, result.IPAddress, result.Port)
	val, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return DB.Set(key, val, pebble.Sync)
}

// getWarpScanResult fetches a single result without error if not found.
func getWarpScanResult(mode, ipAddress string, port int) (*WarpScanResult, error) {
	if DB == nil {
		return nil, fmt.Errorf("pebble not initialized")
	}
	key := warpKey(mode, ipAddress, port)
	val, closer, err := DB.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var r WarpScanResult
	if err := json.Unmarshal(val, &r); err != nil {
		return nil, err
	}
	return &r, nil
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
		// Preserve existing fail count
		if existing, err := getWarpScanResult(mode, results[i].IPAddress, results[i].Port); err == nil && existing != nil {
			results[i].FailCount = existing.FailCount
			results[i].LastFailed = existing.LastFailed
		}
		results[i].Score = endpointScore(&results[i])

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

// ListWarpScanResults returns all scan results for a given transport mode.
// Results are sorted by Score descending (best first).
// Pass includeRestricted=true to also include QUIC-blocked endpoints.
func ListWarpScanResults(mode string) ([]WarpScanResult, error) {
	return listWarpScanResults(mode, false)
}

// ListAllWarpScanResults returns ALL results including restricted/failed endpoints.
func ListAllWarpScanResults(mode string) ([]WarpScanResult, error) {
	return listWarpScanResults(mode, true)
}

func listWarpScanResults(mode string, includeAll bool) ([]WarpScanResult, error) {
	if DB == nil {
		return nil, fmt.Errorf("pebble database is not initialized")
	}

	prefix := []byte(warpKeyPrefix(mode))
	iter, err := DB.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []WarpScanResult
	for iter.First(); iter.Valid(); iter.Next() {
		var r WarpScanResult
		if err := json.Unmarshal(iter.Value(), &r); err != nil {
			continue
		}
		// Recompute score in case the formula changed since last save
		r.Score = endpointScore(&r)
		results = append(results, r)
	}

	// Sort by Score descending — highest quality first
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// GetRankedEndpoints returns ALL endpoints for a mode, sorted best-first,
// for use by the engine's retry-loop. Unlike GetBestWarpEndpoint, this
// returns every candidate so the engine can try them in order.
//
// Algorithm priority:
//   1. Non-restricted, high score (QUIC works, low latency, zero fails)
//   2. Restricted (TCP-only), high score  (QUIC blocked but TCP fine)
//   3. Failed endpoints (FailCount > 0) as last resort
func GetRankedEndpoints(mode string) ([]WarpScanResult, error) {
	all, err := listWarpScanResults(mode, true)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no WARP endpoints found for mode %q — run a scan first", mode)
	}

	// Separate into tiers
	var tier1 []WarpScanResult // QUIC capable, zero fails
	var tier2 []WarpScanResult // TCP-only (restricted), zero fails
	var tier3 []WarpScanResult // any endpoint with fails (last resort)

	for _, r := range all {
		switch {
		case r.FailCount > 0:
			tier3 = append(tier3, r)
		case !r.IsRestricted:
			tier1 = append(tier1, r)
		default:
			tier2 = append(tier2, r)
		}
	}

	// For masque_h2 all endpoints are inherently TCP, so treat tier2 as normal
	if mode == "masque_h2" || mode == "h2" {
		tier1 = append(tier1, tier2...)
		tier2 = nil
	}

	result := make([]WarpScanResult, 0, len(all))
	result = append(result, tier1...)
	result = append(result, tier2...)
	result = append(result, tier3...)
	return result, nil
}

// GetBestWarpEndpoint returns the single highest-ranked endpoint for the mode.
// Use GetRankedEndpoints for the engine's full retry loop.
func GetBestWarpEndpoint(mode string) (*WarpScanResult, error) {
	ranked, err := GetRankedEndpoints(mode)
	if err != nil {
		return nil, err
	}
	r := ranked[0]
	return &r, nil
}

// IncrementEndpointFailCount increments the failure counter for an endpoint and
// updates its score. This permanently penalises bad endpoints across restarts.
func IncrementEndpointFailCount(mode, ipAddress string, port int) error {
	if DB == nil {
		return fmt.Errorf("pebble not initialized")
	}
	r, err := getWarpScanResult(mode, ipAddress, port)
	if err != nil {
		return err // endpoint not in DB; can't penalise
	}
	r.FailCount++
	r.LastFailed = time.Now().UTC().Format(time.RFC3339)
	r.Score = endpointScore(r)

	key := warpKey(mode, ipAddress, port)
	val, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return DB.Set(key, val, pebble.Sync)
}

// MarkWarpEndpointRestricted flags a specific endpoint as restricted in PebbleDB.
// Also recomputes the score so the change is immediately reflected in rankings.
func MarkWarpEndpointRestricted(mode, ipAddress string, port int) error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}
	r, err := getWarpScanResult(mode, ipAddress, port)
	if err != nil {
		return err
	}
	r.IsRestricted = true
	r.Score = endpointScore(r)

	key := warpKey(mode, ipAddress, port)
	newVal, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return DB.Set(key, newVal, pebble.Sync)
}

// DeleteWarpScanResults purges all WARP scan entries for all modes.
func DeleteWarpScanResults() error {
	if DB == nil {
		return fmt.Errorf("pebble database is not initialized")
	}

	// Include all 3 mode prefixes
	prefixes := []string{"cf:node:masque:", "cf:node:h2:", "cf:node:wg:"}
	batch := DB.NewBatch()

	for _, pfx := range prefixes {
		prefix := []byte(pfx)
		iter, err := DB.NewIter(&pebble.IterOptions{
			LowerBound: prefix,
			UpperBound: prefixUpperBound(prefix),
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

