## Backend Task: B015 - Implement Structured Logging Throughout the Application

**Objective:**
To replace the Go standard library's `log` package with `logrus`, a structured logging library, across all application components. This involves implementing consistent log levels, configurable formatting (JSON for production, text for development), and enriching log entries with relevant contextual information (e.g., UserID, CycleID, ReportStatusID, error details).

**Background:**
Effective and structured logging is essential for monitoring application health, debugging issues, and understanding system behavior, especially in a production environment. The PRD (NFR6) calls for basic logging[cite: 83], and the tech stack notes suggest `logrus` as a suitable option[cite: 90]. This task aims to elevate our logging capabilities for better maintainability and observability.

**Tech Stack:**
* Go version: 1.24
* Logging Library: `github.com/sirupsen/logrus` (already a dependency)
* Configuration: `AppConfig` (for log level and environment)

---

**Steps to Completion:**

1.  **Implement Logger Initialization (`internal/infra/logger/logger.go`):**
    * Create the file `internal/infra/logger/logger.go`.
    * Add a function to initialize and configure a global `logrus` instance or a base logger.
    ```go
    // internal/infra/logger/logger.go
    package logger

    import (
        "os"
        "strings"
        "teacher_notification_bot/internal/infra/config" // Assuming config is accessible or passed

        "[github.com/sirupsen/logrus](https://github.com/sirupsen/logrus)"
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
                // FieldMap: logrus.FieldMap{ // Optional: rename default fields
                //  logrus.FieldKeyTime:  "@timestamp",
                //  logrus.FieldKeyLevel: "@level",
                //  logrus.FieldKeyMsg:   "@message",
                // },
            })
        } else { // Development or other environments
            Log.SetFormatter(&logrus.TextFormatter{
                FullTimestamp:   true,
                TimestampFormat: "2006-01-02 15:04:05",
                ForceColors:     true, // Or based on TTY
                // DisableColors: false,
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

    // For more advanced usage, you might create a wrapper that allows easy context addition.
    // For now, using the global Logrus instance directly with WithFields is acceptable.
    ```

2.  **Update `main.go` to Initialize and Use `logrus`:**
    * Call `logger.Init()` early in `main()` after `AppConfig` is loaded.
    * Replace all `log.Printf`, `log.Fatalf`, etc., calls with `logger.Log.Info`, `logger.Log.Fatalf`, etc.
    ```go
    // cmd/bot/main.go
    // import "teacher_notification_bot/internal/infra/logger" // Add this
    // Remove standard "log" if no longer directly used, or alias it if needed for dependencies.

    func main() {
        // fmt.Println("Teacher Notification Bot starting...") // Can be replaced by logger.Log.Info

        cfg, err := config.Load()
        if err != nil {
            // Logger not yet initialized, use standard log for this critical bootstrap error
            log.Fatalf("FATAL: Could not load application configuration: %v", err)
        }

        // Initialize Logger (AFTER config is loaded)
        logger.Init(cfg) // Use the global logger.Log from now on

        logger.Log.Info("Teacher Notification Bot starting...")
        logger.Log.Infof("Configuration loaded. Admin ID: %d, Manager ID: %d", cfg.AdminTelegramID, cfg.ManagerTelegramID)


        db, err := idb.NewPostgresConnection(cfg.DatabaseURL)
        if err != nil {
            logger.Log.Fatalf("FATAL: Could not connect to database: %v", err)
        }
        defer db.Close()
        logger.Log.Info("Database connection established successfully.")

        // Initialize Repositories
        teacherRepo := idb.NewPostgresTeacherRepository(db) // Consider passing logger.Log.WithField("component", "TeacherRepo")
        notificationRepo := idb.NewPostgresNotificationRepository(db)
        logger.Log.Info("Repositories initialized.")

        // Initialize Services
        // Pass logger.Log or a context-specific logger.Log.WithField("service", "AdminService")
        adminLogger := logger.Log.WithField("service", "AdminService")
        adminService := app.NewAdminService(teacherRepo, cfg.AdminTelegramID, adminLogger) // Modify NewAdminService to accept logger

        telegramClientAdapter := telegram.NewTelebotAdapter(bot) // Bot needs to be initialized before this
        
        notifServiceLogger := logger.Log.WithField("service", "NotificationService")
        notificationService := app.NewNotificationServiceImpl(
            teacherRepo,
            notificationRepo,
            telegramClientAdapter,
            notifServiceLogger, // Pass the contextual logger
            cfg.ManagerTelegramID,
        )
        logger.Log.Info("Application services initialized.")

        // Initialize Telegram Bot (move this up if telegramClientAdapter needs it earlier)
        pref := telebot.Settings{
            Token: cfg.TelegramToken,
            Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
            OnError: func(err error, c telebot.Context) { // Global bot error handler
                entry := logger.Log.WithError(err).WithField("component", "telebot_global_error_handler")
                if c != nil {
                    entry = entry.WithFields(logrus.Fields{
                        "callback_data":  c.Callback().Data,
                        "message_text":   c.Text(),
                        "sender_id":      c.Sender().ID,
                        // Add other relevant context from c
                    })
                }
                entry.Error("Telebot encountered an error")
            },
        }
        bot, err := telebot.NewBot(pref)
        if err != nil {
            logger.Log.Fatalf("FATAL: Could not create Telegram bot: %v", err)
        }
        // Re-initialize adapter if bot is created after its first use.
        // It's better to initialize bot first, then adapter, then services needing the adapter.
        // (Corrected order assumed for this snippet: bot init, then adapter, then services)


        // Initialize Scheduler
        schedulerLogger := logger.Log.WithField("component", "NotificationScheduler")
        notifScheduler := scheduler.NewNotificationScheduler(
            notificationService,
            notificationRepo,
            schedulerLogger, // Pass contextual logger
            cfg.CronSpec15th,
            cfg.CronSpecDaily,
            cfg.CronSpecReminderCheck, // Added in B012
            cfg.CronSpecNextDayCheck,  // Added in B013 (was B015 in thought process)
        )
        
        // Register Handlers
        // Handlers should also ideally get a logger instance or use a request-scoped logger
        telegram.RegisterAdminHandlers(bot, adminService, cfg.AdminTelegramID, logger.Log.WithField("handler_group", "admin"))
        telegram.RegisterTeacherResponseHandlers(bot, notificationService, logger.Log.WithField("handler_group", "teacher_response"))
        logger.Log.Info("Command handlers registered.")

        notifScheduler.Start()
        logger.Log.Info("Application setup complete. Bot and Scheduler are starting...")
        go bot.Start()

        // Graceful shutdown
        // ... (use logger.Log for shutdown messages)
    }
    ```
    * **Refinement:** Services and handlers should accept `*logrus.Entry` or `*logrus.Logger` via their constructors/registration functions.

