package pebble

import (
	"encoding/json"
	"sort"
	"strings"

	"clever-connect/internal/models"

	"github.com/cockroachdb/pebble"
)

func SaveDomain(domain *models.Domain) error {
	if domain.Category == "" {
		domain.Category = "ALL"
	}
	key := []byte("domain_" + domain.ID)
	val, err := json.Marshal(domain)
	if err != nil {
		return err
	}
	return DB.Set(key, val, pebble.Sync)
}

func SaveDomainsBulk(domains []models.Domain) error {
	if len(domains) == 0 {
		return nil
	}
	batch := DB.NewBatch()
	for _, d := range domains {
		if d.Category == "" {
			d.Category = "ALL"
		}
		key := []byte("domain_" + d.ID)
		val, _ := json.Marshal(d)
		batch.Set(key, val, pebble.Sync)
	}
	err := batch.Commit(pebble.Sync)
	batch.Close()
	return err
}

func GetDomain(id string) (*models.Domain, error) {
	key := []byte("domain_" + id)
	val, closer, err := DB.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var d models.Domain
	if err := json.Unmarshal(val, &d); err != nil {
		return nil, err
	}
	if d.Category == "" {
		d.Category = "ALL"
	}
	return &d, nil
}

func GetDomainByNameAndCategory(name, category string) (*models.Domain, error) {
	if category == "" {
		category = "ALL"
	}
	iter, err := DB.NewIter(nil)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	prefix := []byte("domain_")
	for iter.SeekGE(prefix); iter.Valid() && strings.HasPrefix(string(iter.Key()), "domain_"); iter.Next() {
		var d models.Domain
		if err := json.Unmarshal(iter.Value(), &d); err == nil {
			if d.Category == "" {
				d.Category = "ALL"
			}
			if d.DomainName == name && d.Category == category {
				return &d, nil
			}
		}
	}
	return nil, pebble.ErrNotFound
}

func GetDomainByName(name string) (*models.Domain, error) {
	return GetDomainByNameAndCategory(name, "ALL")
}

func ListCategories() []string {
	categoriesMap := make(map[string]bool)
	categoriesMap["ALL"] = true

	iter, err := DB.NewIter(nil)
	if err != nil {
		return []string{"ALL"}
	}
	defer iter.Close()

	prefix := []byte("domain_")
	for iter.SeekGE(prefix); iter.Valid() && strings.HasPrefix(string(iter.Key()), "domain_"); iter.Next() {
		var d models.Domain
		if err := json.Unmarshal(iter.Value(), &d); err == nil {
			if d.Category != "" {
				categoriesMap[d.Category] = true
			}
		}
	}

	var list []string
	for cat := range categoriesMap {
		list = append(list, cat)
	}
	sort.Strings(list)
	return list
}

func ListDomains(category, search, status, tlsFilter string, httpStatus int, limit, offset int, sortBy, sortOrder string) ([]models.Domain, int) {
	var all []models.Domain
	
	iter, err := DB.NewIter(nil)
	if err != nil {
		return all, 0
	}
	defer iter.Close()

	prefix := []byte("domain_")
	for iter.SeekGE(prefix); iter.Valid() && strings.HasPrefix(string(iter.Key()), "domain_"); iter.Next() {
		var d models.Domain
		if err := json.Unmarshal(iter.Value(), &d); err == nil {
			if d.Category == "" {
				d.Category = "ALL"
			}

			// Filter: Category
			if category != "" && category != "ALL" && d.Category != category {
				continue
			}

			// Filter: Search (name or IP)
			if search != "" {
				s := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(d.DomainName), s) && !strings.Contains(strings.ToLower(d.IPAddresses), s) {
					continue
				}
			}

			// Filter: Status
			if status != "" && d.Status != status {
				continue
			}

			// Filter: TLS Filter (valid, invalid, expired)
			if tlsFilter != "" {
				if tlsFilter == "valid" && !d.TLSStatus {
					continue
				}
				if tlsFilter == "invalid" && d.TLSStatus {
					continue
				}
				if tlsFilter == "expired" && (!d.TLSStatus || d.TLSExpiryDays > 0) {
					// expired tls is invalid or has 0/negative expiry days
					continue
				}
			}

			// Filter: HTTP Status
			if httpStatus > 0 && d.HTTPStatus != httpStatus {
				continue
			}

			all = append(all, d)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		var isLess bool
		switch sortBy {
		case "domain_name":
			isLess = all[i].DomainName < all[j].DomainName
		case "status":
			isLess = all[i].Status < all[j].Status
		case "latency_ms":
			isLess = all[i].LatencyMs < all[j].LatencyMs
		case "tls_expiry_days":
			isLess = all[i].TLSExpiryDays < all[j].TLSExpiryDays
		case "http_status":
			isLess = all[i].HTTPStatus < all[j].HTTPStatus
		default: // created_at
			isLess = all[i].CreatedAt.Before(all[j].CreatedAt)
		}
		if sortOrder == "desc" {
			return !isLess
		}
		return isLess
	})

	total := len(all)
	if limit > 0 {
		if offset >= total {
			return []models.Domain{}, total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		return all[offset:end], total
	}
	return all, total
}

func DeleteDomain(id string) error {
	key := []byte("domain_" + id)
	return DB.Delete(key, pebble.Sync)
}

func DeleteDomainsBulk(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	batch := DB.NewBatch()
	for _, id := range ids {
		key := []byte("domain_" + id)
		batch.Delete(key, pebble.Sync)
	}
	err := batch.Commit(pebble.Sync)
	batch.Close()
	return err
}

func DeleteAllDomains(category string) error {
	iter, err := DB.NewIter(nil)
	if err != nil {
		return err
	}
	defer iter.Close()

	batch := DB.NewBatch()
	prefix := []byte("domain_")
	for iter.SeekGE(prefix); iter.Valid() && strings.HasPrefix(string(iter.Key()), "domain_"); iter.Next() {
		var d models.Domain
		if err := json.Unmarshal(iter.Value(), &d); err == nil {
			if category == "" || category == "ALL" || d.Category == category {
				batch.Delete(iter.Key(), pebble.Sync)
			}
		}
	}
	err = batch.Commit(pebble.Sync)
	batch.Close()
	return err
}
