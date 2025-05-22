## Backend Task: B011 - Implement Handling of Teacher's "Yes" Response

**Objective:**
To implement the Telegram callback query handler and associated application service logic for when a teacher clicks the "Да" (Yes) button in response to a report question. This involves updating the `TeacherReportStatus`, determining if more questions are pending for the current cycle, sending the next question if applicable, or finalizing the process by notifying the manager and the teacher if all reports are confirmed.

**Background:**
This is a crucial step in the bot's interactive workflow[cite: 16, 17]. Correctly processing a "Yes" response moves the teacher through the checklist of required reports. If all reports are confirmed, the system needs to notify the manager for payroll processing and inform the teacher of completion, as per PRD requirements FR4.1, FR5.1, FR5.2, and Use Case UC1[cite: 39, 40, 41, 42, 43, 44, 64, 65, 70, 71].

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Dependencies: `teacher.Repository`, `notification.Repository`, `AppConfig`, `NotificationService`, `TelegramClient`, Logger

---

**Steps to Completion:**

1.  **Enhance `NotificationServiceImpl` in `internal/app/notification_service.go`:**
    * Add the `ProcessTeacherYesResponse` method.
    * Add a private helper method `sendManagerConfirmationAndTeacherFinalReply`.
    ```go
    // internal/app/notification_service.go
    // ... (existing imports and NotificationServiceImpl struct) ...

    func (s *NotificationServiceImpl) ProcessTeacherYesResponse(ctx context.Context, reportStatusID int64) error {
        s.logger.Printf("INFO: Processing 'Yes' response for ReportStatusID: %d", reportStatusID)

        // 1a. Fetch TeacherReportStatus
        currentReportStatus, err := s.notifRepo.GetReportStatusByID(ctx, reportStatusID)
        if err != nil {
            if err == idb.ErrReportStatusNotFound {
                s.logger.Printf("WARN: ReportStatusID %d not found. Possibly a stale callback.", reportStatusID)
                return nil // Acknowledge callback, but nothing to process
            }
            s.logger.Printf("ERROR: Failed to get report status by ID %d: %v", reportStatusID, err)
            return fmt.Errorf("failed to get report status by ID %d: %w", reportStatusID, err)
        }

        // If already answered 'Yes', to prevent reprocessing (e.g. double clicks)
        if currentReportStatus.Status == notification.StatusAnsweredYes {
            s.logger.Printf("INFO: ReportStatusID %d already marked as ANSWERED_YES. No action needed.", reportStatusID)
            return nil
        }

        // 1b. Update Status
        currentReportStatus.Status = notification.StatusAnsweredYes
        currentReportStatus.UpdatedAt = time.Now() // Service layer can set this before repo call
        if err := s.notifRepo.UpdateReportStatus(ctx, currentReportStatus); err != nil {
            s.logger.Printf("ERROR: Failed to update report status ID %d to ANSWERED_YES: %v", reportStatusID, err)
            return fmt.Errorf("failed to update report status ID %d: %w", reportStatusID, err)
        }
        s.logger.Printf("INFO: ReportStatusID %d updated to ANSWERED_YES.", reportStatusID)

        // 1c. Fetch Teacher and Cycle details
        teacherInfo, err := s.teacherRepo.GetByID(ctx, currentReportStatus.TeacherID)
        if err != nil {
            s.logger.Printf("ERROR: Failed to get teacher details for TeacherID %d: %v", currentReportStatus.TeacherID, err)
            return fmt.Errorf("failed to get teacher %d: %w", currentReportStatus.TeacherID, err)
        }

        currentCycle, err := s.notifRepo.GetCycleByID(ctx, currentReportStatus.CycleID)
        if err != nil {
            s.logger.Printf("ERROR: Failed to get cycle details for CycleID %d: %v", currentReportStatus.CycleID, err)
            return fmt.Errorf("failed to get cycle %d: %w", currentReportStatus.CycleID, err)
        }

        // 1d. Determine Next Action
        allExpectedReportsForCycle := determineReportsForCycle(currentCycle.Type) // Helper from B010

        allConfirmed, err := s.notifRepo.AreAllReportsConfirmedForTeacher(ctx, teacherInfo.ID, currentCycle.ID, allExpectedReportsForCycle)
        if err != nil {
            s.logger.Printf("ERROR: Failed to check if all reports confirmed for TeacherID %d, CycleID %d: %v", teacherInfo.ID, currentCycle.ID, err)
            return fmt.Errorf("failed to check all reports confirmed for teacher %d, cycle %d: %w", teacherInfo.ID, currentCycle.ID, err)
        }

        if allConfirmed {
            s.logger.Printf("INFO: All reports confirmed for TeacherID %d in CycleID %d.", teacherInfo.ID, currentCycle.ID)
            return s.sendManagerConfirmationAndTeacherFinalReply(ctx, teacherInfo, currentCycle)
        } else {
            // Determine next report to ask
            nextReportKey, err := s.determineNextReportKey(ctx, teacherInfo.ID, currentCycle.ID, currentReportStatus.ReportKey, allExpectedReportsForCycle)
            if err != nil {
                s.logger.Printf("ERROR: Could not determine next report key for TeacherID %d, CycleID %d: %v", teacherInfo.ID, currentCycle.ID, err)
                // Potentially send a generic "something went wrong message" or just log
                return err // Or a more user-friendly error if this state is unexpected
            }

            if nextReportKey == "" { // Should be caught by allConfirmed, but as a safeguard
                s.logger.Printf("WARN: All reports appeared confirmed, but determineNextReportKey found no next key for TeacherID %d, CycleID %d. Finalizing.", teacherInfo.ID, currentCycle.ID)
                return s.sendManagerConfirmationAndTeacherFinalReply(ctx, teacherInfo, currentCycle)
            }

            s.logger.Printf("INFO: Next report for TeacherID %d in CycleID %d is %s.", teacherInfo.ID, currentCycle.ID, nextReportKey)
            return s.sendSpecificReportQuestion(ctx, teacherInfo, currentCycle.ID, nextReportKey)
        }
    }

    // determineNextReportKey finds the next report in sequence that isn't 'ANSWERED_YES'.
    func (s *NotificationServiceImpl) determineNextReportKey(ctx context.Context, teacherID int64, cycleID int32, currentAnsweredKey notification.ReportKey, allCycleKeys []notification.ReportKey) (notification.ReportKey, error) {
        foundCurrent := false
        for _, key := range allCycleKeys {
            if key == currentAnsweredKey {
                foundCurrent = true
                continue // Look for the one *after* the current one
            }
            if !foundCurrent && currentAnsweredKey != "" { // If currentAnsweredKey is empty, means we are looking for the first one
                 // This case might not be needed if this func is only called after one YES.
                 // For robustness, let's ensure we start from beginning if current is not in list or empty.
                 // However, allCycleKeys is ordered.
            }
            
            // This logic relies on allCycleKeys being correctly ordered.
            // We need to find the first key in allCycleKeys for which the status is NOT 'ANSWERED_YES'.
            // The `currentAnsweredKey` helps to know where we are, but the primary goal is to find the next *unanswered* one.

            // Simpler approach: iterate all expected keys and find the first one not yet 'ANSWERED_YES'.
            reportStatus, err := s.notifRepo.GetReportStatus(ctx, teacherID, cycleID, key)
            if err != nil {
                if err == idb.ErrReportStatusNotFound {
                    s.logger.Printf("WARN: Report status for TeacherID %d, CycleID %d, Key %s not found. This might be an issue or an uninitialized report.", teacherID, cycleID, key)
                    // This case means the status record wasn't created in B010, which is an issue.
                    // For now, we assume records exist. If it doesn't exist, it's PENDING.
                    return key, nil // Treat as pending if not found for some reason (should not happen)
                }
                return "", fmt.Errorf("error fetching status for key %s: %w", key, err)
            }
            if reportStatus.Status != notification.StatusAnsweredYes {
                return key, nil // This is the next key to ask
            }
        }
        return "", nil // All keys are StatusAnsweredYes or list is exhausted
    }
    
    // sendSpecificReportQuestion sends a question for a given report key.
    func (s *NotificationServiceImpl) sendSpecificReportQuestion(ctx context.Context, teacherInfo *teacher.Teacher, cycleID int32, reportKey notification.ReportKey) error {
        reportStatus, err := s.notifRepo.GetReportStatus(ctx, teacherInfo.ID, cycleID, reportKey)
        if err != nil {
            s.logger.Printf("ERROR: Could not fetch report status for TeacherID %d, ReportKey %s for sending specific question: %v", teacherInfo.ID, reportKey, err)
            return fmt.Errorf("failed to fetch status for %s: %w", reportKey, err)
        }

        var questionText string
        switch reportKey {
        case notification.ReportKeyTable1Lessons: // Should not typically be re-asked via this function, but for completeness
            questionText = fmt.Sprintf("Заполнена ли Таблица 1: Проведенные уроки (отчёт за текущий период)?")
        case notification.ReportKeyTable3Schedule:
            questionText = fmt.Sprintf("Отлично! Заполнена ли Таблица 3: Расписание (проверка актуальности)?") // [cite: 40]
        case notification.ReportKeyTable2OTV:
            questionText = fmt.Sprintf("Супер! Заполнена ли Таблица 2: Таблица ОТВ (все проведенные уроки за всё время)?") // [cite: 41]
        default:
            s.logger.Printf("ERROR: Unknown report key %s for TeacherID %d", reportKey, teacherInfo.ID)
            return fmt.Errorf("unknown report key: %s", reportKey)
        }
        
        fullMessage := fmt.Sprintf("Привет, %s! %s", teacherInfo.FirstName, questionText)

        replyMarkup := &telebot.ReplyMarkup{ResizeKeyboard: true}
        btnYes := replyMarkup.Data("Да", fmt.Sprintf("ans_yes_%d", reportStatus.ID))
        btnNo := replyMarkup.Data("Нет", fmt.Sprintf("ans_no_%d", reportStatus.ID))
        replyMarkup.Inline(replyMarkup.Row(btnYes, btnNo))

        err = s.telegramClient.SendMessage(teacherInfo.TelegramID, fullMessage, &telebot.SendOptions{ReplyMarkup: replyMarkup})
        if err != nil {
            s.logger.Printf("ERROR: Failed to send question for %s to Teacher %s (TG_ID: %d): %v", reportKey, teacherInfo.FirstName, teacherInfo.TelegramID, err)
            return fmt.Errorf("failed to send question for %s: %w", reportKey, err)
        }
        s.logger.Printf("INFO: Successfully sent question for %s to Teacher %s (TG_ID: %d)", reportKey, teacherInfo.FirstName, teacherInfo.TelegramID)
        
        reportStatus.LastNotifiedAt = sql.NullTime{Time: time.Now(), Valid: true}
        reportStatus.Status = notification.StatusPendingQuestion // Ensure it's marked as pending if re-asking
        if errUpdate := s.notifRepo.UpdateReportStatus(ctx, reportStatus); errUpdate != nil {
            s.logger.Printf("ERROR: Failed to update LastNotifiedAt for ReportStatusID %d after sending question: %v", reportStatus.ID, errUpdate)
        }
        return nil
    }

    // sendManagerConfirmationAndTeacherFinalReply handles the final messages.
    func (s *NotificationServiceImpl) sendManagerConfirmationAndTeacherFinalReply(ctx context.Context, teacherInfo *teacher.Teacher, cycleInfo *notification.Cycle) error {
        // 1. Send to Manager
        // Fetch manager ID from config - this needs to be available to the service, e.g. via AppConfig passed to NewNotificationServiceImpl
        // For now, assuming a placeholder s.managerTelegramID
        // This means AppConfig needs to be a dependency or relevant parts of it.
        // Let's assume for now it's hardcoded or passed during service creation for simplicity of this task.
        // In a real app, cfg := s.config; managerID := cfg.ManagerTelegramID;
        managerTelegramID := int64(0) // Placeholder - This MUST be replaced by config value
        if s.appConfigProvider != nil { // Assuming an AppConfigProvider interface or direct config struct
            managerTelegramID = s.appConfigProvider.GetManagerTelegramID()
        } else {
             s.logger.Printf("WARN: Manager Telegram ID not configured in NotificationService. Cannot send manager confirmation.")
             // For this task, we'll proceed to teacher reply even if manager ID is missing
        }

        if managerTelegramID != 0 {
            teacherFullName := teacherInfo.FirstName
            if teacherInfo.LastName.Valid {
                teacherFullName += " " + teacherInfo.LastName.String
            }
            managerMessage := fmt.Sprintf("Преподаватель %s подтвердил заполнение всех таблиц. Можно выплачивать ЗП.", teacherFullName) // [cite: 19, 71]
            
            err := s.telegramClient.SendMessage(managerTelegramID, managerMessage, nil) // No keyboard for manager
            if err != nil {
                s.logger.Printf("ERROR: Failed to send confirmation to manager (ID: %d) for teacher %s: %v", managerTelegramID, teacherFullName, err)
                // Non-fatal for teacher's experience, but should be monitored
            } else {
                s.logger.Printf("INFO: Confirmation sent to manager (ID: %d) for teacher %s.", managerTelegramID, teacherFullName)
            }
        }

        // 2. Send to Teacher
        teacherReplyMessage := "Спасибо! Все таблицы подтверждены." // [cite: 43]
        err := s.telegramClient.SendMessage(teacherInfo.TelegramID, teacherReplyMessage, nil) // No keyboard for final teacher reply
        if err != nil {
            s.logger.Printf("ERROR: Failed to send final confirmation to teacher %s (TG_ID: %d): %v", teacherInfo.FirstName, teacherInfo.TelegramID, err)
            return fmt.Errorf("failed to send final reply to teacher: %w", err) // This error is more critical to report
        }
        s.logger.Printf("INFO: Final confirmation sent to teacher %s (TG_ID: %d).", teacherInfo.FirstName, teacherInfo.TelegramID)
        return nil
    }

    // Add AppConfigProvider to NotificationServiceImpl if not already there:
    // type NotificationServiceImpl struct {
    //     // ... other fields
    //     appConfigProvider config.Provider // Assuming config.Provider interface with GetManagerTelegramID()
    // }
    // And update NewNotificationServiceImpl to accept it.
    // For this task, we will assume managerTelegramID is retrieved within sendManagerConfirmationAndTeacherFinalReply if needed.
    // Let's simplify: Pass managerTelegramID directly to NewNotificationServiceImpl
    // Or, it could be a field in the service struct initialized from AppConfig in main.go.
    // For current task structure, add managerTelegramID to NotificationServiceImpl fields.
    // Updated NotificationServiceImpl struct:
    // type NotificationServiceImpl struct {
    //     teacherRepo    teacher.Repository
    //     notifRepo      notification.Repository
    //     telegramClient telegram.Client
    //     logger         *log.Logger
    //     managerTelegramID int64 // Added
    // }
    // func NewNotificationServiceImpl(..., managerID int64) *NotificationServiceImpl {
    //     return &NotificationServiceImpl{..., managerTelegramID: managerID}
    // }
    // And use s.managerTelegramID in sendManagerConfirmationAndTeacherFinalReply
    ```
    * **Self-correction for `sendManagerConfirmationAndTeacherFinalReply`:** The `managerTelegramID` needs to be available. The simplest way for this task is to add it as a field to `NotificationServiceImpl` and initialize it in `main.go` from `cfg.ManagerTelegramID`.

