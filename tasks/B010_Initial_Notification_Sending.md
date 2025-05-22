## Backend Task: B010 - Implement Initial Notification Sending Logic (First Question)

**Objective:**
To implement the core `NotificationService` that, when triggered (e.g., by the scheduler), will:
1.  Ensure a `NotificationCycle` record exists for the current event (creating one if necessary).
2.  Fetch all active teachers.
3.  For each active teacher, create initial `TeacherReportStatus` records for all reports relevant to the current cycle type.
4.  Send the *first* notification message (regarding "Таблица 1") with "Да"/"Нет" inline buttons to each active teacher.

**Background:**
This task transitions the bot from setup and admin functions to its primary role: proactively engaging teachers. It directly addresses PRD requirements for automatic notification dispatch (FR2.3) and the content/order of the initial questions (FR3.1, FR3.2, FR3.3). The interaction starts with the first question about "Таблица 1".

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Dependencies: `teacher.Repository`, `notification.Repository`, `AppConfig`, Logger

---

**Steps to Completion:**

1.  **Define `TelegramClient` Interface and Adapter:**
    * Create `internal/infra/telegram/client.go`.
    * Define a `Client` interface for abstracting Telegram operations.
    * Implement `TelebotAdapter` that wraps `*telebot.Bot`.
    ```go
    // internal/infra/telegram/client.go
    package telegram

    import "gopkg.in/telebot.v3"

    // Client defines an interface for sending messages via a Telegram bot.
    // This helps in decoupling the application logic from the specific bot library.
    type Client interface {
        SendMessage(recipientChatID int64, text string, options *telebot.SendOptions) error
    }

    // TelebotAdapter implements the Client interface using the gopkg.in/telebot.v3 library.
    type TelebotAdapter struct {
        bot *telebot.Bot
    }

    func NewTelebotAdapter(b *telebot.Bot) *TelebotAdapter {
        return &TelebotAdapter{bot: b}
    }

    // SendMessage sends a text message to the specified recipient.
    func (tba *TelebotAdapter) SendMessage(recipientChatID int64, text string, options *telebot.SendOptions) error {
        recipient := &telebot.User{ID: recipientChatID} // Or Chat if it's a group, but for teachers it's user
        _, err := tba.bot.Send(recipient, text, options)
        return err
    }
    ```

