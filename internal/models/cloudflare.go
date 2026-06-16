package models

import (
	"time"

	"gorm.io/gorm"
)

// CloudflareAccount represents a managed Cloudflare account config/credentials
type CloudflareAccount struct {
	gorm.Model
	AccountName  string    `gorm:"size:191;not null" json:"account_name"`
	AccountID    string    `gorm:"size:191;not null" json:"account_id"`
	AccessToken  string    `gorm:"type:text;not null" json:"access_token"`
	RefreshToken string    `gorm:"type:text;not null" json:"refresh_token"`
	TokenExpiry  time.Time `json:"token_expiry"`
	Email        string    `gorm:"size:191" json:"email"`
	Status       string    `gorm:"size:50;default:'active'" json:"status"` // "active", "error"
}
