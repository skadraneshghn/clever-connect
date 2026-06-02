package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username string `gorm:"size:191;uniqueIndex;not null" json:"username"`
	Password string `gorm:"not null" json:"-"`
	Role     string `gorm:"default:'admin'" json:"role"`
}

type ClientSession struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	Username      string    `gorm:"not null" json:"username"`
	IP            string    `json:"ip"`
	Country       string    `json:"country"`
	Flag          string    `json:"flag"`
	Protocol      string    `json:"protocol"`
	ConnectedAt   time.Time `json:"connected_at"`
	UploadSpeed   float64   `json:"upload_speed"`   // MB/s
	DownloadSpeed float64   `json:"download_speed"` // MB/s
	Active        bool      `gorm:"default:true" json:"active"`
}

// EhcoServerConfig stores how the Clever Cloud server listens for incoming tunnel traffic
type EhcoServerConfig struct {
	gorm.Model
	ListenPort string `json:"listen_port" gorm:"default:'3001'"`
	AuthToken  string `json:"auth_token"`
	TargetMode string `json:"target_mode" gorm:"default:'direct'"` // 'direct' or 'xray'
	TargetHost string `json:"target_host" gorm:"default:'127.0.0.1:80'"`
	
	// --- NEW CTO CONFIGS ---
	EnableMux  bool   `json:"enable_mux" gorm:"default:true"`
	KeepAlive  int    `json:"keep_alive" gorm:"default:15"` // In seconds
	IsActive   bool   `json:"is_active" gorm:"default:false"`
}

// EhcoClientConfig stores how the local machine connects to Clever Cloud
type EhcoClientConfig struct {
	gorm.Model
	LocalPort  string `json:"local_port" gorm:"default:'1080'"`
	RemoteURL  string `json:"remote_url"` // e.g., wss://app.cleverapps.io/tunnel
	AuthToken  string `json:"auth_token"`
	
	// --- NEW CTO CONFIGS ---
	SNI        string `json:"sni"` // Essential for TLS obfuscation
	EnableMux  bool   `json:"enable_mux" gorm:"default:true"`
	KeepAlive  int    `json:"keep_alive" gorm:"default:15"`
	BypassIR   bool   `json:"bypass_ir" gorm:"default:true"`
	IsActive   bool   `json:"is_active" gorm:"default:false"`
}