2.  **Update `NotificationServiceImpl` struct and constructor:**
    * Add `managerTelegramID int64` to `NotificationServiceImpl` fields.
    * Update `NewNotificationServiceImpl` to accept and store `managerTelegramID`.
    ```go
    // internal/app/notification_service.go
    // ... (imports) ...
    type NotificationServiceImpl struct {
        teacherRepo       teacher.Repository
        notifRepo         notification.Repository
        telegramClient    telegram.Client
        logger            *log.Logger
        managerTelegramID int64 // Added
    }

    func NewNotificationServiceImpl(
        tr teacher.Repository,
        nr notification.Repository,
        tc telegram.Client,
        logger *log.Logger,
        managerID int64, // Added
    ) *NotificationServiceImpl {
        return &NotificationServiceImpl{
            teacherRepo:       tr,
            notifRepo:         nr,
            telegramClient:    tc,
            logger:            logger,
            managerTelegramID: managerID, // Added
        }
    }
    // Now, in sendManagerConfirmationAndTeacherFinalReply, use s.managerTelegramID
    // ...
    // if s.managerTelegramID != 0 {
    //    err := s.telegramClient.SendMessage(s.managerTelegramID, managerMessage, nil)
    // ...
    ```

3.  **Implement Telegram Callback Handler for "Да":**
    * Create `internal/infra/telegram/teacher_response_handlers.go`.
    ```go
    // internal/infra/telegram/teacher_response_handlers.go
    package telegram

    import (
        "fmt"
        "strconv"
        "strings"
        "teacher_notification_bot/internal/app" // For NotificationService interface

        "gopkg.in/telebot.v3"
    )

    func RegisterTeacherResponseHandlers(b *telebot.Bot, notificationService app.NotificationService) {
        // Handler for "ans_yes_<report_status_id>"
        b.Handle(telebot. हरहССబ్యాక్యCallback(startsWith: "ans_yes_"), func(c telebot.Context) error {
            callbackData := c.Callback().Data
            parts := strings.Split(callbackData, "_") // ans_yes_123
            if len(parts) != 3 {
                c.Bot().OnError(fmt.Errorf("invalid callback data format for 'yes': %s", callbackData), c)
                return c.Respond(&telebot.CallbackResponse{Text: "Ошибка обработки ответа."})
            }

            reportStatusIDStr := parts[2]
            reportStatusID, err := strconv.ParseInt(reportStatusIDStr, 10, 64)
            if err != nil {
                c.Bot().OnError(fmt.Errorf("invalid reportStatusID '%s' in callback: %w", reportStatusIDStr, err), c)
                return c.Respond(&telebot.CallbackResponse{Text: "Ошибка ID отчета."})
            }
            
            // Call the application service
            err = notificationService.ProcessTeacherYesResponse(c.Request().Context(), reportStatusID)
            if err != nil {
                // Log the error in the service layer. Respond generically to user.
                // Specific user-facing messages should be sent by the service if it can recover or guide user.
                c.Bot().OnError(fmt.Errorf("error processing 'Yes' response for statusID %d: %w", reportStatusID, err), c) // Log full error
                // Optionally, send a generic error message back to the user via a normal message if appropriate
                // For callbacks, a simple Respond is often enough unless direct feedback is needed.
                // c.Send("Произошла внутренняя ошибка. Пожалуйста, попробуйте позже или свяжитесь с администратором.")
                return c.Respond(&telebot.CallbackResponse{Text: "Произошла ошибка."})
            }

            return c.Respond(&telebot.CallbackResponse{Text: "Ответ 'Да' принят!"}) // Simple ack
        })

        // Placeholder for "ans_no_<report_status_id>" - to be implemented in B012
        b.Handle(telebot. हरहССబ్యాక్యCallback(startsWith: "ans_no_"), func(c telebot.Context) error {
            // Logic for "No" will be in Task B012
            c.Bot().OnError(fmt.Errorf("received 'NO' callback, not yet implemented: %s", c.Callback().Data), c)
            return c.Respond(&telebot.CallbackResponse{Text: "Обработка ответа 'Нет' в разработке."})
        })
    }
    ```
    * **Note on `telebot. हरहССబ్యాక్యCallback`:** The `telebot. हरहССబ్యాక్యCallback` is a placeholder for `telebot. сегментCallback` which appears to be the correct way to match callback data prefixes in `gopkg.in/telebot.v3`. If that exact helper doesn't exist, a general `b.Handle(telebot.OnCallback, ...)` with manual prefix checking inside the handler would be used. Assuming a helper like `telebot.prefixCallback` or `telebot.Route("ans_yes_", ...)` or similar exists for cleaner routing, or a general callback handler with string prefix check. For `gopkg.in/telebot.v3`, it's typically `b.Handle("\f" + "ans_yes_", handlerFunc)` where `\f` denotes a callback endpoint. Or using `c.Callback().Unique` for exact match, but for prefix matching we would need a general handler and then a `strings.HasPrefix`. Let's assume the `\f<prefix>` method.

