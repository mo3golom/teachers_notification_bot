## Backend Task: B012 - Implement Handling of Teacher's "No" Response and 1-Hour Reminder

**Objective:**
To implement the Telegram callback query handler for when a teacher clicks the "Нет" (No) button. This action will update the `TeacherReportStatus` to indicate a "No" answer, schedule a reminder for 1 hour later by setting a `remind_at` timestamp, and inform the teacher about this upcoming reminder. Additionally, a recurring background job will be set up to process these scheduled 1-hour reminders by re-sending the original question.

**Background:**
Teachers may not always be ready to confirm table completion upon the first query. The PRD outlines a workflow (FR4.2[cite: 66, 67], UC2 [cite: 45, 46]) where a "No" response triggers a 1-hour follow-up. This task implements that initial "No" handling and the first type of automated reminder.

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Cron Library: `github.com/robfig/cron/v3`
* Dependencies: `teacher.Repository`, `notification.Repository`, `AppConfig`, `NotificationService`, `TelegramClient`, Logger

---

**Steps to Completion:**

1.  **Database Migration for `remind_at`:**
    * Generate new migration files:
        ```bash
        migrate create -ext sql -dir migrations add_remind_at_to_teacher_report_statuses
        ```
    * **UP Migration (`..._add_remind_at_to_teacher_report_statuses.up.sql`):**
        ```sql
        ALTER TABLE teacher_report_statuses
        ADD COLUMN remind_at TIMESTAMPTZ DEFAULT NULL;
        ```
    * **DOWN Migration (`..._add_remind_at_to_teacher_report_statuses.down.sql`):**
        ```sql
        ALTER TABLE teacher_report_statuses
        DROP COLUMN remind_at;
        ```
    * Apply the migration: `migrate -database "$DATABASE_URL" -path migrations up`.