3.  **Refactor Services, Handlers, Scheduler, Repositories (Example for one service):**
    * **Modify Service Constructor & Methods:**
        * Example for `AdminService` (`internal/app/admin_service.go`):
        ```go
        // internal/app/admin_service.go
        // import "[github.com/sirupsen/logrus](https://github.com/sirupsen/logrus)"

        type AdminService struct {
            teacherRepo     teacher.Repository
            adminTelegramID int64
            log             *logrus.Entry // Use Entry for service-specific context
        }

        func NewAdminService(tr teacher.Repository, adminID int64, baseLogger *logrus.Logger /* or *logrus.Entry */) *AdminService {
            return &AdminService{
                teacherRepo:     tr,
                adminTelegramID: adminID,
                log:             baseLogger.WithField("service", "AdminService"), // Add service context
            }
        }

        func (s *AdminService) AddTeacher(ctx context.Context, performingAdminID int64, /*...*/) (*teacher.Teacher, error) {
            s.log.WithFields(logrus.Fields{
                "performing_admin_id": performingAdminID,
                "new_teacher_tg_id": newTeacherTelegramID,
            }).Info("Attempting to add teacher")

            if performingAdminID != s.adminTelegramID {
                s.log.WithField("performing_admin_id", performingAdminID).Warn("Unauthorized attempt to add teacher")
                return nil, ErrAdminNotAuthorized
            }
            // ... existing logic ...
            // On error:
            // return nil, fmt.Errorf("failed to check existing teacher: %w", err)
            // Becomes:
            // s.log.WithError(err).Error("Failed to check existing teacher")
            // return nil, fmt.Errorf("failed to check existing teacher: %w", err) // Keep wrapping for app errors

            // On success:
            // s.log.WithFields(logrus.Fields{"teacher_id": newTeacher.ID, "telegram_id": newTeacher.TelegramID}).Info("Teacher added successfully")
        }
        ```
    * **Apply similar changes to:**
        * `NotificationServiceImpl` (constructor and all methods).
        * Telegram Handlers (pass `logger.Log.WithField("handler", "/command_name")` or similar to handler registration, then use it).
        * `NotificationScheduler` (already shown in `main.go` example).
        * Repositories: Optionally pass logger if detailed DB query logging is desired, though service-level logging of repository outcomes is often sufficient. For now, focus on service/handler/scheduler.

