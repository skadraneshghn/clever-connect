package config

import (
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppMode             string // "client" or "server"
	Port                string
	JWTSecret           []byte
	WSHeartbeatInterval time.Duration
	ServerURL           string
	ServerAuthToken     string

	// SQLite (Client mode)
	SQLitePath string

	// MySQL (Server mode)
	MySQLUser     string
	MySQLPassword string
	MySQLHost     string
	MySQLPort     string
	MySQLDBName   string

	// Seed Admin
	AdminUsername string
	AdminPassword string
}

func LoadConfig() *Config {
	// Try loading from .env if present
	_ = godotenv.Load()

	appMode := os.Getenv("APP_MODE")
	if appMode == "" {
		appMode = "client" // default
	}

	port := os.Getenv("PORT")
	if port == "" {
		if appMode == "server" {
			port = "8081"
		} else {
			port = "8080"
		}
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "super-secret-jwt-key"
	}

	wsIntervalStr := os.Getenv("WS_HEARTBEAT_INTERVAL")
	wsInterval := 5 * time.Second
	if wsIntervalStr != "" {
		if parsed, err := time.ParseDuration(wsIntervalStr); err == nil {
			wsInterval = parsed
		}
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = os.Getenv("CLIVER_SERVER_URL")
	}

	serverAuthToken := os.Getenv("SERVER_AUTH_TOKEN")
	if serverAuthToken == "" {
		serverAuthToken = os.Getenv("CLIVER_SERVER_AUTH_TOKEN")
	}

	cfg := &Config{
		AppMode:             appMode,
		Port:                port,
		JWTSecret:           []byte(jwtSecret),
		WSHeartbeatInterval: wsInterval,
		ServerURL:           serverURL,
		ServerAuthToken:     serverAuthToken,
		SQLitePath:          getEnv("SQLITE_DB_PATH", "data/client.db"),
		MySQLUser:           getEnv("MYSQL_USER", "root"),
		MySQLPassword:       os.Getenv("MYSQL_PASSWORD"),
		MySQLHost:           getEnv("MYSQL_HOST", "127.0.0.1"),
		MySQLPort:           getEnv("MYSQL_PORT", "3306"),
		MySQLDBName:         getEnv("MYSQL_DB_NAME", "clever_connect_server"),
		AdminUsername:       getEnv("ADMIN_USERNAME", "salman"),
		AdminPassword:       getEnv("ADMIN_PASSWORD", "136517"),
	}

	// Automatic parsing of database URIs (e.g. from Clever Cloud MySQL addon)
	mysqlURI := os.Getenv("MYSQL_ADDON_URI")
	if mysqlURI == "" {
		mysqlURI = os.Getenv("DATABASE_URL")
	}
	if mysqlURI != "" && strings.HasPrefix(mysqlURI, "mysql://") {
		uri := strings.TrimPrefix(mysqlURI, "mysql://")
		parts := strings.SplitN(uri, "@", 2)
		if len(parts) == 2 {
			userPass := parts[0]
			hostPortDb := parts[1]

			up := strings.SplitN(userPass, ":", 2)
			if len(up) == 2 {
				cfg.MySQLUser = up[0]
				cfg.MySQLPassword = up[1]
			}

			hpdb := strings.SplitN(hostPortDb, "/", 2)
			if len(hpdb) == 2 {
				hostPort := hpdb[0]
				cfg.MySQLDBName = hpdb[1]

				if idx := strings.Index(cfg.MySQLDBName, "?"); idx > 0 {
					cfg.MySQLDBName = cfg.MySQLDBName[:idx]
				}

				hp := strings.SplitN(hostPort, ":", 2)
				if len(hp) == 2 {
					cfg.MySQLHost = hp[0]
					cfg.MySQLPort = hp[1]
				} else {
					cfg.MySQLHost = hostPort
					cfg.MySQLPort = "3306"
				}
			}
		}
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}
