package models

import "gorm.io/gorm"

// CloudflareAccount represents a managed Cloudflare account config/credentials
type CloudflareAccount struct {
	gorm.Model
	AccountName string `gorm:"size:191;not null" json:"account_name"`
	AccountID   string `gorm:"size:191;not null" json:"account_id"`
	APIToken    string `gorm:"size:191;not null" json:"api_token"`
	Email       string `gorm:"size:191" json:"email"`
	Status      string `gorm:"size:50;default:'active'" json:"status"` // "active", "error"
}