4.  **Standardize Log Messages & Contextual Fields:**
    * Review all new `logrus` calls. Ensure:
        * Consistent use of log levels (e.g., `Info` for regular operations, `Warn` for recoverable issues/unexpected but handled states, `Error` for operational errors, `Debug` for verbose dev info).
        * Key identifiers (e.g., `teacher_id`, `telegram_id`, `cycle_id`, `report_status_id`, `admin_id`, `handler_name`, `job_name`) are included as fields using `WithField` or `WithFields`.
        * Error logging always uses `WithError(err)`.

---

**Acceptance Criteria:**

* A global/base `logrus` logger is initialized in `internal/infra/logger/logger.go`, with its level and formatter (JSON for prod/staging, Text for dev) configured via `AppConfig` (`LogLevel`, `Environment`).
* All previous uses of the standard `log` package in `main.go`, application services (`AdminService`, `NotificationService`), Telegram command/callback handlers, and the scheduler (`NotificationScheduler`) are replaced with `logrus` calls.
* Services, handlers, and the scheduler are provided with a `logrus.Entry` (or `logrus.Logger`) instance, typically with added component-specific context (e.g., `logger.Log.WithField("service", "NotificationService")`).
* Log messages are structured and consistently include relevant contextual fields (e.g., `teacher_id`, `cycle_id`, `error_details`, `operation_name`).
* Errors are logged using `logrus.WithError(err)` along with contextual fields.
* When `AppConfig.Environment` is "production" or "staging", logs are output in JSON format.
* When `AppConfig.Environment` is "development", logs are output in a human-readable, colored (if terminal supports) text format.
* The `AppConfig.LogLevel` correctly controls the verbosity of the logs (e.g., "debug" shows more than "info").
* Fatal errors during application startup (e.g., config load failure, DB connection issues) use `logrus.Fatalf` and correctly terminate the application.

---

**Critical Tests (Manual & Log Inspection):**

1.  **Development Logging Format & Level:**
    * **Setup:** Set `LOG_LEVEL="debug"` and `ENVIRONMENT="development"` in `.env` or environment variables.
    * **Action:** Start the application. Perform a few admin actions (e.g., `/add_teacher`, `/list_teachers`) and simulate a teacher responding to a notification. Trigger a scheduler job manually (by adjusting cron spec).
    * **Verify:** Logs appear in the console in colored, human-readable text format. `DEBUG`, `INFO`, `WARN`, `ERROR` messages are all visible. Contextual fields (like `service`, `handler`, IDs) are present in the log lines.
2.  **Production Logging Format & Level:**
    * **Setup:** Set `LOG_LEVEL="info"` and `ENVIRONMENT="production"` in `.env`.
    * **Action:** Restart the application. Perform similar actions as in Test 1.
    * **Verify:** Logs appear in the console in JSON format. `DEBUG` messages are *not* visible. `INFO`, `WARN`, `ERROR` messages are present as JSON objects. Contextual fields are keys within the JSON objects.
3.  **Log Level Control Test:**
    * **Setup:** Set `LOG_LEVEL="warn"` and `ENVIRONMENT="development"`.
    * **Action:** Perform actions that would typically generate `INFO` and `DEBUG` logs (e.g., successful command execution). Then, perform an action that generates a `WARN` or `ERROR` (e.g., try to add an existing teacher, which should log a warning/error at service level if not just a user message).
    * **Verify:** `INFO` and `DEBUG` logs are suppressed. `WARN` and `ERROR` logs are visible.
4.  **Contextual Fields Verification:**
    * During any test run (e.g., Test 1 or 2), pick a few key log entries (e.g., "Attempting to add teacher", "Processing 'Yes' response", "Scheduler job triggered").
    * **Verify:** Confirm that relevant IDs (e.g., `performing_admin_id`, `new_teacher_tg_id` for add; `report_status_id` for response; `job_name` for scheduler) and other useful context are present as fields in the structured log output.
5.  **Error Logging Format (`WithError`):**
    * **Action:** Intentionally trigger a code path that results in an error being logged (e.g., by providing invalid data that passes command parsing but fails service-level validation that logs an error, or by temporarily causing a downstream dependency like the DB to fail if tests allow for it).
    * **Verify:** The error log entry includes the error message and stack trace (if available/configured) via the `error` field (or similar, as produced by `logrus.WithError()`), along with other contextual fields.
6.  **`Fatalf` on Startup:**
    * **Action:** Temporarily corrupt `DATABASE_URL` in `.env` to be invalid.
    * **Verify:** The application fails to start, and a `FATAL` level log message (from `logrus.Fatalf`) regarding the DB connection failure is printed before exit.