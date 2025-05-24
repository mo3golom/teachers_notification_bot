// internal/infra/logger/logger.go
package logger

import (
	"os"
	"strings"
	"teacher_notification_bot/internal/infra/config"

	"github.com/sirupsen/logrus"
)

// Log is the global logger instance
var Log = logrus.New()

// Init initializes the global logger based on application configuration.
func Init(cfg *config.AppConfig) {
	Log.SetOutput(os.Stdout) // Default output

	// Set Log Level
	level, err := logrus.ParseLevel(strings.ToLower(cfg.LogLevel))
	if err != nil {
		Log.Warnf("Invalid log level '%s', defaulting to 'info'. Error: %v", cfg.LogLevel, err)
		Log.SetLevel(logrus.InfoLevel)
	} else {
		Log.SetLevel(level)
	}

	// Set Log Formatter
	if strings.ToLower(cfg.Environment) == "production" || strings.ToLower(cfg.Environment) == "staging" {
		Log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00", // ISO8601
		})
	} else { // Development or other environments
		Log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			ForceColors:     true, // Or based on TTY
		})
	}

	Log.Info("Logger initialized successfully.")
	Log.Debugf("Log level set to: %s", Log.GetLevel().String())
	Log.Debugf("Log format set for environment: %s", cfg.Environment)
}

// Get returns the configured global logger.
// Useful if you want to avoid direct global var usage in some places, though direct use of logger.Log is common with logrus.
func Get() *logrus.Logger {
	return Log
}
