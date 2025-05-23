package config

import (
	"fmt"
	"os"
	"strconv"
	"strings" // For LogLevel normalization

	"github.com/joho/godotenv"
)

// AppConfig holds all configuration for the application
type AppConfig struct {
	TelegramToken     string
	DatabaseURL       string
	AdminTelegramID   int64
	ManagerTelegramID int64
	LogLevel          string
	Environment       string
}

// Load reads configuration from environment variables and .env file (if present).
func Load() (*AppConfig, error) {
	// Attempt to load .env file. Errors are ignored if the file doesn't exist.
	// godotenv.Load will not override existing env variables.
	_ = godotenv.Load()

	cfg := &AppConfig{}
	var err error

	cfg.TelegramToken = os.Getenv("TELEGRAM_TOKEN")
	if cfg.TelegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_TOKEN is not set")
	}

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	adminIDStr := os.Getenv("ADMIN_TELEGRAM_ID")
	if adminIDStr == "" {
		return nil, fmt.Errorf("ADMIN_TELEGRAM_ID is not set")
	}
	cfg.AdminTelegramID, err = strconv.ParseInt(adminIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ADMIN_TELEGRAM_ID: %w", err)
	}

	managerIDStr := os.Getenv("MANAGER_TELEGRAM_ID")
	if managerIDStr == "" {
		return nil, fmt.Errorf("MANAGER_TELEGRAM_ID is not set")
	}
	cfg.ManagerTelegramID, err = strconv.ParseInt(managerIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid MANAGER_TELEGRAM_ID: %w", err)
	}

	cfg.LogLevel = strings.ToLower(os.Getenv("LOG_LEVEL"))
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info" // Default log level
	}

	cfg.Environment = strings.ToLower(os.Getenv("ENVIRONMENT"))
	if cfg.Environment == "" {
		cfg.Environment = "development" // Default environment
	}

	return cfg, nil
}
