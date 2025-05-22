## Backend Task: B013 - Implement Next-Day Reminder Logic

**Objective:**
To implement the functionality for sending a single follow-up reminder on the "next day" if a teacher has not responded ("Да" or "Нет") to an initial notification or to a 1-hour reminder that was sent on the previous day. This involves a daily scheduled job to identify such "stalled" statuses and trigger the re-sending of the relevant question.

**Background:**
The PRD specifies a final reminder attempt (FR4.3[cite: 68], part of UC2 [cite: 18]) to maximize the chances of getting a response from teachers who might have missed earlier notifications. This next-day reminder is designed as a single, non-spammy follow-up.

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Cron Library: `github.com/robfig/cron/v3`
* Dependencies: `teacher.Repository`, `notification.Repository`, `AppConfig`, `NotificationService`, `TelegramClient`, Logger

---

**Steps to Completion:**

1.  **Define New `InteractionStatus` Constant:**
    * In `internal/domain/notification/shared_types.go`, add a new status:
    ```go
    // internal/domain/notification/shared_types.go
    // ...
    const (
        // ... existing statuses
        StatusNextDayReminderSent InteractionStatus = "NEXT_DAY_REMINDER_SENT"
    )
    ```

2.  **Update `NotificationRepository` Interface and Implementation:**
    * **Interface (`internal/domain/notification/repository.go`):** Add `ListStalledStatusesFromPreviousDay`.
    ```go
    // internal/domain/notification/repository.go
    type Repository interface {
        // ... existing methods
        ListStalledStatusesFromPreviousDay(ctx context.Context, statusesToConsider []InteractionStatus, startOfPreviousDay, endOfPreviousDay time.Time) ([]*ReportStatus, error)
    }
    ```
    * **Implementation (`internal/infra/database/postgres_notification_repository.go`):** Implement the new method.
    ```go
    // internal/infra/database/postgres_notification_repository.go
    // ...
    func (r *PostgresNotificationRepository) ListStalledStatusesFromPreviousDay(
        ctx context.Context,
        statusesToConsider []notification.InteractionStatus,
        startOfPreviousDay time.Time, // e.g., Yesterday 00:00:00
        endOfPreviousDay time.Time,     // e.g., Yesterday 23:59:59.999999
    ) ([]*notification.ReportStatus, error) {
        if len(statusesToConsider) == 0 {
            return []*notification.ReportStatus{}, nil
        }

        // Convert InteractionStatus slice to string slice for pq.Array
        statusStrings := make([]string, len(statusesToConsider))
        for i, s := range statusesToConsider {
            statusStrings[i] = string(s)
        }

        // Query for statuses that were last notified within the previous day's timeframe
        // and are still in one of the statusesToConsider.
        // This assumes last_notified_at is reliably updated upon each notification attempt.
        query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at, remind_at
                   FROM teacher_report_statuses
                   WHERE last_notified_at >= $1 AND last_notified_at <= $2
                     AND status = ANY($3::varchar[])
                   ORDER BY last_notified_at ASC`

        rows, err := r.db.QueryContext(ctx, query, startOfPreviousDay, endOfPreviousDay, pq.Array(statusStrings))
        if err != nil {
            return nil, fmt.Errorf("error querying for stalled statuses from previous day: %w", err)
        }
        defer rows.Close()
        // Ensure scanReportStatuses handles all fields, including remind_at
        return scanReportStatuses(rows)
    }
    ```

3.  **Enhance `NotificationServiceImpl` for Processing Next-Day Reminders:**
    * In `internal/app/notification_service.go`, add the `ProcessNextDayReminders` method.
    ```go
    // internal/app/notification_service.go
    // ...
    func (s *NotificationServiceImpl) ProcessNextDayReminders(ctx context.Context) error {
        s.logger.Println("INFO: Processing scheduled next-day reminders...")
        
        now := time.Now()
        // Define "previous day" range precisely, considering server's local timezone for consistency with cron.
        // For simplicity, using simple date arithmetic. Be mindful of timezones in production.
        // Location should match the cron job's location.
        loc := time.Local // Or a specific configured timezone
        year, month, day := now.Date()
        startOfToday := time.Date(year, month, day, 0, 0, 0, 0, loc)
        endOfPreviousDay := startOfToday.Add(-1 * time.Nanosecond) // Yesterday 23:59:59.999...
        startOfPreviousDay := startOfToday.AddDate(0, 0, -1)   // Yesterday 00:00:00

        s.logger.Printf("INFO: Checking for stalled statuses between %s and %s", startOfPreviousDay.Format(time.RFC3339), endOfPreviousDay.Format(time.RFC3339))

        statusesToConsider := []notification.InteractionStatus{
            notification.StatusPendingQuestion,
            notification.StatusAwaitingReminder1H,
        }

        stalledStatuses, err := s.notifRepo.ListStalledStatusesFromPreviousDay(ctx, statusesToConsider, startOfPreviousDay, endOfPreviousDay)
        if err != nil {
            s.logger.Printf("ERROR: Failed to list stalled statuses for next-day reminder: %v", err)
            return fmt.Errorf("failed to list stalled statuses: %w", err)
        }

        if len(stalledStatuses) == 0 {
            s.logger.Println("INFO: No statuses found needing a next-day reminder.")
            return nil
        }
        s.logger.Printf("INFO: Found %d status(es) needing a next-day reminder.", len(stalledStatuses))

        for _, rs := range stalledStatuses {
            s.logger.Printf("INFO: Processing next-day reminder for ReportStatusID: %d (TeacherID: %d, ReportKey: %s, CurrentStatus: %s)", rs.ID, rs.TeacherID, rs.ReportKey, rs.Status)

            teacherInfo, err := s.teacherRepo.GetByID(ctx, rs.TeacherID)
            if err != nil {
                s.logger.Printf("ERROR: Failed to get teacher (ID %d) for next-day reminder: %v", rs.TeacherID, err)
                continue // Skip this reminder
            }
            
            // Update status before sending to prevent re-processing if send fails temporarily
            // but ensure we record the attempt.
            originalStatusBeforeReminder := rs.Status // For logging or conditional logic if needed
            rs.Status = notification.StatusNextDayReminderSent
            rs.ResponseAttempts++ // This is another attempt to get a response
            // LastNotifiedAt will be set by sendSpecificReportQuestion or explicitly after send
            rs.RemindAt = sql.NullTime{Valid: false} // Clear any previous reminder time

            // Re-send the specific question
            if err := s.sendSpecificReportQuestion(ctx, teacherInfo, rs.CycleID, rs.ReportKey); err != nil {
                s.logger.Printf("ERROR: Failed to send next-day reminder for ReportStatusID %d: %v", rs.ID, err)
                // If send fails, what to do? Status is already NEXT_DAY_REMINDER_SENT.
                // This indicates the attempt was made. Perhaps log and monitor send failures.
                // We won't update the DB status if send failed here, as LastNotifiedAt won't be current.
                // The sendSpecificReportQuestion updates LastNotifiedAt on successful send.
                // Let's ensure we update the status to NEXT_DAY_REMINDER_SENT regardless of send outcome,
                // but LastNotifiedAt only if send was successful.
                // The sendSpecificReportQuestion already sets LastNotifiedAt.

                // Re-fetch to get the latest LastNotifiedAt if sendSpecificReportQuestion updated it.
                // However, sendSpecificReportQuestion takes a *pointer* to teacher.Teacher, not ReportStatus.
                // So, we need to update the status record *after* sendSpecificReportQuestion.
                // If sendSpecificReportQuestion fails, we still update status to NEXT_DAY_REMINDER_SENT
                // but LastNotifiedAt would not be updated by it.
                rs.Status = notification.StatusNextDayReminderSent // Ensure this is the final status for this attempt
                if errUpdate := s.notifRepo.UpdateReportStatus(ctx, rs); errUpdate != nil {
                     s.logger.Printf("ERROR: Failed to update ReportStatusID %d after FAILED next-day reminder send attempt: %v", rs.ID, errUpdate)
                }

            } else {
                // sendSpecificReportQuestion on success would have updated rs.LastNotifiedAt (via its own UpdateReportStatus call for that status).
                // Now, we ensure the status is NEXT_DAY_REMINDER_SENT.
                // Fetch the latest version of rs as sendSpecificReportQuestion might have updated it (especially LastNotifiedAt).
                updatedRs, fetchErr := s.notifRepo.GetReportStatusByID(ctx, rs.ID)
                if fetchErr != nil {
                    s.logger.Printf("ERROR: Failed to re-fetch ReportStatusID %d after successful next-day reminder send: %v", rs.ID, fetchErr)
                    // Fallback to original rs, but LastNotifiedAt might be stale
                    updatedRs = rs
                }
                updatedRs.Status = notification.StatusNextDayReminderSent // Final status for this path
                updatedRs.ResponseAttempts = rs.ResponseAttempts // Preserve incremented attempts

                if errUpdate := s.notifRepo.UpdateReportStatus(ctx, updatedRs); errUpdate != nil {
                     s.logger.Printf("ERROR: Failed to update ReportStatusID %d after successful next-day reminder: %v", updatedRs.ID, errUpdate)
                } else {
                     s.logger.Printf("INFO: Successfully sent and updated status for next-day reminder for ReportStatusID: %d", updatedRs.ID)
                }
            }
        }
        return nil
    }
    ```

4.  **Add Cron Job for Next-Day Reminder Check:**
    * In `internal/infra/scheduler/scheduler.go`:
        * Add `cronSpecNextDayCheck` to `NotificationScheduler` struct and `NewNotificationScheduler` parameters.
        * Add the new cron job in `NotificationScheduler.Start()`:
    ```go
    // internal/infra/scheduler/scheduler.go
    // In NewNotificationScheduler, add cronSpecNextDayCheck string argument
    // In NotificationScheduler struct, add field: cronSpecNextDayCheck string
    // In NotificationScheduler.Start():
    // ...
        _, err = s.cronEngine.AddFunc(s.cronSpecNextDayCheck, func() {
            s.logger.Println("INFO: Cron job triggered for processing next-day reminders.")
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) // Longer timeout for potentially more items
            defer cancel()
            if err := s.notifService.ProcessNextDayReminders(ctx); err != nil {
                s.logger.Printf("ERROR: Error during next-day reminder processing: %v", err)
            }
        })
        if err != nil {
            s.logger.Fatalf("FATAL: Could not add next-day reminder processing cron job: %v", err)
        }
    // ...
    ```
    * Update `AppConfig` (add `CronSpecNextDayCheck`, e.g., default `"0 9 * * *"` for 9 AM) and `.env.example`.
    * Update `main.go` to pass the new cron spec to `NewNotificationScheduler`.

---

**Acceptance Criteria:**

* A new `InteractionStatus` `StatusNextDayReminderSent` is defined.
* `NotificationRepository` includes a `ListStalledStatusesFromPreviousDay` method that correctly queries for `TeacherReportStatus` records:
    * Where `last_notified_at` falls within the entire span of the "previous day" (e.g., from yesterday 00:00:00 to yesterday 23:59:59.999).
    * And `status` is one of the specified non-responsive types (`StatusPendingQuestion`, `StatusAwaitingReminder1H`).
* `NotificationServiceImpl.ProcessNextDayReminders` is implemented:
    * It runs via a daily cron job (e.g., configured for 09:00 AM server time).
    * It correctly calculates the "previous day's" date range.
    * It calls `ListStalledStatusesFromPreviousDay` to fetch relevant statuses.
    * For each fetched "stalled" status:
        * The original question associated with its `reportKey` is re-sent to the teacher (using `sendSpecificReportQuestion`).
        * The `TeacherReportStatus` record is updated:
            * `status` is set to `StatusNextDayReminderSent`.
            * `last_notified_at` is updated to the current time of sending the reminder.
            * `response_attempts` is incremented.
            * `remind_at` (if previously set for 1-hour reminder) can be considered cleared or irrelevant for this state.
* This next-day reminder is a single, final automated attempt for that specific report query within the cycle.
* Configuration for the "next-day reminder check" cron spec is added to `AppConfig` and `.env.example`, and wired into the scheduler in `main.go`.

---

**Critical Tests (Manual & Logging Based):**

1.  **Next-Day Reminder for No Response to Initial Question:**
    * **Setup "Day 1":**
        * Trigger initial notification (Task B010) for a teacher. They receive the "Таблица 1" question.
        * `TeacherReportStatus` for "Таблица 1" is `StatusPendingQuestion`, `last_notified_at` is on "Day 1".
        * The teacher does *not* respond at all on "Day 1".
    * **Action "Day 2":**
        * Manually trigger the `ProcessNextDayReminders` method (e.g., by adjusting its cron spec to run now, or calling it directly in a test setup in `main.go` after advancing system time or ensuring "yesterday" matches "Day 1").
    * **Verify:**
        * **Logs:** The job should log that it found the stalled status for "Таблица 1" from "Day 1".
        * **Telegram (Teacher):** The teacher should receive the "Таблица 1" question again on "Day 2".
        * **DB:** The `teacher_report_statuses` record for "Таблица 1" should now have `status = StatusNextDayReminderSent`, `last_notified_at` updated to "Day 2's" send time, and `response_attempts` incremented.
2.  **Next-Day Reminder for No Response to 1-Hour Reminder:**
    * **Setup "Day 1":**
        * Teacher receives "Таблица 1" question (B010).
        * Teacher clicks "Нет" (B012). Status becomes `StatusAnsweredNo`.
        * The 1-hour reminder for "Таблица 1" is sent later on "Day 1" (B012). Status becomes `StatusAwaitingReminder1H`, `last_notified_at` is updated to this reminder time on "Day 1".
        * The teacher does *not* respond to this 1-hour reminder on "Day 1".
    * **Action "Day 2":**
        * Trigger `ProcessNextDayReminders`.
    * **Verify:**
        * **Logs:** Job finds the stalled `StatusAwaitingReminder1H` for "Таблица 1" from "Day 1".
        * **Telegram (Teacher):** Teacher receives "Таблица 1" question again on "Day 2".
        * **DB:** Status becomes `StatusNextDayReminderSent`, `last_notified_at` updated to "Day 2", `response_attempts` incremented.
3.  **No Next-Day Reminder if Teacher Responded on "Day 1":**
    * **Setup "Day 1":**
        * Teacher receives "Таблица 1" question.
        * Teacher responds "Да" (or "Нет" then "Да" to the 1-hour reminder) on "Day 1". The status is now `StatusAnsweredYes` or has progressed.
    * **Action "Day 2":**
        * Trigger `ProcessNextDayReminders`.
    * **Verify:**
        * **Logs:** The job runs. `ListStalledStatusesFromPreviousDay` should *not* return this teacher's "Таблица 1" status because its status is no longer `StatusPendingQuestion` or `StatusAwaitingReminder1H`.
        * **Telegram (Teacher):** Teacher does *not* receive a next-day reminder for "Таблица 1".
4.  **No Further Reminder after `StatusNextDayReminderSent`:**
    * **Setup:** Complete Test 1 or Test 2. The status for "Таблица 1" is now `StatusNextDayReminderSent`, and `last_notified_at` was on "Day 2".
    * **Action "Day 3":**
        * Trigger `ProcessNextDayReminders`.
    * **Verify:**
        * **Logs:** The job runs. `ListStalledStatusesFromPreviousDay` (checking for statuses from "Day 2" that are `StatusPendingQuestion` or `StatusAwaitingReminder1H`) should *not* pick up the `StatusNextDayReminderSent` status.
        * **Telegram (Teacher):** No new reminder sent.