4.  **Correct Callback Registration in `teacher_response_handlers.go`:**
    ```go
    // internal/infra/telegram/teacher_response_handlers.go
    // ...
    func RegisterTeacherResponseHandlers(b *telebot.Bot, notificationService app.NotificationService) {
        // Using the \f<unique_id> pattern for telebot v3 callback handlers
        // The unique part of the callback data will be after the prefix.
        // We need a general handler for callbacks and then parse.
        // Or, if the library supports regex/parameterized routes for callbacks, use that.
        // For simplicity, let's assume a general callback handler and parse inside.
        // However, the "ans_yes_" prefix is better handled by specific endpoint if possible.

        // Corrected way for telebot.v3 for prefix matching (if it exists as such)
        // Or, more commonly:
        b.Handle(telebot.OnCallback, func(c telebot.Context) error {
            data := c.Callback().Data
            // Manually route based on prefix
            if strings.HasPrefix(data, "ans_yes_") {
                parts := strings.Split(data, "_") // ans_yes_123
                if len(parts) != 3 {
                    c.Bot().OnError(fmt.Errorf("invalid callback data format for 'yes': %s", data), c)
                    return c.Respond(&telebot.CallbackResponse{Text: "Ошибка обработки ответа."})
                }
                reportStatusIDStr := parts[2]
                reportStatusID, err := strconv.ParseInt(reportStatusIDStr, 10, 64)
                if err != nil {
                    c.Bot().OnError(fmt.Errorf("invalid reportStatusID '%s' in callback: %w", reportStatusIDStr, err), c)
                    return c.Respond(&telebot.CallbackResponse{Text: "Ошибка ID отчета."})
                }
                err = notificationService.ProcessTeacherYesResponse(c.Request().Context(), reportStatusID)
                if err != nil {
                    c.Bot().OnError(fmt.Errorf("error processing 'Yes' response for statusID %d: %w", reportStatusID, err), c)
                    return c.Respond(&telebot.CallbackResponse{Text: "Произошла ошибка."})
                }
                return c.Respond(&telebot.CallbackResponse{Text: "Ответ 'Да' принят!"}) // Simple ack
            } else if strings.HasPrefix(data, "ans_no_") {
                // Logic for "No" will be in Task B012
                c.Bot().OnError(fmt.Errorf("received 'NO' callback, not yet implemented: %s", data), c)
                return c.Respond(&telebot.CallbackResponse{Text: "Обработка ответа 'Нет' в разработке."})
            }
            // Fallback for unhandled callbacks
            c.Bot().OnError(fmt.Errorf("unhandled callback data: %s", data), c)
            return c.Respond(&telebot.CallbackResponse{Text: "Неизвестное действие."})
        })
    }
    ```

