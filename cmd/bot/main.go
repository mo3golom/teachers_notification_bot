package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"teacher_notification_bot/internal/app"
	"teacher_notification_bot/internal/infra/config"
	idb "teacher_notification_bot/internal/infra/database"
	"teacher_notification_bot/internal/infra/logger"
	"teacher_notification_bot/internal/infra/scheduler"
	"teacher_notification_bot/internal/infra/telegram"

	"github.com/sirupsen/logrus"
	"gopkg.in/telebot.v3"
)

func main() {
	fmt.Println("Teacher Notification Bot starting...")

	ctx := context.Background()

	// Logger not yet initialized, use standard log for this critical bootstrap error
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: Could not load application configuration: %v", err)
	}

	// Initialize Logger (AFTER config is loaded)
	logger.Init(cfg)

	logger.Log.Info("Teacher Notification Bot starting...")
	logger.Log.Infof("Configuration loaded. LogLevel: %s, Environment: %s, Admin ID: %d, Manager ID: %d", cfg.LogLevel, cfg.Environment, cfg.AdminTelegramID, cfg.ManagerTelegramID)

	// Initialize Database Connection
	db, err := idb.NewPostgresConnection(cfg.DatabaseURL)
	if err != nil {
		logger.Log.Fatalf("FATAL: Could not connect to database: %v", err)
	}
	// defer db.Close() // Explicit close during graceful shutdown
	logger.Log.Info("Database connection established successfully.")

	// Initialize Repositories
	teacherRepo := idb.NewPostgresTeacherRepository(db)
	notificationRepo := idb.NewPostgresNotificationRepository(db)
	logger.Log.Info("Repositories initialized.")

	// Initialize AdminService
	adminLogger := logger.Log.WithField("service", "AdminService")
	adminService := app.NewAdminService(teacherRepo, cfg.AdminTelegramID, adminLogger)

	// Initialize Telegram Bot
	pref := telebot.Settings{
		Token:  cfg.TelegramToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
		OnError: func(err error, c telebot.Context) { // Global bot error handler
			entry := logger.Log.WithError(err).WithField("component", "telebot_global_error_handler")
			if c != nil {
				fields := logrus.Fields{}
				if s := c.Sender(); s != nil {
					fields["sender_id"] = s.ID
				}
				// c.Text() gets text from message or callback query.
				if text := c.Text(); text != "" {
					fields["context_text"] = text
				}
				if cb := c.Callback(); cb != nil {
					fields["callback_data"] = cb.Data
				}
				// Add chat ID if available
				if chat := c.Chat(); chat != nil {
					fields["chat_id"] = chat.ID
				}
				entry = entry.WithFields(fields)
			}
			entry.Error("Telebot encountered an error")
		},
	}
	bot, err := telebot.NewBot(pref)
	if err != nil {
		logger.Log.Fatalf("FATAL: Could not create Telegram bot: %v", err)
	}

	// Create TelebotAdapter
	telegramClientAdapter := telegram.NewTelebotAdapter(bot)

	// Initialize REAL NotificationService
	notifServiceLogger := logger.Log.WithField("service", "NotificationService")
	notificationService := app.NewNotificationServiceImpl(
		teacherRepo,
		notificationRepo,
		telegramClientAdapter,
		notifServiceLogger,
		cfg.ManagerTelegramID, // Pass ManagerTelegramID
	)
	logger.Log.Info("Application services initialized.")

	// Initialize NotificationScheduler
	schedulerLogger := logger.Log.WithField("component", "NotificationScheduler")
	notifScheduler := scheduler.NewNotificationScheduler(
		notificationService, // Pass the REAL service
		notificationRepo,
		schedulerLogger,
		cfg.CronSpec15th,
		cfg.CronSpecDailyCheckForLastDay,
		cfg.CronSpecReminderCheck,
		cfg.CronSpecNextDayCheck,
	)
	logger.Log.Info("Notification scheduler initialized.")

	notifScheduler.Start() // Start the cron jobs

	// Register Handlers
	telegram.RegisterAdminHandlers(ctx, bot, adminService, cfg.AdminTelegramID, logger.Log.WithField("handler_group", "admin"))
	telegram.RegisterTeacherResponseHandlers(ctx, bot, notificationService, logger.Log.WithField("handler_group", "teacher_response"))
	logger.Log.Info("Command handlers registered.")

	logger.Log.Info("Application setup complete. Bot and Scheduler are starting...")

	// Start bot in a goroutine so it doesn't block graceful shutdown handling
	go bot.Start()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-quit // Block until a signal is received

	logger.Log.Info("Shutting down application...")
	notifScheduler.Stop()
	db.Close() // Explicitly close DB connection
	// bot.Stop() // If your bot library has a stop method, call it. Telebot poller stops on its own.
	// db.Close() is handled by defer
	logger.Log.Info("Application shut down gracefully.")
}