2.  **Implement `NotificationServiceImpl`:**
    * In `internal/app/notification_service.go`, replace the `MockNotificationService` with `NotificationServiceImpl`.
    ```go
    // internal/app/notification_service.go
    package app

    import (
        "context"
        "database/sql"
        "fmt"
        "log"
        "teacher_notification_bot/internal/domain/notification"
        "teacher_notification_bot/internal/domain/teacher"
        idb "teacher_notification_bot/internal/infra/database" // Alias for your DB errors
        "teacher_notification_bot/internal/infra/telegram"   // For TelegramClient
        "time"

        "gopkg.in/telebot.v3" // For telebot.ReplyMarkup and telebot.SendOptions
    )

    // NotificationServiceImpl implements the NotificationService interface.
    type NotificationServiceImpl struct {
        teacherRepo    teacher.Repository
        notifRepo      notification.Repository
        telegramClient telegram.Client // Using the interface
        logger         *log.Logger
    }

    func NewNotificationServiceImpl(
        tr teacher.Repository,
        nr notification.Repository,
        tc telegram.Client,
        logger *log.Logger,
    ) *NotificationServiceImpl {
        return &NotificationServiceImpl{
            teacherRepo:    tr,
            notifRepo:      nr,
            telegramClient: tc,
            logger:         logger,
        }
    }

    // InitiateNotificationProcess starts the notification workflow.
    func (s *NotificationServiceImpl) InitiateNotificationProcess(ctx context.Context, cycleType notification.CycleType, cycleDate time.Time) error {
        s.logger.Printf("INFO: Initiating notification process for CycleType: %s, Date: %s", cycleType, cycleDate.Format("2006-01-02"))

        // 1. Find or Create NotificationCycle
        currentCycle, err := s.notifRepo.GetCycleByDateAndType(ctx, cycleDate, cycleType)
        if err != nil {
            if err == idb.ErrCycleNotFound {
                s.logger.Printf("INFO: No existing cycle for %s on %s. Creating new cycle.", cycleType, cycleDate.Format("2006-01-02"))
                newCycle := &notification.Cycle{
                    CycleDate: cycleDate,
                    Type:      cycleType,
                }
                if err := s.notifRepo.CreateCycle(ctx, newCycle); err != nil {
                    s.logger.Printf("ERROR: Failed to create notification cycle: %v", err)
                    return fmt.Errorf("failed to create notification cycle: %w", err)
                }
                currentCycle = newCycle
                s.logger.Printf("INFO: New notification cycle created with ID: %d", currentCycle.ID)
            } else {
                s.logger.Printf("ERROR: Failed to get notification cycle: %v", err)
                return fmt.Errorf("failed to get notification cycle: %w", err)
            }
        } else {
            s.logger.Printf("INFO: Using existing notification cycle ID: %d for %s on %s", currentCycle.ID, cycleType, cycleDate.Format("2006-01-02"))
        }

        // 2. Fetch Active Teachers
        activeTeachers, err := s.teacherRepo.ListActive(ctx)
        if err != nil {
            s.logger.Printf("ERROR: Failed to list active teachers: %v", err)
            return fmt.Errorf("failed to list active teachers: %w", err)
        }
        if len(activeTeachers) == 0 {
            s.logger.Println("INFO: No active teachers found. Notification process will not send any messages.")
            return nil
        }
        s.logger.Printf("INFO: Found %d active teachers.", len(activeTeachers))

        // 3. Determine Reports for the Cycle
        reportsForCycle := determineReportsForCycle(cycleType)
        if len(reportsForCycle) == 0 {
            s.logger.Printf("WARN: No reports defined for cycle type: %s", cycleType)
            return nil
        }

        // 4. Create Initial TeacherReportStatus Records (Bulk Preferred)
        var statusesToCreate []*notification.ReportStatus
        now := time.Now()
        for _, t := range activeTeachers {
            for _, reportKey := range reportsForCycle {
                // Check if status already exists for this teacher, cycle, reportKey (idempotency)
                _, err := s.notifRepo.GetReportStatus(ctx, t.ID, currentCycle.ID, reportKey)
                if err == nil {
                    s.logger.Printf("INFO: Report status for TeacherID %d, CycleID %d, ReportKey %s already exists. Skipping creation.", t.ID, currentCycle.ID, reportKey)
                    continue // Skip if already exists
                }
                if err != idb.ErrReportStatusNotFound {
                    s.logger.Printf("ERROR: Failed to check existing report status for TeacherID %d, CycleID %d, ReportKey %s: %v", t.ID, currentCycle.ID, reportKey, err)
                    // Decide whether to continue with other statuses or return an error for the whole batch
                    continue // For now, log and continue
                }

                statusesToCreate = append(statusesToCreate, &notification.ReportStatus{
                    TeacherID:        t.ID,
                    CycleID:          currentCycle.ID,
                    ReportKey:        reportKey,
                    Status:           notification.StatusPendingQuestion,
                    LastNotifiedAt:   sql.NullTime{}, // Will be set after successful send
                    ResponseAttempts: 0,
                })
            }
        }

        if len(statusesToCreate) > 0 {
            if err := s.notifRepo.BulkCreateReportStatuses(ctx, statusesToCreate); err != nil {
                s.logger.Printf("ERROR: Failed to bulk create teacher report statuses: %v", err)
                // Depending on error, might need to decide if partial success is okay or rollback
                // For now, we log and proceed to send for successfully created/existing statuses.
            } else {
                s.logger.Printf("INFO: Successfully created/verified %d teacher report statuses.", len(statusesToCreate))
            }
        }


        // 5. Send First Notification (Table 1)
        firstReportKey := notification.ReportKeyTable1Lessons // Always start with Table 1 [cite: 39, 58, 60]
        for _, t := range activeTeachers {
            // Fetch the just created/verified status for the first report to get its ID for callback
            reportStatus, err := s.notifRepo.GetReportStatus(ctx, t.ID, currentCycle.ID, firstReportKey)
            if err != nil {
                s.logger.Printf("ERROR: Could not fetch report status for TeacherID %d, ReportKey %s for sending initial notification: %v", t.ID, firstReportKey, err)
                continue // Skip this teacher if their initial status record is missing
            }
            
            // Only send if status is PENDING_QUESTION and not recently notified (e.g. if process restarts)
            // For this initial send, LastNotifiedAt should be null or very old.
            if reportStatus.Status != notification.StatusPendingQuestion {
                 s.logger.Printf("INFO: Initial notification for TeacherID %d, ReportKey %s skipped, status is %s.", t.ID, firstReportKey, reportStatus.Status)
                 continue
            }


            teacherName := t.FirstName // PRD Example: "Привет, [Имя Преподавателя]!" [cite: 39]
            messageText := fmt.Sprintf("Привет, %s! Заполнена ли Таблица 1: Проведенные уроки (отчёт за текущий период)?", teacherName) // [cite: 39]

            // Define callback data. Needs to be unique and identifiable.
            // Example: "ans:<ReportStatusID>:<response>" -> "ans:123:yes"
            // FR3.3: inline-кнопки "Да" и "Нет"
            replyMarkup := &telebot.ReplyMarkup{ResizeKeyboard: true} // Inline keyboard
            btnYes := replyMarkup.Data("Да", fmt.Sprintf("ans_yes_%d", reportStatus.ID))
            btnNo := replyMarkup.Data("Нет", fmt.Sprintf("ans_no_%d", reportStatus.ID))
            replyMarkup.Inline(replyMarkup.Row(btnYes, btnNo))

            err = s.telegramClient.SendMessage(t.TelegramID, messageText, &telebot.SendOptions{ReplyMarkup: replyMarkup, ParseMode: telebot.ModeDefault})
            if err != nil {
                s.logger.Printf("ERROR: Failed to send initial notification for Table 1 to Teacher %s (TG_ID: %d): %v", teacherName, t.TelegramID, err)
                // Optionally, update reportStatus to reflect send failure if necessary
            } else {
                s.logger.Printf("INFO: Successfully sent initial notification for Table 1 to Teacher %s (TG_ID: %d)", teacherName, t.TelegramID)
                // Update LastNotifiedAt for this specific status
                reportStatus.LastNotifiedAt = sql.NullTime{Time: now, Valid: true}
                if errUpdate := s.notifRepo.UpdateReportStatus(ctx, reportStatus); errUpdate != nil {
                    s.logger.Printf("ERROR: Failed to update LastNotifiedAt for TeacherID %d, ReportStatusID %d: %v", t.ID, reportStatus.ID, errUpdate)
                }
            }
        }
        return nil
    }

    func determineReportsForCycle(cycleType notification.CycleType) []notification.ReportKey {
        switch cycleType {
        case notification.CycleTypeMidMonth:
            return []notification.ReportKey{ // [cite: 15, 58, 59]
                notification.ReportKeyTable1Lessons,
                notification.ReportKeyTable3Schedule,
            }
        case notification.CycleTypeEndMonth:
            return []notification.ReportKey{ // [cite: 16, 60, 61, 62]
                notification.ReportKeyTable1Lessons,
                notification.ReportKeyTable3Schedule,
                notification.ReportKeyTable2OTV,
            }
        default:
            return []notification.ReportKey{}
        }
    }
    ```

