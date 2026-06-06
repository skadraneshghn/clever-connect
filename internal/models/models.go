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

// ──────────────────────────────────────────────────────────────────────────────
// Soroush WebRTC "The Hive" Tunnel Models (ADDITIVE — parallel to Ehco)
// ──────────────────────────────────────────────────────────────────────────────

// SoroushAccount stores authenticated Soroush messenger accounts used as
// tunnel workers. Each account holds its MTProto auth key material for
// autonomous JWT token generation via the Soroush LiveKit SFU.
type SoroushAccount struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	PhoneNumber   string `gorm:"size:20;uniqueIndex;not null" json:"phone_number"`
	Name          string `gorm:"size:100" json:"name"`
	SoroushUserID int64  `json:"soroush_user_id"`
	AccessHash    int64  `json:"access_hash"`
	DisplayName   string `gorm:"size:100" json:"display_name"`
	AuthKey       []byte `json:"-"`                              // 256-byte MTProto auth key
	AuthKeyID     []byte `json:"-"`                              // 8-byte auth key ID
	ServerSalt    []byte `json:"-"`                              // 8-byte server salt
	DcID          int    `json:"dc_id" gorm:"default:2"`
	Role          string `json:"role" gorm:"size:20;default:'worker'"`   // 'host' or 'worker'
	IsServerNode  bool   `json:"is_server_node" gorm:"default:false"`
	Status        string `json:"status" gorm:"size:30;default:'idle'"`   // idle, connected, busy, tunnel_active, error
	LastActive    string `json:"last_active"`
	LiveKitToken  string `json:"livekit_token" gorm:"type:text"`         // Per-account LiveKit JWT token (unique identity per worker)
}

// SoroushTunnelConfig stores the Hive tunnel engine configuration.
// This is a singleton row — only one config exists at a time.
// The PSK field is used for QUIC TLS identity verification and
// the HKDF-based handshake sync protocol.
type SoroushTunnelConfig struct {
	gorm.Model
	GroupChatID     int64  `json:"group_chat_id"`
	GroupAccessHash int64  `json:"group_access_hash"`
	CallID          int64  `json:"call_id"`             // Static bypass parameter
	CallAccessHash  string `json:"call_access_hash"`    // Static bypass parameter
	ServerIdentity  string `json:"server_identity"`     // The exact Soroush UserID of the Queen (e.g., "64698297")
	PSK             string `json:"psk"`                 // Pre-Shared Key for worker auth
	LiveKitURL            string `json:"livekit_url"`         // LiveKit SFU WebSocket endpoint (e.g., wss://k.splus.ir)
	FallbackLiveKitToken  string `json:"fallback_livekit_token" gorm:"type:text"` // Manual fallback LiveKit token
	SocksPort             int    `json:"socks_port" gorm:"default:4046"`
	IsActive              bool   `json:"is_active" gorm:"default:false"`
	EngineMode            string `json:"engine_mode" gorm:"size:30;default:'swarm'"` // 'swarm' (LiveKit SFU Swarm)
	MaxWorkers            int    `json:"max_workers" gorm:"default:5"`
	LoadBalanceAlgo       string `json:"load_balance_algo" gorm:"size:30;default:'least-latency'"` // 'round-robin', 'least-latency'
}

// LeechConfig stores the advanced settings for the download manager
type LeechConfig struct {
	gorm.Model
	DefaultSavePath     string `json:"default_save_path" gorm:"default:'/downloads'"`
	MaxConcurrent       int    `json:"max_concurrent" gorm:"default:3"`
	ThreadsPerJob       int    `json:"threads_per_job" gorm:"default:8"`
	UserAgent           string `json:"user_agent" gorm:"default:'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0'"`
	ProxyURL            string `json:"proxy_url"`           // Optional HTTP/SOCKS5 proxy
	PremiumUserID       string `json:"premium_user_id"`
	PremiumAPIKey       string `json:"premium_api_key"`
	AutoUploadToTelegram bool  `json:"auto_upload_to_telegram" gorm:"default:false"` // Auto-upload completed downloads to Telegram
	AutoUploadChatID    int64  `json:"auto_upload_chat_id"`                          // Target chat ID for auto-uploads (0 = first admin)
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
	UsePremium    bool      `json:"use_premium" gorm:"default:false"`
	ErrorMessage  string    `json:"error_message"`
	FileExists    bool      `gorm:"-" json:"file_exists"`
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
	SelectedFiles string    `gorm:"type:text" json:"selected_files"` // JSON array of selected file indices
	ErrorMessage  string    `json:"error_message"`
	FileExists    bool      `gorm:"-" json:"file_exists"`
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
	MaxFileSize         int    `json:"max_file_size" gorm:"default:2000"`                 // Maximum file size in MB
	EnableFileSharing   bool   `json:"enable_file_sharing" gorm:"default:true"`
	EnableNotifications bool   `json:"enable_notifications" gorm:"default:true"`
	IsActive            bool   `json:"is_active" gorm:"default:false"`                  // Whether the bot should auto-start
	AppID               int    `json:"app_id"`
	AppHash             string `json:"app_hash"`
	MTProtoServer       string `json:"mtproto_server"`
	MTProtoPublicKey    string `json:"mtproto_public_key" gorm:"type:text"`
	PhoneNumber         string `json:"phone_number"`
	AuthType            string `json:"auth_type" gorm:"default:'bot'"` // 'bot' or 'user'
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

// ──────────────────────────────────────────────────────────────────────────────
// Enterprise Job Scheduler Models
// ──────────────────────────────────────────────────────────────────────────────

// Job status constants
const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
	JobStatusScheduled = "scheduled" // For cron-scheduled jobs
)

