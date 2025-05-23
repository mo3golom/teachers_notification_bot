package main

import (
	"fmt"
	"log" // Using standard log for now, will replace with structured logger later
	"os"  // For os.Exit

	"teacher_notification_bot/internal/infra/config" // Adjust import path if needed
)

func main() {
	fmt.Println("Teacher Notification Bot starting...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: Could not load application configuration: %v", err)
		os.Exit(1) // Ensure application exits
	}

	log.Printf("INFO: Configuration loaded successfully. LogLevel: %s, Environment: %s", cfg.LogLevel, cfg.Environment)
	// Placeholder for further application startup logic using cfg
}
