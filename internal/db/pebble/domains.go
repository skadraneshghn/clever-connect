package pebble

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/models"

	"gorm.io/gorm"
)

func SaveDomain(domain *models.Domain) error {
	if db.DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	if domain.Category == "" {
		domain.Category = "ALL"
	}
	domain.UpdatedAt = time.Now()
	return db.DB.Save(domain).Error
}

func SaveDomainsBulk(domains []models.Domain) error {
	if db.DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	if len(domains) == 0 {
		return nil
	}
	return db.DB.Transaction(func(tx *gorm.DB) error {
		for i := range domains {
			if domains[i].Category == "" {
				domains[i].Category = "ALL"
			}
			domains[i].UpdatedAt = time.Now()
			if err := tx.Save(&domains[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func GetDomain(id string) (*models.Domain, error) {
	if db.DB == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var d models.Domain
	err := db.DB.First(&d, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	if d.Category == "" {
		d.Category = "ALL"
	}
	return &d, nil
}

func GetDomainByNameAndCategory(name, category string) (*models.Domain, error) {
	if db.DB == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if category == "" {
		category = "ALL"
	}
	var d models.Domain
	err := db.DB.First(&d, "domain_name = ? AND category = ?", name, category).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func GetDomainByName(name string) (*models.Domain, error) {
	return GetDomainByNameAndCategory(name, "ALL")
}

func ListCategories() []string {
	if db.DB == nil {
		return []string{"ALL"}
	}
	var categories []string
	db.DB.Model(&models.Domain{}).Distinct().Pluck("category", &categories)
	
	categoriesMap := make(map[string]bool)
	categoriesMap["ALL"] = true
	for _, cat := range categories {
		if cat != "" {
			categoriesMap[cat] = true
		}
	}
	var list []string
	for cat := range categoriesMap {
		list = append(list, cat)
	}
	sort.Strings(list)
	return list
}

type DomainStats struct {
	Total    int `json:"total"`
	Online   int `json:"online"`
	Offline  int `json:"offline"`
	Checking int `json:"checking"`
	SSLValid int `json:"ssl_valid"`
}

func ListDomains(category, search, status, tlsFilter string, httpStatus int, limit, offset int, sortBy, sortOrder string) ([]models.Domain, int, DomainStats) {
	var stats DomainStats
	if db.DB == nil {
		return []models.Domain{}, 0, stats
	}

	// 1. Calculate stats for category (ignoring search filters and pagination)
	var allForStats []models.Domain
	statQuery := db.DB.Model(&models.Domain{})
	if category != "" && category != "ALL" {
		statQuery = statQuery.Where("category = ?", category)
	}
	statQuery.Find(&allForStats)

	for _, d := range allForStats {
		stats.Total++
		if d.Status == "online" {
			stats.Online++
		} else if d.Status == "offline" || d.Status == "timeout" || d.Status == "nxdomain" {
			stats.Offline++
		} else if d.Status == "checking" {
			stats.Checking++
		}
		if d.TLSStatus {
			stats.SSLValid++
		}
	}

	// 2. Build filtered query
	query := db.DB.Model(&models.Domain{})
	if category != "" && category != "ALL" {
		query = query.Where("category = ?", category)
	}
	if search != "" {
		s := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(domain_name) LIKE ? OR LOWER(ip_addresses) LIKE ?", s, s)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if tlsFilter != "" {
		if tlsFilter == "valid" {
			query = query.Where("tls_status = ?", true)
		} else if tlsFilter == "invalid" {
			query = query.Where("tls_status = ?", false)
		} else if tlsFilter == "expired" {
			query = query.Where("tls_status = ? AND tls_expiry_days <= 0", true)
		}
	}
	if httpStatus > 0 {
		query = query.Where("http_status = ?", httpStatus)
	}

	var total64 int64
	query.Count(&total64)
	total := int(total64)

	// Order by
	orderCol := "created_at"
	switch sortBy {
	case "domain_name":
		orderCol = "domain_name"
	case "status":
		orderCol = "status"
	case "latency_ms":
		orderCol = "latency_ms"
	case "tls_expiry_days":
		orderCol = "tls_expiry_days"
	case "http_status":
		orderCol = "http_status"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	query = query.Order(orderCol + " " + sortOrder)

	// Pagination
	var result []models.Domain
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	err := query.Find(&result).Error
	if err != nil {
		return []models.Domain{}, 0, stats
	}

	for i := range result {
		if result[i].Category == "" {
			result[i].Category = "ALL"
		}
	}

	return result, total, stats
}

func DeleteDomain(id string) error {
	if db.DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	return db.DB.Delete(&models.Domain{}, "id = ?", id).Error
}

func DeleteDomainsBulk(ids []string) error {
	if db.DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	if len(ids) == 0 {
		return nil
	}
	return db.DB.Delete(&models.Domain{}, "id IN ?", ids).Error
}

func DeleteAllDomains(category string) error {
	if db.DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	if category == "" || category == "ALL" {
		return db.DB.Where("1 = 1").Delete(&models.Domain{}).Error
	}
	return db.DB.Delete(&models.Domain{}, "category = ?", category).Error
}