2.  **Update `TeacherReportStatus` Entity:**
    * In `internal/domain/notification/status.go`, add `RemindAt` to the `ReportStatus` struct:
    ```go
    // internal/domain/notification/status.go
    // ...
    type ReportStatus struct {
        // ... existing fields
        RemindAt         sql.NullTime      // When a reminder should be sent
        // ... CreatedAt, UpdatedAt
    }
    ```
    * Update repository methods (`CreateReportStatus`, `UpdateReportStatus`, `GetReportStatus`, `GetReportStatusByID`, `scanReportStatuses` helper) in `postgres_notification_repository.go` to include scanning and potentially inserting/updating the `remind_at` field.
        * For `CreateReportStatus` (and `BulkCreate`): `remind_at` will typically be `NULL` initially.
        * `UpdateReportStatus` query should be able to set `remind_at`. Example:
        ```sql
        // In UpdateReportStatus in postgres_notification_repository.go
        // query := `UPDATE teacher_report_statuses
        //            SET status = $1, last_notified_at = $2, response_attempts = $3, updated_at = NOW(), remind_at = $4
        //            WHERE id = $5
        //            RETURNING updated_at`
        // err := r.db.QueryRowContext(ctx, query, rs.Status, rs.LastNotifiedAt, rs.ResponseAttempts, rs.RemindAt, rs.ID).Scan(&rs.UpdatedAt)
        ```
        * Ensure scan functions correctly handle `rs.RemindAt`.

3.  **Enhance `NotificationServiceImpl` for "No" Response:**
    * In `internal/app/notification_service.go`, add `ProcessTeacherNoResponse`.
    ```go
    // internal/app/notification_service.go
    // ... (existing imports and NotificationServiceImpl struct) ...

    func (s *NotificationServiceImpl) ProcessTeacherNoResponse(ctx context.Context, reportStatusID int64) error {
        s.logger.Printf("INFO: Processing 'No' response for ReportStatusID: %d", reportStatusID)

        currentReportStatus, err := s.notifRepo.GetReportStatusByID(ctx, reportStatusID)
        if err != nil {
            if err == idb.ErrReportStatusNotFound {
                s.logger.Printf("WARN: ReportStatusID %d not found for 'No' response. Stale callback?", reportStatusID)
                return nil // Acknowledge, nothing to do
            }
            s.logger.Printf("ERROR: Failed to get report status by ID %d for 'No' response: %v", reportStatusID, err)
            return fmt.Errorf("failed to get report status %d: %w", reportStatusID, err)
        }

        // Avoid reprocessing if already handled or moved to a later reminder state
        if currentReportStatus.Status == notification.StatusAnsweredNo ||
           currentReportStatus.Status == notification.StatusAwaitingReminder1H ||
           currentReportStatus.Status == notification.StatusAwaitingReminderNextDay {
            s.logger.Printf("INFO: ReportStatusID %d already processed for 'No' or awaiting reminder. Status: %s", reportStatusID, currentReportStatus.Status)
            // Optionally send a less active message like "Reminder is already scheduled."
            // For now, just acknowledge.
            return nil
        }
        
        currentReportStatus.Status = notification.StatusAnsweredNo
        currentReportStatus.ResponseAttempts++
        currentReportStatus.RemindAt = sql.NullTime{Time: time.Now().Add(1 * time.Hour), Valid: true}
        currentReportStatus.UpdatedAt = time.Now()

        if err := s.notifRepo.UpdateReportStatus(ctx, currentReportStatus); err != nil {
            s.logger.Printf("ERROR: Failed to update report status ID %d for 'No' response: %v", reportStatusID, err)
            return fmt.Errorf("failed to update report status %d: %w", reportStatusID, err)
        }
        s.logger.Printf("INFO: ReportStatusID %d updated to %s, RemindAt set to %s.", reportStatusID, currentReportStatus.Status, currentReportStatus.RemindAt.Time.Format(time.RFC3339))

        // Fetch teacher info to get TelegramID for sending the confirmation.
        teacherInfo, err := s.teacherRepo.GetByID(ctx, currentReportStatus.TeacherID)
        if err != nil {
            s.logger.Printf("ERROR: Failed to get teacher (ID %d) for sending 'No' confirmation: %v", currentReportStatus.TeacherID, err)
            // Log error but don't let it fail the whole operation if status was updated.
        } else {
            ackMessage := "Понял(а). Напомню через час." // [cite: 45]
            errSend := s.telegramClient.SendMessage(teacherInfo.TelegramID, ackMessage, nil)
            if errSend != nil {
                s.logger.Printf("ERROR: Failed to send 'No' response ack to TeacherID %d (TG_ID %d): %v", teacherInfo.ID, teacherInfo.TelegramID, errSend)
            } else {
                s.logger.Printf("INFO: 'No' response ack sent to TeacherID %d (TG_ID %d).", teacherInfo.ID, teacherInfo.TelegramID)
            }
        }
        return nil
    }
    ```

4.  **Implement Telegram Callback Handler for "Нет":**
    * In `internal/infra/telegram/teacher_response_handlers.go`, update the handler for callbacks starting with `ans_no_`.
    ```go
    // internal/infra/telegram/teacher_response_handlers.go
    // ... (within RegisterTeacherResponseHandlers, inside the general OnCallback handler) ...
            // else if strings.HasPrefix(data, "ans_no_") { // From B011
            //     parts := strings.Split(data, "_") // ans_no_123
            //     if len(parts) != 3 {
            //         c.Bot().OnError(fmt.Errorf("invalid callback data format for 'no': %s", data), c)
            //         return c.Respond(&telebot.CallbackResponse{Text: "Ошибка обработки ответа."})
            //     }
            //     reportStatusIDStr := parts[2]
            //     reportStatusID, err := strconv.ParseInt(reportStatusIDStr, 10, 64)
            //     if err != nil {
            //         c.Bot().OnError(fmt.Errorf("invalid reportStatusID '%s' in 'no' callback: %w", reportStatusIDStr, err), c)
            //         return c.Respond(&telebot.CallbackResponse{Text: "Ошибка ID отчета."})
            //     }
            //
            //     err = notificationService.ProcessTeacherNoResponse(c.Request().Context(), reportStatusID)
            //     if err != nil {
            //         c.Bot().OnError(fmt.Errorf("error processing 'No' response for statusID %d: %w", reportStatusID, err), c)
            //         return c.Respond(&telebot.CallbackResponse{Text: "Произошла ошибка."})
            //     }
            //     // The service sends the textual "Понял(а)..." message.
            //     // Callback an ack to remove the "processing" state on the button.
            //     return c.Respond() // Minimal ack, or use text "Напоминание установлено."
            // }
    ```
    *The above logic should be integrated into the `telebot.OnCallback` handler from B011.*

5.  **Update `NotificationRepository` for Listing Due 1-Hour Reminders:**
    * Add to `internal/domain/notification/repository.go` interface:
    ```go
    // internal/domain/notification/repository.go
    // ...
    type Repository interface {
        // ... existing methods
        ListDueReminders(ctx context.Context, targetStatus InteractionStatus, remindAtOrBefore time.Time) ([]*ReportStatus, error)
    }
    ```
    * Implement in `internal/infra/database/postgres_notification_repository.go`:
    ```go
    // internal/infra/database/postgres_notification_repository.go
    // ...
    func (r *PostgresNotificationRepository) ListDueReminders(ctx context.Context, targetStatus notification.InteractionStatus, remindAtOrBefore time.Time) ([]*notification.ReportStatus, error) {
        query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at, remind_at
                   FROM teacher_report_statuses
                   WHERE status = $1 AND remind_at IS NOT NULL AND remind_at <= $2
                   ORDER BY remind_at ASC` // Process older ones first
        rows, err := r.db.QueryContext(ctx, query, targetStatus, remindAtOrBefore)
        if err != nil {
            return nil, fmt.Errorf("error querying for due reminders (status: %s): %w", targetStatus, err)
        }
        defer rows.Close()
        // Use the scanReportStatuses helper, ensuring it handles the new remind_at field
        return scanReportStatuses(rows) // Make sure scanReportStatuses is updated for remind_at
    }
    ```
    * **Important:** Ensure `scanReportStatuses` in `postgres_notification_repository.go` is updated to scan the `remind_at` field.
    ```go
    // In scanReportStatuses:
    // if err := rows.Scan(..., &rs.RemindAt); err != nil { ... }
    ```

6.  **Enhance `NotificationServiceImpl` for Processing 1-Hour Reminders:**
    * Add `ProcessScheduled1HourReminders` method to `internal/app/notification_service.go`.
    ```go
    // internal/app/notification_service.go
    // ...
    func (s *NotificationServiceImpl) ProcessScheduled1HourReminders(ctx context.Context) error {
        s.logger.Println("INFO: Processing scheduled 1-hour reminders...")
        now := time.Now()
        
        // Fetch statuses that were 'ANSWERED_NO' and are due for a 1-hour reminder
        dueStatuses, err := s.notifRepo.ListDueReminders(ctx, notification.StatusAnsweredNo, now)
        if err != nil {
            s.logger.Printf("ERROR: Failed to list due 1-hour reminders: %v", err)
            return fmt.Errorf("failed to list due 1-hour reminders: %w", err)
        }

        if len(dueStatuses) == 0 {
            s.logger.Println("INFO: No 1-hour reminders currently due.")
            return nil
        }
        s.logger.Printf("INFO: Found %d status(es) due for 1-hour reminder.", len(dueStatuses))

        for _, rs := range dueStatuses {
            s.logger.Printf("INFO: Processing 1-hour reminder for ReportStatusID: %d (TeacherID: %d, ReportKey: %s)", rs.ID, rs.TeacherID, rs.ReportKey)

            teacherInfo, err := s.teacherRepo.GetByID(ctx, rs.TeacherID)
            if err != nil {
                s.logger.Printf("ERROR: Failed to get teacher (ID %d) for 1-hour reminder: %v", rs.TeacherID, err)
                continue // Skip this reminder
            }

            // Re-send the specific question for rs.ReportKey
            // The sendSpecificReportQuestion method already handles logging and updating LastNotifiedAt
            // We need to ensure it also sets the status appropriately for a reminder.
            // Let's update sendSpecificReportQuestion to accept the target status after sending.
            // Or, update status here BEFORE sending.

            rs.Status = notification.StatusAwaitingReminder1H // Update status to indicate reminder has been processed
            rs.RemindAt = sql.NullTime{Valid: false}         // Clear RemindAt as this slot is used
            // LastNotifiedAt will be set by sendSpecificReportQuestion
            // ResponseAttempts is not incremented for sending a reminder itself, only for "No" answers.

            // Call helper to send the question. This helper should set LastNotifiedAt.
            // It also sets status to PENDING_QUESTION for the teacher's view, but our rs.Status for tracking is AWAITING_REMINDER_1H.
            // This needs careful state distinction.
            // Let's refine: sendSpecificReportQuestion is about *asking*. The state *before* asking
            // and *after* successfully asking needs to be managed here.

            if err := s.sendSpecificReportQuestion(ctx, teacherInfo, rs.CycleID, rs.ReportKey); err != nil {
                s.logger.Printf("ERROR: Failed to send 1-hour reminder for ReportStatusID %d: %v", rs.ID, err)
                // If send fails, should we revert status or keep RemindAt?
                // For now, we attempted, so we'll update the status to reflect attempt.
                // The sendSpecificReportQuestion already updates LastNotifiedAt on success inside it.
            } else {
                 // If successfully sent, now update the status to AWAITING_REMINDER_1H and clear RemindAt
                if errUpdate := s.notifRepo.UpdateReportStatus(ctx, rs); errUpdate != nil {
                     s.logger.Printf("ERROR: Failed to update ReportStatusID %d after sending 1-hour reminder: %v", rs.ID, errUpdate)
                } else {
                     s.logger.Printf("INFO: Successfully sent and updated status for 1-hour reminder for ReportStatusID: %d", rs.ID)
                }
            }
        }
        return nil
    }
    ```

7.  **Add Cron Job for 1-Hour Reminder Processing:**
    * In `internal/infra/scheduler/scheduler.go`, add a new cron job to `NotificationScheduler.Start()`.
    * Add a new config `CronSpecReminderCheck` to `AppConfig` and `.env.example` (e.g., `*/5 * * * *` for every 5 minutes).
    ```go
    // internal/infra/scheduler/scheduler.go
    // In NewNotificationScheduler, add cronSpecReminderCheck string argument
    // In NotificationScheduler struct, add field: cronSpecReminderCheck string
    // In NotificationScheduler.Start():
    // ...
        _, err = s.cronEngine.AddFunc(s.cronSpecReminderCheck, func() {
            s.logger.Println("INFO: Cron job triggered for processing 1-hour reminders.")
            ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute) // Context for the job
            defer cancel()
            if err := s.notifService.ProcessScheduled1HourReminders(ctx); err != nil {
                s.logger.Printf("ERROR: Error during 1-hour reminder processing: %v", err)
            }
        })
        if err != nil {
            s.logger.Fatalf("FATAL: Could not add 1-hour reminder processing cron job: %v", err)
        }
    // ...
    ```
    * Update `AppConfig` and `NewNotificationScheduler` in `main.go` to pass this new cron spec.

---

**Acceptance Criteria:**

* The `teacher_report_statuses` table is successfully migrated to include the `remind_at TIMESTAMPTZ NULL` column. The `ReportStatus` Go struct and repository methods are updated accordingly.
* `NotificationServiceImpl.ProcessTeacherNoResponse` is implemented:
    * It correctly updates the `TeacherReportStatus` to `status = StatusAnsweredNo`.
    * It increments `response_attempts` for that status.
    * It sets `remind_at` to approximately 1 hour in the future from the current time.
    * It sends the message "Понял(а). Напомню через час." to the teacher who clicked "Нет"[cite: 45].
* The Telegram callback handler for `ans_no_<status_id>` correctly invokes `ProcessTeacherNoResponse` and acknowledges the callback.
* `NotificationRepository` includes a `ListDueReminders` method that correctly fetches `ReportStatus` records where `status = StatusAnsweredNo`, `remind_at` is not null, and `remind_at` is less than or equal to the current time.
* `NotificationServiceImpl.ProcessScheduled1HourReminders` is implemented:
    * It calls `ListDueReminders` to get pending 1-hour reminders.
    * For each due reminder, it re-sends the original question for that `reportKey` to the correct teacher (using the existing `sendSpecificReportQuestion` helper).
    * After attempting to send the reminder, the `TeacherReportStatus` is updated: `status` is changed to `StatusAwaitingReminder1H`, `remind_at` is cleared (set to NULL), and `last_notified_at` is updated to the current time.
* A new cron job is added to `NotificationScheduler` (or a similar scheduler component) that runs at a frequent interval (e.g., every 5-10 minutes).
* This cron job successfully calls `NotificationService.ProcessScheduled1HourReminders`.
* Configuration for the reminder check cron spec is added to `AppConfig` and `.env.example`.

---

**Critical Tests (Manual & Logging Based):**

1.  **Teacher Clicks "Нет" Button:**
    * **Setup:** A teacher has received an initial notification question (e.g., for "Таблица 1" as per B010).
    * **Action:** The teacher clicks the "Нет" button on that message.
    * **Verify:**
        * **DB:** The corresponding `teacher_report_statuses` record for "Таблица 1" should have its `status` updated to `StatusAnsweredNo`, `response_attempts` incremented by 1, and `remind_at` set to a timestamp approximately 1 hour from now.
        * **Telegram (Teacher):** The teacher receives the message "Понял(а). Напомню через час."[cite: 45].
        * **Logs:** Confirm the `ProcessTeacherNoResponse` method was executed and logged its actions.
2.  **1-Hour Reminder Processing and Delivery:**
    * **Setup:** Complete Test 1. Wait until the `remind_at` timestamp is in the past (or adjust system time for testing). Ensure the reminder processing cron job is configured to run.
    * **Action:** Let the reminder processing cron job execute.
    * **Verify:**
        * **Logs:** The cron job for reminders should log its execution. The `ProcessScheduled1HourReminders` method should log that it found the due status.
        * **Telegram (Teacher):** The teacher should receive the *exact same question again* for "Таблица 1", with fresh "Да"/"Нет" buttons.
        * **DB:** The `teacher_report_statuses` record for "Таблица 1" should now have `status = StatusAwaitingReminder1H`, `remind_at` should be `NULL`, and `last_notified_at` updated to the current time.
3.  **No Reminder Sent if Status Changed Before Reminder Time:**
    * **Setup:** Complete Test 1 (teacher clicked "Нет", `remind_at` is set).
    * **Action (Before Reminder Time):** Simulate the teacher clicking "Да" to the *original* question (if buttons are still active or message re-sent some other way), and this "Да" response is processed (Task B011), changing the status to `StatusAnsweredYes`.
    * **Action (At Reminder Time):** Let the reminder processing cron job execute.
    * **Verify:**
        * **Logs:** The reminder job runs. `ListDueReminders` should *not* return this status (as its `status` is no longer `StatusAnsweredNo`).
        * **Telegram (Teacher):** The teacher should *not* receive the 1-hour reminder for "Таблица 1".
4.  **Multiple Due Reminders Processed:**
    * **Setup:** Have two different `TeacherReportStatus` records (for different teachers or different reports if applicable) that are both `StatusAnsweredNo` and have `remind_at` timestamps in the past.
    * **Action:** Let the reminder processing cron job execute.
    * **Verify:** Both reminders should be processed, messages sent to respective teachers, and their statuses updated.
5.  **Error Handling for "Нет" Callback (Invalid ID):**
    * **Action:** Simulate a callback to the "Нет" handler with an invalid or non-existent `reportStatusID`.
    * **Verify:** The system logs an error. The callback is acknowledged to Telegram without crashing the bot. The teacher receives a generic acknowledgement or no error message.