// SchedulerJob is the central model tracking each unit of work in the scheduler.
type SchedulerJob struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UUID        string     `gorm:"size:36;uniqueIndex" json:"uuid"`
	Type        string     `gorm:"size:100;not null;index" json:"type"`                      // e.g., file_compress, leech_download, custom_task
	Name        string     `gorm:"size:255;not null" json:"name"`                             // Human-readable name
	Description string     `gorm:"type:text" json:"description"`                              // Extended description
	Category    string     `gorm:"size:100;index;default:'general'" json:"category"`           // Grouping: general, files, download, system, cron
	Status      string     `gorm:"size:50;not null;index;default:'queued'" json:"status"`      // queued, running, completed, failed, cancelled, scheduled
	Priority    int        `gorm:"default:5;index" json:"priority"`                            // 1=highest, 10=lowest
	Progress    int        `gorm:"default:0" json:"progress"`                                  // 0-100
	Message     string     `gorm:"type:text" json:"message"`                                   // Status message or error details
	Payload     string     `gorm:"type:text" json:"payload"`                                   // JSON payload for the job handler
	CronExpr    string     `gorm:"size:100" json:"cron_expr"`                                  // Optional cron expression (robfig/cron format)
	RetryCount  int        `gorm:"default:0" json:"retry_count"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// SchedulerJobLog stores granular execution logs for each job run.
type SchedulerJobLog struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	SchedulerJobID uint      `gorm:"index;not null" json:"scheduler_job_id"`
	Level          string    `gorm:"size:20;not null" json:"level"` // INFO, WARN, ERROR, DEBUG
	Message        string    `gorm:"type:text;not null" json:"message"`
	CreatedAt      time.Time `json:"created_at"`
}

// SchedulerConfig stores admin-configurable scheduler parameters.
type SchedulerConfig struct {
	gorm.Model
	MaxConcurrentJobs   int  `json:"max_concurrent_jobs" gorm:"default:4"`
	DefaultPriority     int  `json:"default_priority" gorm:"default:5"`
	RetryLimit          int  `json:"retry_limit" gorm:"default:3"`
	RetryDelaySeconds   int  `json:"retry_delay_seconds" gorm:"default:30"`
	JobTimeoutSeconds   int  `json:"job_timeout_seconds" gorm:"default:3600"`
	PurgeAfterDays      int  `json:"purge_after_days" gorm:"default:30"`
	EnableCronJobs      bool `json:"enable_cron_jobs" gorm:"default:true"`
	EnableNotifications bool `json:"enable_notifications" gorm:"default:false"`
}

// TelegramSubscriber stores Telegram users who have interacted with the bot.
type TelegramSubscriber struct {
	gorm.Model
	ChatID    int64  `gorm:"uniqueIndex;not null" json:"chat_id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Active    bool   `gorm:"default:true" json:"active"`
}

// ──────────────────────────────────────────────────────────────────────────────
// YouTube Downloader Models
// ──────────────────────────────────────────────────────────────────────────────

// YouTubeJob tracks individual YouTube video download tasks
type YouTubeJob struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	VideoURL         string    `gorm:"type:text;not null" json:"video_url"`
	VideoID          string    `json:"video_id"`
	Title            string    `gorm:"type:text" json:"title"`
	Author           string    `json:"author"`
	Duration         string    `json:"duration"` // Human-readable duration
	DurationSeconds  int64     `json:"duration_seconds"`
	Thumbnail        string    `gorm:"type:text" json:"thumbnail"`
	Filename         string    `json:"filename"`
	SaveDirectory    string    `json:"save_directory"`
	SelectedITag     int       `json:"selected_itag"`
	QualityLabel     string    `json:"quality_label"` // e.g., "1080p", "720p", "360p"
	MimeType         string    `json:"mime_type"`
	TotalBytes       int64     `json:"total_bytes"`
	Downloaded       int64     `json:"downloaded"`
	Status           string    `json:"status" gorm:"default:'pending'"` // pending, fetching, downloading, converting, completed, error
	Progress         float64   `json:"progress"`                        // 0.0 to 100.0
	ConvertProgress  float64   `json:"convert_progress"`                // 0.0 to 100.0 (TV conversion progress)
	Speed            float64   `json:"speed"`                           // MB/s
	ConvertToTV      bool      `json:"convert_to_tv" gorm:"default:false"`
	ConvertStatus    string    `json:"convert_status"` // "", "queued", "converting", "completed", "error"
	ErrorMessage     string    `json:"error_message"`
	FileExists       bool      `gorm:"-" json:"file_exists"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// YouTubeConfig stores default configurations for YouTube downloads
type YouTubeConfig struct {
	gorm.Model
	DefaultSavePath string `json:"default_save_path" gorm:"default:'./downloads/youtube'"`
	MaxConcurrent   int    `json:"max_concurrent" gorm:"default:2"`
	ProxyURL        string `json:"proxy_url"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Spotify Downloader Models
