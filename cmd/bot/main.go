package main

import (
	"fmt"
	"log"
	"time"

	"teacher_notification_bot/internal/app"
	"teacher_notification_bot/internal/infra/config"
	idb "teacher_notification_bot/internal/infra/database"
	"teacher_notification_bot/internal/infra/telegram"

	"gopkg.in/telebot.v3"
)

func main() {
	fmt.Println("Teacher Notification Bot starting...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: Could not load application configuration: %v", err)
	}

	log.Printf("INFO: Configuration loaded. LogLevel: %s, Environment: %s, Admin ID: %d", cfg.LogLevel, cfg.Environment, cfg.AdminTelegramID)

	// Initialize Database Connection
	db, err := idb.NewPostgresConnection(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("FATAL: Could not connect to database: %v", err)
	}
	defer db.Close()
	log.Println("INFO: Database connection established successfully.")

	// Initialize Repositories
	teacherRepo := idb.NewPostgresTeacherRepository(db)
	log.Println("INFO: Teacher repository initialized.")
	notificationRepo := idb.NewPostgresNotificationRepository(db)
	log.Println("INFO: Notification repository initialized.")

	// Initialize AdminService
	adminService := app.NewAdminService(teacherRepo, notificationRepo, cfg.AdminTelegramID)
	log.Println("INFO: Admin service initialized.")

	// Initialize Telegram Bot
	pref := telebot.Settings{
		Token:  cfg.TelegramToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
		OnError: func(err error, c telebot.Context) { // Global error handler
			log.Printf("ERROR (telebot): %v", err)
			if c != nil {
				log.Printf("ERROR (telebot context): Message: %s, Sender: %d, Chat: %d", c.Text(), c.Sender().ID, c.Chat().ID)
			}
		},
	}
	bot, err := telebot.NewBot(pref)
	if err != nil {
		log.Fatalf("FATAL: Could not create Telegram bot: %v", err)
	}

	// Register Handlers
	telegram.RegisterAdminHandlers(bot, adminService, cfg.AdminTelegramID) // Pass configured Admin ID
	log.Println("INFO: Admin command handlers registered.")

	log.Println("INFO: Application setup complete. Bot is starting...")
	bot.Start() // Start the bot
	// Placeholder for bot startup logic
	// select {} // Keep alive for bot (actual bot loop will be different)
}