5.  **Update `main.go` to Register Teacher Response Handlers:**
    * In `cmd/bot/main.go`, call `telegram.RegisterTeacherResponseHandlers`.
    ```go
    // cmd/bot/main.go
    // ... (after bot and notificationService are initialized)
        telegram.RegisterAdminHandlers(bot, adminService, cfg.AdminTelegramID)
        telegram.RegisterTeacherResponseHandlers(bot, notificationService) // Added
        mainLogger.Println("INFO: Admin and Teacher Response command handlers registered.")
    // ...

    // Also update NewNotificationServiceImpl call in main.go:
    notificationService := app.NewNotificationServiceImpl(
        teacherRepo,
        notificationRepo,
        telegramClientAdapter,
        notifServiceLogger,
        cfg.ManagerTelegramID, // Pass ManagerTelegramID
    )
    ```

---

**Acceptance Criteria:**

* The `NotificationServiceImpl` in `internal/app/notification_service.go` is enhanced with a `ProcessTeacherYesResponse` method and has access to `ManagerTelegramID`.
* When `ProcessTeacherYesResponse` is invoked with a valid `reportStatusID` for a report that is currently `PENDING_QUESTION` or similar non-final state:
    * The `TeacherReportStatus.Status` for that `reportStatusID` is updated to `notification.StatusAnsweredYes` in the database.
    * **If it's NOT the last report in the sequence for the teacher and current cycle type:**
        * The system correctly identifies the next report (e.g., Table 3 after Table 1, or Table 2 after Table 3 for end-month).
        * The teacher receives a new Telegram message asking the question for this *next* report, including new "Да"/"Нет" inline buttons linked to the *next report's status ID*.
        * The `LastNotifiedAt` and `Status` (to `PENDING_QUESTION`) fields for this newly asked report's `TeacherReportStatus` record are updated.
    * **If it IS the last report in the sequence for the teacher/cycle (and all expected reports are now `StatusAnsweredYes`):**
        * The configured Manager (from `cfg.ManagerTelegramID`) receives a Telegram message: "Преподаватель [Teacher First Name Last Name] подтвердил заполнение всех таблиц. Можно выплачивать ЗП."[cite: 19, 71].
        * The Teacher who responded receives a final confirmation Telegram message: "Спасибо! Все таблицы подтверждены."[cite: 43].
