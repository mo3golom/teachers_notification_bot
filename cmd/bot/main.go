package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"teacher_notification_bot/internal/app"
	"teacher_notification_bot/internal/infra/config"
	idb "teacher_notification_bot/internal/infra/database"
	"teacher_notification_bot/internal/infra/scheduler"
	"teacher_notification_bot/internal/infra/telegram"

	"gopkg.in/telebot.v3"
)

func main() {
	fmt.Println("Teacher Notification Bot starting...")

	mainLogger := log.New(os.Stdout, "MAIN: ", log.LstdFlags|log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		mainLogger.Fatalf("FATAL: Could not load application configuration: %v", err)
	}

	mainLogger.Printf("INFO: Configuration loaded. LogLevel: %s, Environment: %s, Admin ID: %d", cfg.LogLevel, cfg.Environment, cfg.AdminTelegramID)

	// Initialize Database Connection
	db, err := idb.NewPostgresConnection(cfg.DatabaseURL)
	if err != nil {
		mainLogger.Fatalf("FATAL: Could not connect to database: %v", err)
	}
	defer db.Close()
	mainLogger.Println("INFO: Database connection established successfully.")

	// Initialize Repositories
	teacherRepo := idb.NewPostgresTeacherRepository(db)
	mainLogger.Println("INFO: Teacher repository initialized.")
	notificationRepo := idb.NewPostgresNotificationRepository(db)
	mainLogger.Println("INFO: Notification repository initialized.")

	// Initialize AdminService
	adminService := app.NewAdminService(teacherRepo, notificationRepo, cfg.AdminTelegramID)
	mainLogger.Println("INFO: Admin service initialized.")

	// Initialize (Mock) NotificationService
	mockNotifServiceLogger := log.New(os.Stdout, "MOCK_NOTIF_SVC: ", log.LstdFlags|log.Lshortfile)
	mockNotificationService := app.NewMockNotificationService(mockNotifServiceLogger) // Using the mock
	mainLogger.Println("INFO: Mock Notification service initialized.")

	// Initialize NotificationScheduler
	schedulerLogger := log.New(os.Stdout, "SCHEDULER: ", log.LstdFlags|log.Lshortfile)
	notifScheduler := scheduler.NewNotificationScheduler(
		mockNotificationService, // Pass the mock service
		notificationRepo,
		schedulerLogger,
		cfg.CronSpec15th,
		cfg.CronSpecDaily,
	)
	notifScheduler.Start() // Start the cron jobs

	// Initialize Telegram Bot
	pref := telebot.Settings{
		Token:  cfg.TelegramToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
		OnError: func(err error, c telebot.Context) { // Global error handler
			log.Printf("ERROR (telebot): %v", err)
			if c != nil && c.Sender() != nil && c.Chat() != nil {
				log.Printf("ERROR (telebot context): Message: %s, Sender: %d, Chat: %d", c.Text(), c.Sender().ID, c.Chat().ID)
			}
		},
	}
	bot, err := telebot.NewBot(pref)
	if err != nil {
		mainLogger.Fatalf("FATAL: Could not create Telegram bot: %v", err)
	}

	// Register Handlers
	telegram.RegisterAdminHandlers(bot, adminService, cfg.AdminTelegramID) // Pass configured Admin ID
	mainLogger.Println("INFO: Admin command handlers registered.")

	mainLogger.Println("INFO: Application setup complete. Bot and Scheduler are starting...")

	// Start bot in a goroutine so it doesn't block graceful shutdown handling
	go bot.Start()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // Block until a signal is received

	mainLogger.Println("INFO: Shutting down application...")
	notifScheduler.Stop()
	// bot.Stop() // If your bot library has a stop method, call it. Telebot poller stops on its own.
	// db.Close() is handled by defer
	mainLogger.Println("INFO: Application shut down gracefully.")
}
