package db

import (
	"fmt"
	"os"
	"path/filepath"

	"clever-connect/internal/config"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(cfg *config.Config) *gorm.DB {
	var err error

	// Use GORM silent mode — we handle logging through our own system
	gormCfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	}

	if cfg.AppMode == "client" {
		// SQLite Mode
		logger.Info("DB", "Connecting to SQLite database", "path", cfg.SQLitePath)

		// Ensure parent directory exists
		dir := filepath.Dir(cfg.SQLitePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Fatal("DB", "Failed to create database directories", "error", err)
		}

		DB, err = gorm.Open(sqlite.Open(cfg.SQLitePath), gormCfg)
		if err != nil {
			logger.Fatal("DB", "Failed to connect to SQLite", "path", cfg.SQLitePath, "error", err)
		}
		logger.Info("DB", "SQLite connection established", "path", cfg.SQLitePath)
	} else {
		// MySQL Mode (Server panel)
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.MySQLUser,
			cfg.MySQLPassword,
			cfg.MySQLHost,
			cfg.MySQLPort,
			cfg.MySQLDBName,
		)
		logger.Info("DB", "Connecting to MySQL database",
			"user", cfg.MySQLUser,
			"host", cfg.MySQLHost,
			"port", cfg.MySQLPort,
			"database", cfg.MySQLDBName,
		)

		DB, err = gorm.Open(mysql.Open(dsn), gormCfg)
		if err != nil {
			// Elegant fallback to SQLite for easy development/review!
			fallbackPath := "data/server_fallback.db"
			logger.Warn("DB", "Failed to connect to MySQL — activating SQLite fallback",
				"error", err,
				"fallback", fallbackPath,
			)

			dir := filepath.Dir(fallbackPath)
			_ = os.MkdirAll(dir, 0755)

			DB, err = gorm.Open(sqlite.Open(fallbackPath), gormCfg)
			if err != nil {
				logger.Fatal("DB", "Database initialization failed completely", "error", err)
			}
			logger.Info("DB", "SQLite fallback connection established", "path", fallbackPath)
		} else {
			logger.Info("DB", "MySQL connection established",
				"host", cfg.MySQLHost,
				"database", cfg.MySQLDBName,
			)
		}
	}

	// Auto Migration
	logger.Info("DB", "Executing automatic database schema migrations")
	migrateDB := DB
	if DB.Dialector.Name() == "mysql" {
		migrateDB = DB.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci")
	}
	if err := migrateDB.AutoMigrate(&models.User{}, &models.ClientSession{}, &models.EhcoServerConfig{}, &models.EhcoClientConfig{}, &models.LeechConfig{}, &models.LeechJob{}, &models.TelegramConfig{}); err != nil {
		logger.Fatal("DB", "Auto migration failed", "error", err)
	}
	
	// Ensure the table collation is converted to utf8mb4 to support emoji/symbols in welcome messages
	if DB.Dialector.Name() == "mysql" {
		DB.Exec("ALTER TABLE `telegram_configs` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	}
	logger.Info("DB", "Schema migrations completed successfully")

	// Seed default LeechConfig
	var leechCfg models.LeechConfig
	if err := DB.First(&leechCfg).Error; err != nil {
		logger.Info("DB", "Seeding default remote downloader configuration")
		DB.Create(&models.LeechConfig{
			DefaultSavePath: "./downloads",
			MaxConcurrent:   3,
			ThreadsPerJob:   8,
			UserAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
		})
	}

	// Seed Admin User
	seedAdmin(cfg)

	return DB
}

func seedAdmin(cfg *config.Config) {
	var admin models.User
	result := DB.Where("username = ?", cfg.AdminUsername).First(&admin)
	if result.Error != nil {
		logger.Info("DB", "Seeding administrator account", "username", cfg.AdminUsername)

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
		if err != nil {
			logger.Fatal("DB", "Failed to hash seeded password", "error", err)
		}

		admin = models.User{
			Username: cfg.AdminUsername,
			Password: string(hashedPassword),
			Role:     "admin",
		}

		if err := DB.Create(&admin).Error; err != nil {
			logger.Fatal("DB", "Failed to seed administrator", "error", err)
		}
		logger.Info("DB", "Administrator seeded successfully", "username", cfg.AdminUsername)
	} else {
		logger.Info("DB", "Seed integrity validated — administrator account exists", "username", cfg.AdminUsername)
	}
}
