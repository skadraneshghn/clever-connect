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

	// --- DYNAMIC EDGE BRIDGE ---
	EnableBridge bool   `json:"enable_bridge" gorm:"default:false"`
	BridgeURL    string `json:"bridge_url"`
	BridgeSNI    string `json:"bridge_sni"`
}

// LeechConfig stores the advanced settings for the download manager
type LeechConfig struct {
	gorm.Model
	DefaultSavePath string `json:"default_save_path" gorm:"default:'/downloads'"`
	MaxConcurrent   int    `json:"max_concurrent" gorm:"default:3"`
	ThreadsPerJob   int    `json:"threads_per_job" gorm:"default:8"`
	UserAgent       string `json:"user_agent" gorm:"default:'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0'"`
	ProxyURL        string `json:"proxy_url"` // Optional HTTP/SOCKS5 proxy
}

// LeechJob tracks individual remote download tasks
type LeechJob struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	URL           string    `gorm:"type:text;not null" json:"url"`
	Filename      string    `json:"filename"`
	SaveDirectory string    `json:"save_directory"`
	TotalBytes    int64     `json:"total_bytes"`
	Downloaded    int64     `json:"downloaded"`
	Status        string    `json:"status" gorm:"default:'pending'"` // pending, downloading, paused, completed, error
	Progress      float64   `json:"progress"` // 0.0 to 100.0
	Speed         float64   `json:"speed"`    // MB/s
	Threads       int       `json:"threads"`
	Username      string    `json:"username"`
	Password      string    `json:"password"`
	ErrorMessage  string    `json:"error_message"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TorrentJob tracks BitTorrent tasks
type TorrentJob struct {
	InfoHash      string    `gorm:"primaryKey" json:"info_hash"`
	Name          string    `json:"name"`
	MagnetURI     string    `gorm:"type:text" json:"magnet_uri"`
	TorrentPath   string    `json:"torrent_path"` // Local path to saved .torrent
	SaveDirectory string    `json:"save_directory"`
	Status        string    `json:"status" gorm:"default:'downloading'"` // downloading, paused, completed, seeding, error
	TotalBytes    int64     `json:"total_bytes"`
	Downloaded    int64     `json:"downloaded"`
	Uploaded      int64     `json:"uploaded"`
	Progress      float64   `json:"progress"`
	DownloadSpeed float64   `json:"download_speed"` // MB/s
	UploadSpeed   float64   `json:"upload_speed"`   // MB/s
	Peers         int       `json:"peers"`
	ErrorMessage  string    `json:"error_message"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TelegramConfig stores the Telegram bot configuration, persisted in the database.
// All settings are configurable from the admin panel and the REST API.
type TelegramConfig struct {
	gorm.Model
	BotToken            string `json:"bot_token" gorm:"type:text"`
	AdminUserIDs        string `json:"admin_user_ids" gorm:"type:text"`                // Comma-separated Telegram user IDs
	WelcomeMessage      string `json:"welcome_message" gorm:"type:text"`
	PollingInterval     int    `json:"polling_interval" gorm:"default:10"`              // Seconds between long-poll cycles
	MaxFileSize         int    `json:"max_file_size" gorm:"default:50"`                 // Maximum file size in MB
	EnableFileSharing   bool   `json:"enable_file_sharing" gorm:"default:true"`
	EnableNotifications bool   `json:"enable_notifications" gorm:"default:true"`
	IsActive            bool   `json:"is_active" gorm:"default:false"`                  // Whether the bot should auto-start
}

// TorrentConfig stores advanced client configurations for BitTorrent client
type TorrentConfig struct {
	gorm.Model
	SaveDirectory              string  `json:"save_directory" gorm:"default:'./data/manager/downloads'"`
	MaxConnectionsPerTorrent   int     `json:"max_connections_per_torrent" gorm:"default:200"`
	MaxHalfOpenConnections     int     `json:"max_half_open_connections" gorm:"default:100"`
	UploadLimitMB              float64 `json:"upload_limit_mb" gorm:"default:0"` // 0 is unlimited
	DownloadLimitMB            float64 `json:"download_limit_mb" gorm:"default:0"` // 0 is unlimited
	EnableDHT                  bool    `json:"enable_dht" gorm:"default:true"`
	EnablePEX                  bool    `json:"enable_pex" gorm:"default:true"`
	EnableUTP                  bool    `json:"enable_utp" gorm:"default:true"`
	EnableTCP                  bool    `json:"enable_tcp" gorm:"default:true"`
	EnableUpload               bool    `json:"enable_upload" gorm:"default:true"`
	PieceHashersPerTorrent     int     `json:"piece_hashers_per_torrent" gorm:"default:4"`
	CustomTrackers             string  `json:"custom_trackers" gorm:"type:text"`
}