3.  **Update `main.go` for Real `NotificationServiceImpl` and `TelebotAdapter`:**
    * Modify `cmd/bot/main.go` to initialize and use the actual service and adapter.
    ```go
    // cmd/bot/main.go
    // ... (imports) ...

    func main() {
        // ... (config loading, logger setup) ...
        mainLogger := log.New(os.Stdout, "MAIN: ", log.LstdFlags|log.Lshortfile)

        db, err := idb.NewPostgresConnection(cfg.DatabaseURL)
        // ... (error handling, defer db.Close()) ...

        teacherRepo := idb.NewPostgresTeacherRepository(db)
        notificationRepo := idb.NewPostgresNotificationRepository(db)
        adminService := app.NewAdminService(teacherRepo, cfg.AdminTelegramID)
        
        // Initialize Telegram Bot
        pref := telebot.Settings{
            Token: cfg.TelegramToken,
            Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
            OnError: func(err error, c telebot.Context) {
                mainLogger.Printf("ERROR (telebot_global): %v", err)
                // ... (more context if c is not nil)
            },
        }
        bot, err := telebot.NewBot(pref)
        if err != nil {
            mainLogger.Fatalf("FATAL: Could not create Telegram bot: %v", err)
        }

        // Create TelebotAdapter
        telegramClientAdapter := telegram.NewTelebotAdapter(bot)
        mainLogger.Println("INFO: Telegram client adapter initialized.")

        // Initialize REAL NotificationService
        notifServiceLogger := log.New(os.Stdout, "NOTIF_SVC: ", log.LstdFlags|log.Lshortfile)
        notificationService := app.NewNotificationServiceImpl(
            teacherRepo,
            notificationRepo,
            telegramClientAdapter, // Pass the adapter
            notifServiceLogger,
        )
        mainLogger.Println("INFO: Notification service initialized.")

        // Initialize NotificationScheduler
        schedulerLogger := log.New(os.Stdout, "SCHEDULER: ", log.LstdFlags|log.Lshortfile)
        notifScheduler := scheduler.NewNotificationScheduler(
            notificationService, // Pass the REAL service
            notificationRepo,    // Scheduler might still need repo for cycle checks or direct creation
            schedulerLogger,
            cfg.CronSpec15th,
            cfg.CronSpecDaily,
        )
        // ... (rest of main: handler registration, scheduler.Start(), bot.Start(), graceful shutdown)
        telegram.RegisterAdminHandlers(bot, adminService, cfg.AdminTelegramID)
        // TODO: Register handlers for teacher responses (callbacks) in a later task
        mainLogger.Println("INFO: Admin command handlers registered.")

        notifScheduler.Start()

        mainLogger.Println("INFO: Application setup complete. Bot and Scheduler are starting...")
        go bot.Start()

        quit := make(chan os.Signal, 1)
        signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
        <-quit

        mainLogger.Println("INFO: Shutting down application...")
        notifScheduler.Stop()
        db.Close()
        mainLogger.Println("INFO: Application shut down gracefully.")
    }
    ```