* A Telegram callback query handler is implemented in `internal/infra/telegram/teacher_response_handlers.go`.
* This handler correctly:
    * Triggers for callback data that starts with `ans_yes_` (or a similar convention defined in B010).
    * Parses the `reportStatusID` from the callback data.
    * Calls `NotificationService.ProcessTeacherYesResponse` with the extracted `reportStatusID`.
    * Sends an acknowledgement to Telegram for the callback query (e.g., `c.Respond()`).
* If `ProcessTeacherYesResponse` is called for a `reportStatusID` that is already `StatusAnsweredYes`, it handles this gracefully (e.g., logs it, acknowledges callback, does not re-process).
* The new `RegisterTeacherResponseHandlers` function is called in `main.go` to activate the callback handlers.

---

**Critical Tests (Manual & Logging Based):**

1.  **Mid-Month Cycle - "Yes" to Table 1:**
    * **Setup:** Initiate a `MID_MONTH` notification cycle for an active teacher (as done in B010). The teacher should have received the question for "Таблица 1".
    * **Action:** Click the "Да" button on the "Таблица 1" message received by the teacher's Telegram account.
    * **Verify:**
        * **DB:** The `teacher_report_statuses` record for "Таблица 1" for this teacher/cycle should be updated to `status = 'ANSWERED_YES'`.
        * **Telegram (Teacher):** The teacher should receive a new message asking about "Таблица 3: Расписание" [cite: 40] with new "Да"/"Нет" buttons.
        * **DB:** The `teacher_report_statuses` record for "Таблица 3" should have its `last_notified_at` updated and `status` set to `PENDING_QUESTION`.
        * **Telegram (Manager):** The manager should *not* receive any message yet.
        * **Logs:** Confirm the sequence of operations and decisions in service logs.