// ──────────────────────────────────────────────────────────────────────────────

// SpotifyConfig stores admin-configurable Spotify downloader settings
type SpotifyConfig struct {
	gorm.Model
	ClientID         string `json:"client_id" gorm:"type:text"`
	ClientSecret     string `json:"client_secret" gorm:"type:text"`
	DefaultSavePath  string `json:"default_save_path" gorm:"default:'./downloads/spotify/audios'"`
	DefaultFormat    string `json:"default_format" gorm:"default:'mp3'"`         // mp3, flac, opus, m4a, wav, ogg
	DefaultBitrate   string `json:"default_bitrate" gorm:"default:'320k'"`       // 128k, 192k, 256k, 320k, auto
	MaxConcurrent    int    `json:"max_concurrent" gorm:"default:3"`
	EmbedMetadata    bool   `json:"embed_metadata" gorm:"default:true"`          // Embed ID3 tags & cover art
	EmbedLyrics      bool   `json:"embed_lyrics" gorm:"default:true"`            // Embed lyrics if available
	OverwriteExist   bool   `json:"overwrite_existing" gorm:"default:false"`     // Overwrite files if they exist
	ProxyURL         string `json:"proxy_url"`                                    // Optional proxy for Spotify API
	FileNameTemplate string `json:"file_name_template" gorm:"default:'{artist} - {title}'"` // Filename template
}

// SpotifyJob tracks individual Spotify track download tasks through the full pipeline
type SpotifyJob struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	SpotifyURL     string    `gorm:"type:text;not null" json:"spotify_url"`       // Original Spotify URL
	SpotifyID      string    `json:"spotify_id"`                                   // Spotify Track ID
	Title          string    `gorm:"type:text" json:"title"`
	Artist         string    `json:"artist"`
	Artists        string    `gorm:"type:text" json:"artists"`                     // JSON array of artist names
	Album          string    `json:"album"`
	AlbumArtist    string    `json:"album_artist"`
	CoverURL       string    `gorm:"type:text" json:"cover_url"`                  // High-res album art URL
	ReleaseDate    string    `json:"release_date"`
	TrackNumber    int       `json:"track_number"`
	TotalTracks    int       `json:"total_tracks"`
	DiscNumber     int       `json:"disc_number"`
	DurationMs     int       `json:"duration_ms"`
	ISRC           string    `json:"isrc"`                                         // International Standard Recording Code
	Genre          string    `json:"genre"`
	Explicit       bool      `json:"explicit"`
	Popularity     int       `json:"popularity"`
	YouTubeURL     string    `gorm:"type:text" json:"youtube_url"`                // Matched YouTube video URL
	Filename       string    `json:"filename"`
	SaveDirectory  string    `json:"save_directory"`
	Format         string    `json:"format" gorm:"default:'mp3'"`                 // Output format
	Bitrate        string    `json:"bitrate" gorm:"default:'320k'"`               // Output bitrate
	TotalBytes     int64     `json:"total_bytes"`
	Downloaded     int64     `json:"downloaded"`
	Status         string    `json:"status" gorm:"default:'pending'"`             // pending, fetching_meta, matching, downloading, converting, tagging, completed, error
	Progress       float64   `json:"progress"`                                    // 0.0 to 100.0
	Speed          float64   `json:"speed"`                                       // MB/s
	AlbumJobID     string    `json:"album_job_id"`                                // Group tracks from same album
	ErrorMessage   string    `json:"error_message"`
	FileExists     bool      `gorm:"-" json:"file_exists"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FileRegistry tracks unique files saved on disk via their BLAKE3 checksum
type FileRegistry struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Checksum    string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"checksum"`
	FilePath    string    `gorm:"type:text;not null" json:"file_path"`
	FileSize    int64     `json:"file_size"`
	MimeType    string    `json:"mime_type"`
	URL         string    `gorm:"type:text" json:"url"`
	ETag        string    `gorm:"type:varchar(256);index" json:"etag"`
	TgFileID    int64     `gorm:"index" json:"tg_file_id"`
	TorrentHash string    `gorm:"type:varchar(40);index" json:"torrent_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