---

**Acceptance Criteria:**

* The `telegram.Client` interface and `telegram.TelebotAdapter` are implemented in `internal/infra/telegram/client.go`.
* The `app.NotificationServiceImpl` (real implementation) replaces the mock service.
* When `NotificationServiceImpl.InitiateNotificationProcess` is invoked (e.g., by the scheduler or a manual test call):
    * A `NotificationCycle` DB record is successfully created (or an existing one for the specific `cycleDate` and `cycleType` is found and used).
    * All currently active teachers are fetched from the `teachers` table via `TeacherRepository`.
    * For each active teacher, `TeacherReportStatus` DB records are created for all report keys relevant to the `cycleType` (e.g., Table1 & Table3 for Mid-Month; Table1, Table3, & Table2 for End-Month [cite: 15, 16, 58, 59, 60, 61, 62]). These initial statuses are set to `notification.StatusPendingQuestion`. Idempotency is handled (statuses are not duplicated if they already exist for that teacher/cycle/report).
    * Each active teacher receives a Telegram message specifically for the *first report* (`notification.ReportKeyTable1Lessons`).
    * The message content is "Привет, [TeacherFirstName]! Заполнена ли Таблица 1: Проведенные уроки (отчёт за текущий период)?"[cite: 39].
    * The message includes an inline keyboard with "Да" and "Нет" buttons[cite: 17, 63]. The callback data for these buttons should uniquely identify the `TeacherReportStatus.ID` (e.g., `ans_yes_<status_id>`).
    * After successfully sending the message for Table 1, the `LastNotifiedAt` field for that specific `TeacherReportStatus` record is updated to the current time.
    * All operations (DB interactions, Telegram sends) are logged appropriately, including errors.