2.  **Mid-Month Cycle - "Yes" to Table 3 (Final Confirmation):**
    * **Setup:** Continuing from Test 1, the teacher has now received the question for "Таблица 3".
    * **Action:** Click the "Да" button on the "Таблица 3" message.
    * **Verify:**
        * **DB:** `teacher_report_statuses` for "Таблица 3" updated to `status = 'ANSWERED_YES'`.
        * **Telegram (Teacher):** Teacher receives "Спасибо! Все таблицы подтверждены."[cite: 43].
        * **Telegram (Manager):** Manager receives "Преподаватель [Teacher Name] подтвердил заполнение всех таблиц. Можно выплачивать ЗП."[cite: 19, 71].
        * **Logs:** Confirm finalization logic.
3.  **End-Month Cycle - Sequence Test:**
    * **Setup:** Initiate an `END_MONTH` cycle. Teacher receives "Таблица 1" question.
    * **Action 1:** Teacher clicks "Да" for Table 1.
    * **Verify 1:** Teacher receives "Таблица 3" question. DB updated for Table 1 (YES) and Table 3 (PENDING, notified). Manager NOT notified.
    * **Action 2:** Teacher clicks "Да" for Table 3.
    * **Verify 2:** Teacher receives "Таблица 2: Таблица ОТВ" question[cite: 41]. DB updated for Table 3 (YES) and Table 2 (PENDING, notified). Manager NOT notified.
    * **Action 3:** Teacher clicks "Да" for Table 2.
    * **Verify 3:** Same final outcome as Test 2 (final messages to teacher and manager).
4.  **Invalid `reportStatusID` in Callback:**
    * **Action:** Manually craft and send (if possible with a bot client, or simulate by calling handler with bad data) a callback like `ans_yes_999999` (non-existent ID).
    * **Verify:** Bot acknowledges callback to Telegram. Error is logged in the application. No crash. Teacher ideally sees a generic ack or no disruptive error.
5.  **Callback for Already `ANSWERED_YES` Status:**
    * **Setup:** Complete Test 1 (Teacher answered "Yes" to Table 1, status updated).
    * **Action:** Click the "Да" button *again* on the *original* "Таблица 1" message (if the buttons are still active/message exists).
    * **Verify:** System handles this gracefully. No duplicate messages (e.g., Table 3 question isn't sent again). No duplicate manager notification. Callback is acknowledged. Logs indicate status was already "ANSWERED_YES".