## Backend Task: B009 - Implement Core Notification Scheduler

**Objective:**
To set up an automated cron job scheduler within the Go application. This scheduler will be responsible for triggering the notification process on two specific occasions each month: on the 15th day and on the last day, both at a predefined time (e.g., 10:00 AM server time).

**Background:**
The Product Requirements Document (PRD) mandates automatic, scheduled notifications to teachers[cite: 7, 14]. This scheduler is the engine that will initiate these notification workflows (FR2.1, FR2.2 [cite: 56, 57]), ensuring timely reminders without manual intervention.

**Tech Stack:**
* Go version: 1.24
* Cron Library: `github.com/robfig/cron/v3`
* Dependencies: `notification.Repository` (from B008), `AppConfig`

---

**Steps to Completion:**

1.  **Define Placeholder `NotificationAppService` Interface:**
    * Create `internal/app/notification_service.go` (if it doesn't exist).
    * Define a placeholder interface for the `NotificationService`. This service will eventually contain the core logic for initiating notifications, but for this task, we only need its method signature for the scheduler to call.
    ```go
    // internal/app/notification_service.go
    package app

    import (
        "context"
        "teacher_notification_bot/internal/domain/notification" // Adjust import path
    )

    // NotificationService defines the operations for managing the notification process.
    // This is a placeholder for now; its full implementation will come in later tasks.
    type NotificationService interface {
        // InitiateNotificationProcess starts the notification workflow for a given cycle type.
        // It will find/create a NotificationCycle, identify target teachers,
        // create initial TeacherReportStatus entries, and send the first notifications.
        InitiateNotificationProcess(ctx context.Context, cycleType notification.CycleType, cycleDate time.Time) error
    }

    // Mock implementation for now, just to allow scheduler to compile and run.
    // This will be replaced by the actual service implementation later.
    type MockNotificationService struct {
        // Add any dependencies this mock might need, e.g., a logger
        logger *log.Logger // Example: using standard log
    }

    func NewMockNotificationService(logger *log.Logger) *MockNotificationService {
        return &MockNotificationService{logger: logger}
    }

    func (m *MockNotificationService) InitiateNotificationProcess(ctx context.Context, cycleType notification.CycleType, cycleDate time.Time) error {
        // In a real implementation, we'd create a notification.Cycle here first if it doesn't exist for cycleDate & cycleType
        // then proceed with the notification logic.
        m.logger.Printf("INFO (MockNotificationService): InitiateNotificationProcess called for CycleType: %s, Date: %s", cycleType, cycleDate.Format("2006-01-02"))
        // Simulate work or simply log for now.
        return nil
    }
    ```
    * **Note:** For the scheduler task, we will use this `MockNotificationService`. The real `NotificationService` will be built in subsequent tasks.

2.  **Implement `NotificationScheduler`:**
    * Create `internal/infra/scheduler/scheduler.go`.
    * Define the `Scheduler` struct and its methods. It will depend on the `notification.Repository`, the (mock) `app.NotificationService`, and a logger.
    ```go
    // internal/infra/scheduler/scheduler.go
    package scheduler

    import (
        "context"
        "log"
        "teacher_notification_bot/internal/app" // For NotificationService interface
        "teacher_notification_bot/internal/domain/notification"
        "time"

        "[github.com/robfig/cron/v3](https://github.com/robfig/cron/v3)"
    )

    type NotificationScheduler struct {
        cronEngine      *cron.Cron
        notifService    app.NotificationService // Using the interface
        notifRepo       notification.Repository
        logger          *log.Logger
        cronSpec15th    string
        cronSpecLastDay string // This will run daily, logic inside checks if it's the last day
    }

    func NewNotificationScheduler(
        notifService app.NotificationService,
        notifRepo notification.Repository,
        logger *log.Logger,
        cronSpec15th string, // e.g., "0 10 15 * *" (10:00 AM on 15th)
        cronSpecDailyCheckForLastDay string, // e.g., "0 10 * * *" (10:00 AM daily)
    ) *NotificationScheduler {
        return &NotificationScheduler{
            cronEngine:      cron.New(cron.WithLocation(time.Local)), // Use server's local time for cron
            notifService:    notifService,
            notifRepo:       notifRepo,
            logger:          logger,
            cronSpec15th:    cronSpec15th,
            cronSpecLastDay: cronSpecDailyCheckForLastDay,
        }
    }

    func (s *NotificationScheduler) Start() {
        s.logger.Println("INFO: Starting notification scheduler...")

        // Job for the 15th of the month
        _, err := s.cronEngine.AddFunc(s.cronSpec15th, func() {
            s.logger.Println("INFO: Cron job triggered for 15th of month.")
            s.executeNotificationProcess(notification.CycleTypeMidMonth)
        })
        if err != nil {
            s.logger.Fatalf("FATAL: Could not add 15th of month cron job: %v", err)
        }

        // Job that runs daily but only proceeds if it's the last day of the month
        _, err = s.cronEngine.AddFunc(s.cronSpecLastDay, func() {
            s.logger.Println("INFO: Daily cron job triggered for last day check.")
            now := time.Now()
            // Calculate the first day of the next month, then subtract one day to get the last day of the current month.
            firstOfNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
            lastDayOfCurrentMonth := firstOfNextMonth.AddDate(0, 0, -1)

            if now.Day() == lastDayOfCurrentMonth.Day() {
                s.logger.Println("INFO: Today is the last day of the month. Executing end-of-month notification process.")
                s.executeNotificationProcess(notification.CycleTypeEndMonth)
            } else {
                s.logger.Printf("INFO: Today (Day %d) is not the last day of the month (Day %d). Skipping end-of-month process.", now.Day(), lastDayOfCurrentMonth.Day())
            }
        })
        if err != nil {
            s.logger.Fatalf("FATAL: Could not add last day of month cron job: %v", err)
        }

        s.cronEngine.Start()
        s.logger.Println("INFO: Notification scheduler started with jobs.")
    }
    
    // executeNotificationProcess is a helper to handle the common logic for both job types
    func (s *NotificationScheduler) executeNotificationProcess(cycleType notification.CycleType) {
        ctx := context.Background() // Or a more specific context if available
        today := time.Now()
        // Normalize to just the date part for cycleDate consistency
        cycleDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

        // Check if a cycle for this date and type already exists to prevent duplicates
        existingCycle, err := s.notifRepo.GetCycleByDateAndType(ctx, cycleDate, cycleType)
        if err != nil && err != database.ErrCycleNotFound { // database should be your alias for infra/database
            s.logger.Printf("ERROR: Failed to check for existing notification cycle: %v", err)
            return
        }
        if existingCycle != nil {
            s.logger.Printf("INFO: Notification cycle for date %s and type %s already exists (ID: %d). Skipping creation.",
                cycleDate.Format("2006-01-02"), cycleType, existingCycle.ID)
            // Optionally, you might still want to trigger the InitiateNotificationProcess
            // if it's designed to be idempotent or handle existing cycles.
            // For now, let's assume we only process if a new cycle is effectively started here.
            // OR, we always call InitiateNotificationProcess and let IT handle finding/creating the cycle.
            // Let's adjust to always call, and InitiateNotificationProcess will be responsible for idempotency/logic.
        } else {
            s.logger.Printf("INFO: No existing cycle found for date %s and type %s. A new cycle will be implied by InitiateNotificationProcess.",
                 cycleDate.Format("2006-01-02"), cycleType)
            // The actual cycle creation will now be handled within InitiateNotificationProcess or by a call before it.
            // For this task, the main responsibility of the scheduler is to *trigger* the process.
            // The NotificationService will be responsible for creating the Cycle DB record.
        }

        // Call the (mock) application service to start the actual notification process
        // Pass cycleDate so the service knows which date this trigger is for.
        if err := s.notifService.InitiateNotificationProcess(ctx, cycleType, cycleDate); err != nil {
            s.logger.Printf("ERROR: Error during notification process for type %s: %v", cycleType, err)
        } else {
            s.logger.Printf("INFO: Notification process for type %s initiated successfully.", cycleType)
        }
    }


    func (s *NotificationScheduler) Stop() {
        s.logger.Println("INFO: Stopping notification scheduler...")
        ctx := s.cronEngine.Stop() // Stops the scheduler from adding new jobs, waits for running jobs.
        select {
        case <-ctx.Done():
            s.logger.Println("INFO: Notification scheduler gracefully stopped.")
        case <-time.After(5 * time.Second): // Timeout for graceful shutdown
            s.logger.Println("WARN: Notification scheduler stop timed out.")
        }
    }
    ```

3.  **Update `AppConfig` and `.env.example`:**
    * Add cron schedule expressions to `internal/infra/config/config.go`:
    ```go
    // internal/infra/config/config.go
    type AppConfig struct {
        // ... existing fields
        CronSpec15th    string
        CronSpecDaily   string // For the daily check for last day of month
    }

    // In Load() function:
    // ...
    cfg.CronSpec15th = os.Getenv("CRON_SPEC_15TH")
    if cfg.CronSpec15th == "" {
        cfg.CronSpec15th = "0 10 15 * *" // Default: 10:00 AM on 15th
    }
    cfg.CronSpecDaily = os.Getenv("CRON_SPEC_DAILY_FOR_LAST_DAY_CHECK")
    if cfg.CronSpecDaily == "" {
        cfg.CronSpecDaily = "0 10 * * *" // Default: 10:00 AM daily
    }
    // ...
    ```
    * Add to `.env.example`:
    ```env
    # Cron schedule for 15th of month job (e.g., "0 10 15 * *" for 10:00 AM on 15th)
    CRON_SPEC_15TH="0 10 15 * *"
    # Cron schedule for the daily job that checks if it's the last day of month (e.g., "0 10 * * *" for 10:00 AM daily)
    CRON_SPEC_DAILY_FOR_LAST_DAY_CHECK="0 10 * * *"
    ```

4.  **Integrate Scheduler in `main.go`:**
    * Initialize and start the `NotificationScheduler`. Implement graceful shutdown.
    ```go
    // cmd/bot/main.go
    package main

    import (
        // ... existing imports ...
        "os/signal" // For graceful shutdown
        "syscall"   // For graceful shutdown
        "teacher_notification_bot/internal/infra/scheduler" // Added
    )

    func main() {
        // ... (config loading, db init, repo init, admin service init, bot init) ...
        mainLogger := log.New(os.Stdout, "MAIN: ", log.LstdFlags|log.Lshortfile) // Example logger

        // Initialize (Mock) NotificationService
        mockNotifServiceLogger := log.New(os.Stdout, "MOCK_NOTIF_SVC: ", log.LstdFlags|log.Lshortfile)
        mockNotificationService := app.NewMockNotificationService(mockNotifServiceLogger) // Using the mock
        mainLogger.Println("INFO: Mock Notification service initialized.")


        notificationRepo := idb.NewPostgresNotificationRepository(db) // Assuming idb is internal/infra/database
        mainLogger.Println("INFO: Notification repository initialized.")
        
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

        // ... (handler registration) ...
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
        db.Close() // Close DB connection
        mainLogger.Println("INFO: Application shut down gracefully.")
    }
    ```

---

**Acceptance Criteria:**

* A placeholder `app.NotificationService` interface with an `InitiateNotificationProcess` method is defined. A mock implementation of this service is available for testing the scheduler.
* The `scheduler.NotificationScheduler` struct is implemented in `internal/infra/scheduler/scheduler.go`.
* The scheduler is configured with cron expressions for:
    * The 15th of each month at a specific time (e.g., 10:00 AM)[cite: 56].
    * A daily check at a specific time (e.g., 10:00 AM) which internally verifies if it's the last day of the month before proceeding[cite: 57].
* Configuration for these cron expressions is loaded from `AppConfig` with sensible defaults.
* Upon triggering, each scheduled job function:
    * Logs its execution start.
    * Correctly determines the `cycleDate` (current date) and `cycleType` (`MID_MONTH` or `END_MONTH`).
    * Calls the `NotificationService.InitiateNotificationProcess` method with the correct `cycleType` and `cycleDate`. The `NotificationService` (mocked for now) will be responsible for actual `NotificationCycle` DB record creation.
    * Logs success or any errors from the `InitiateNotificationProcess` call.
* The `NotificationScheduler` is initialized and its `Start()` method is called in `main.go`.
* `main.go` implements graceful shutdown for the scheduler (calling `scheduler.Stop()`) upon receiving SIGINT or SIGTERM signals.

---

**Critical Tests (Manual, Logging-Based for Now):**

1.  **Test 15th of Month Job (Simulated Trigger):**
    * Temporarily modify `CRON_SPEC_15TH` in your `.env` file to a spec that runs in the next 1-2 minutes (e.g., `*/1 * * * *` for every minute, for a very short test).
    * Start the application.
    * **Verify Logs:**
        * Log message indicating "Cron job triggered for 15th of month."
        * Log message from `MockNotificationService` like "InitiateNotificationProcess called for CycleType: MID_MONTH, Date: [current_date]".
2.  **Test Last Day of Month Job (Simulated Trigger on Actual Last Day):**
    * If today *is* the last day of the month: Temporarily modify `CRON_SPEC_DAILY_FOR_LAST_DAY_CHECK` to run in the next 1-2 minutes.
    * If today *is not* the last day: This test requires either waiting or temporarily adjusting your system clock to the last day of a month (use with caution, preferably in a VM or isolated dev environment). Then set the cron spec to run soon.
    * Start the application.
    * **Verify Logs:**
        * Log message "Daily cron job triggered for last day check."
        * Log message "Today is the last day of the month. Executing end-of-month notification process."
        * Log message from `MockNotificationService` like "InitiateNotificationProcess called for CycleType: END_MONTH, Date: [current_date]".
3.  **Test Last Day of Month Job (Non-Trigger Day):**
    * Ensure today is *not* the last day of the month.
    * Temporarily modify `CRON_SPEC_DAILY_FOR_LAST_DAY_CHECK` to run in the next 1-2 minutes.
    * Start the application.
    * **Verify Logs:**
        * Log message "Daily cron job triggered for last day check."
        * Log message similar to "Today (Day X) is not the last day of the month (Day Y). Skipping end-of-month process."
        * The `MockNotificationService.InitiateNotificationProcess` should *not* be called for `END_MONTH`.
4.  **Verify `NotificationCycle` Creation Logic (Conceptual via Logs):**
    * The scheduler's `executeNotificationProcess` logs if it finds an existing cycle or if a new one is implied. This part depends on `NotificationService` eventually creating the cycle. For now, observe the scheduler's log messages about this check. The PRD implies `NotificationService` will handle this "InitiateScheduledNotifications() (called by scheduler)".
5.  **Graceful Shutdown Test:**
    * Start the application. Let the scheduler run (or trigger a job).
    * Press `Ctrl+C` in the terminal where the application is running.
    * **Verify Logs:**
        * Log message "Shutting down application..."
        * Log message "Stopping notification scheduler..."
        * Log message "Notification scheduler gracefully stopped." (or timeout warning).
        * Log message "Application shut down gracefully."

*(Note: The actual creation of `NotificationCycle` database records and further processing logic will be handled by the real `NotificationService` implementation in subsequent tasks. This task focuses on the cron trigger mechanism.)*