* The real `NotificationServiceImpl` and `TelebotAdapter` are correctly initialized and wired in `main.go`.

---

**Critical Tests (Manual & Logging Based):**

1.  **Mid-Month Notification Trigger (Simulated):**
    * **Setup:** Ensure at least 2-3 active teachers exist in the DB. Ensure no `notification_cycles` or `teacher_report_statuses` exist for today's date and `MID_MONTH` type.
    * **Action:** Manually trigger `NotificationServiceImpl.InitiateNotificationProcess` with `cycleType = notification.CycleTypeMidMonth` and `cycleDate = time.Now()` (e.g., by adding a temporary call in `main.go` after all initializations or by adjusting the scheduler for an immediate run).
    * **Verify Logs & DB:**
        * Log: Creation of a new `NotificationCycle` for today, type `MID_MONTH`.
        * Log: Fetching of active teachers.
        * Log: Creation of `TeacherReportStatus` records. For each active teacher, there should be statuses for `TABLE_1_LESSONS` and `TABLE_3_SCHEDULE`, all set to `PENDING_QUESTION`.
        * DB: Confirm the `notification_cycles` record. Confirm `teacher_report_statuses` records (2 per active teacher for mid-month).
        * Log: Sending of "Таблица 1" message to each active teacher.
        * DB: `LastNotifiedAt` for the `TABLE_1_LESSONS` status for each teacher should be updated.
    * **Verify Telegram:** Each active teacher's Telegram account should receive the "Привет, [Имя]! Заполнена ли Таблица 1: Проведенные уроки (отчёт за текущий период)?" message with "Да"/"Нет" buttons.
2.  **End-Month Notification Trigger (Simulated - First Question Only):**
    * **Setup:** Similar to Test 1, but clear previous cycle/status data or use a different date.
    * **Action:** Trigger `InitiateNotificationProcess` with `cycleType = notification.CycleTypeEndMonth`.
    * **Verify Logs & DB:**
        * Log: Creation of `NotificationCycle` (type `END_MONTH`).
        * Log: `TeacherReportStatus` creation for `TABLE_1_LESSONS`, `TABLE_3_SCHEDULE`, AND `TABLE_2_OTV` for each active teacher[cite: 16, 60, 61, 62].
        * DB: Confirm these statuses.
        * Log: Sending of "Таблица 1" message.
        * DB: `LastNotifiedAt` updated for `TABLE_1_LESSONS` status.
    * **Verify Telegram:** Same "Таблица 1" message as in Test 1.
3.  **Idempotency Test (Cycle & Status Creation):**
    * Run Test 1 again without clearing the DB.
    * **Verify Logs & DB:**
        * Log: Should indicate that the `NotificationCycle` for today/`MID_MONTH` *already exists* and is being used. No new cycle created.
        * Log: Should indicate that `TeacherReportStatus` records *already exist* for the teachers and relevant reports; no new status records created.
        * Log & Telegram: Depending on the logic for re-sending (current logic checks `StatusPendingQuestion`), messages for Table 1 might be re-sent if they were never answered. If already answered/processed, they should not be re-sent.
4.  **No Active Teachers Scenario:**
    * **Setup:** Deactivate all teachers (set `is_active = false` in the `teachers` table).
    * **Action:** Trigger `InitiateNotificationProcess`.
    * **Verify Logs & DB:**
        * Log: `NotificationCycle` may still be created (as it's a system event).
        * Log: "No active teachers found."
        * DB: No new `teacher_report_statuses` should be created.
        * Verify Telegram: No messages sent.
5.  **Error Handling Simulation (Conceptual - difficult to fully automate manually here):**
    * **DB Error:** If possible, temporarily make the DB unavailable during a part of the process (e.g., during `CreateReportStatus`). Observe logs for error messages.
    * **Telegram Send Error:** Use an invalid (but numerically valid) `TelegramID` for a test teacher. Observe logs for errors from `telegramClient.SendMessage`. The application should continue processing other teachers if possible.