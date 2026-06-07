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
	key := []byte(fmt.Sprintf("%d", id))
	return DB.Delete(key, pebble.Sync)
}

// DeleteAllClientConfigs removes all configs.
func DeleteAllClientConfigs() error {
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

// ListClientConfigs returns configs with optional search, pagination, and subscription filtering.
func ListClientConfigs(search string, subscriptionID *uint, offset, limit int) ([]models.V2RayClientConfig, int) {
	var all []models.V2RayClientConfig
	
	iter, err := DB.NewIter(nil)
	if err != nil {
		return all, 0
	}
	defer iter.Close()

	searchLower := ""
	if search != "" {
		searchLower = strings.ToLower(search)
	}

	for iter.First(); iter.Valid(); iter.Next() {
		var cfg models.V2RayClientConfig
		if err := json.Unmarshal(iter.Value(), &cfg); err == nil {
			// Filtering
			if subscriptionID != nil && cfg.SubscriptionID != *subscriptionID {
				continue
			}
			if searchLower != "" {
				if !strings.Contains(strings.ToLower(cfg.Name), searchLower) && !strings.Contains(strings.ToLower(cfg.Address), searchLower) {
					continue
				}
			}
			all = append(all, cfg)
		}
	}

	// Sort by priority asc, then name asc
	sort.Slice(all, func(i, j int) bool {
		if all[i].Priority == all[j].Priority {
			return all[i].Name < all[j].Name
		}
		return all[i].Priority < all[j].Priority
	})

	total := len(all)

	// Apply pagination
	if offset >= total {
		return []models.V2RayClientConfig{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	if limit > 0 {
		return all[offset:end], total
	}

	return all, total
}
