package models

import (
	"time"

	"gorm.io/gorm"
)

// CloudflareAccount represents a managed Cloudflare account config/credentials
type CloudflareAccount struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	AccountName  string         `gorm:"size:191;not null" json:"account_name"`
	AccountID    string         `gorm:"size:191;not null" json:"account_id"`
	AccessToken  string         `gorm:"type:text;not null" json:"access_token"`
	RefreshToken string         `gorm:"type:text" json:"refresh_token"`
	TokenExpiry  time.Time      `json:"token_expiry"`
	Email        string         `gorm:"size:191" json:"email"`
	AuthType     string         `gorm:"size:50;default:'oauth'" json:"auth_type"` // "oauth", "token", "key"
	Status       string         `gorm:"size:50;default:'active'" json:"status"` // "active", "error"
}

type CloudflareWorkerDeployment struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	AccountID    string         `gorm:"size:191;not null;index" json:"account_id"`
	ScriptName   string         `gorm:"size:191;not null" json:"script_name"`
	LocalPath    string         `gorm:"size:255;default:'data/worker.js'" json:"local_path"`
	DefaultURL   string         `gorm:"size:255" json:"default_url"`   // https://script-name.subdomain.workers.dev
	CustomDomain string         `gorm:"size:191" json:"custom_domain"` // e.g., nova.yourdomain.com
	ZoneID       string         `gorm:"size:191" json:"zone_id"`
	HealthStatus string         `gorm:"size:50;default:'unknown'" json:"health_status"` // "healthy" | "unhealthy" | "error"
	Message      string         `gorm:"type:text" json:"message"`
}

type NovaForwarderConfig struct {
	gorm.Model
	SecretAuthKey    string `gorm:"size:191;not null" json:"secret_auth_key"`
	AssignedCorePort int    `gorm:"default:8081" json:"assigned_core_port"` // Internal loopback to Xray/Singbox in container
	IsEnabled        bool   `gorm:"default:true" json:"is_enabled"`
}


