// internal/app/notification_service.go
package app

import (
	"context"
	"database/sql"
	"fmt"
	"teacher_notification_bot/internal/domain/notification" // Adjust import path
	"teacher_notification_bot/internal/domain/teacher"
	domainTelegram "teacher_notification_bot/internal/domain/telegram" // Import from domain
	idb "teacher_notification_bot/internal/infra/database"             // Alias for your DB errors
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/telebot.v3" // For telebot.ReplyMarkup and telebot.SendOptions
)

// NotificationService defines the operations for managing the notification process.
// This is a placeholder for now; its full implementation will come in later tasks.
type NotificationService interface {
	// InitiateNotificationProcess starts the notification workflow for a given cycle type.
	// It will find/create a NotificationCycle, identify target teachers,
	// create initial TeacherReportStatus entries, and send the first notifications.
	InitiateNotificationProcess(ctx context.Context, cycleType notification.CycleType, cycleDate time.Time) error
	ProcessTeacherYesResponse(ctx context.Context, reportStatusID int64) error
	ProcessTeacherNoResponse(ctx context.Context, reportStatusID int64) error
	ProcessScheduled1HourReminders(ctx context.Context) error
	ProcessNextDayReminders(ctx context.Context) error
}

// NotificationServiceImpl implements the NotificationService interface.
type NotificationServiceImpl struct {
	teacherRepo       teacher.Repository
	notifRepo         notification.Repository
	telegramClient    domainTelegram.Client // Use the interface from the domain package
	log               *logrus.Entry
	managerTelegramID int64 // Added
}

func NewNotificationServiceImpl(
	tr teacher.Repository,
	nr notification.Repository,
	tc domainTelegram.Client, // Use the interface from the domain package
	baseLogger *logrus.Entry,
	managerID int64, // Added
) *NotificationServiceImpl {
	return &NotificationServiceImpl{
		teacherRepo:       tr,
		notifRepo:         nr,
		telegramClient:    tc,
		log:               baseLogger,
		managerTelegramID: managerID, // Added
	}
}

// InitiateNotificationProcess starts the notification workflow.
func (s *NotificationServiceImpl) InitiateNotificationProcess(ctx context.Context, cycleType notification.CycleType, cycleDate time.Time) error {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":  "InitiateNotificationProcess",
		"cycle_type": cycleType,
		"cycle_date": cycleDate.Format("2006-01-02"),
	})
	logCtx.Info("Initiating notification process")

	// 1. Find or Create NotificationCycle
	currentCycle, err := s.notifRepo.GetCycleByDateAndType(ctx, cycleDate, cycleType)
	if err != nil {
		if err == idb.ErrCycleNotFound {
			logCtx.Info("No existing cycle found. Creating new cycle.")
			newCycle := &notification.Cycle{ // Create as a pointer
				CycleDate: cycleDate,
				Type:      cycleType,
			}
			if err := s.notifRepo.CreateCycle(ctx, newCycle); err != nil {
				logCtx.WithError(err).Error("Failed to create notification cycle")
				return fmt.Errorf("failed to create notification cycle: %w", err)
			}
			currentCycle = newCycle // Assign the pointer
			logCtx.WithField("cycle_id", currentCycle.ID).Info("New notification cycle created")
		} else {
			logCtx.WithError(err).Error("Failed to get notification cycle")
			return fmt.Errorf("failed to get notification cycle: %w", err)
		}
	} else {
		logCtx.WithField("cycle_id", currentCycle.ID).Info("Existing cycle found.")
	}

	// 2. Fetch Active Teachers
	activeTeachers, err := s.teacherRepo.ListActive(ctx)
	if err != nil {
		logCtx.WithError(err).Error("Failed to list active teachers")
		return fmt.Errorf("failed to list active teachers: %w", err)
	}
	if len(activeTeachers) == 0 {
		logCtx.Info("No active teachers found. Notification process will not send any messages.")
		return nil
	}
	logCtx.WithField("active_teachers_count", len(activeTeachers)).Info("Found active teachers.")

	// 3. Determine Reports for the Cycle
	reportsForCycle := determineReportsForCycle(cycleType)
	if len(reportsForCycle) == 0 {
		logCtx.Warn("No reports defined for cycle type")
		return nil
	}

	// 4. Create Initial TeacherReportStatus Records (Bulk Preferred)
	var statusesToCreate []*notification.ReportStatus
	now := time.Now() // Use a consistent time for this batch of operations
	for _, t := range activeTeachers {
		for _, reportKey := range reportsForCycle {
			// Check if status already exists for this teacher, cycle, reportKey (idempotency)
			_, err := s.notifRepo.GetReportStatus(ctx, t.ID, currentCycle.ID, reportKey)
			if err == nil {
				logCtx.WithFields(logrus.Fields{"teacher_id": t.ID, "cycle_id": currentCycle.ID, "report_key": reportKey}).Info("Report status already exists. Skipping creation.")
				continue // Skip if already exists
			}
			if err != idb.ErrReportStatusNotFound {
				logCtx.WithError(err).WithFields(logrus.Fields{"teacher_id": t.ID, "cycle_id": currentCycle.ID, "report_key": reportKey}).Error("Failed to check existing report status")
				// Decide whether to continue with other statuses or return an error for the whole batch
				continue // For now, log and continue
			}

			statusesToCreate = append(statusesToCreate, &notification.ReportStatus{
				TeacherID:        t.ID,
				CycleID:          currentCycle.ID,
				ReportKey:        reportKey,
				Status:           notification.StatusPendingQuestion,
				LastNotifiedAt:   sql.NullTime{}, // Will be set after successful send for the specific notification
				ResponseAttempts: 0,
			})
		}
	}

	if len(statusesToCreate) > 0 {
		if err := s.notifRepo.BulkCreateReportStatuses(ctx, statusesToCreate); err != nil {
			logCtx.WithError(err).Error("Failed to bulk create teacher report statuses")
			// Depending on error, might need to decide if partial success is okay or rollback
			// For now, we log and proceed to send for successfully created/existing statuses.
		} else {
			logCtx.WithField("count", len(statusesToCreate)).Info("Successfully created/verified teacher report statuses.")
		}
	}

	// 5. Send First Notification (Table 1)
	firstReportKey := notification.ReportKeyTable1Lessons // Always start with Table 1
	for _, t := range activeTeachers {
		teacherLogCtx := logCtx.WithFields(logrus.Fields{"teacher_id": t.ID, "teacher_tg_id": t.TelegramID, "report_key": firstReportKey})
		reportStatus, err := s.notifRepo.GetReportStatus(ctx, t.ID, currentCycle.ID, firstReportKey)
		if err != nil {
			teacherLogCtx.WithError(err).Error("Could not fetch report status for sending initial notification")
			continue // Skip this teacher if their initial status record is missing
		}
		if reportStatus.Status != notification.StatusPendingQuestion {
			teacherLogCtx.WithField("status", reportStatus.Status).Info("Initial notification skipped, status is not PENDING_QUESTION.")
			continue
		}

		teacherName := t.FirstName
		messageText := fmt.Sprintf("Привет, %s! Заполнена ли Таблица 1: Проведенные уроки (отчёт за текущий период)?", teacherName)

		replyMarkup := &telebot.ReplyMarkup{ResizeKeyboard: true} // Inline keyboard
		btnYes := replyMarkup.Data("Да", fmt.Sprintf("ans_yes_%d", reportStatus.ID))
		btnNo := replyMarkup.Data("Нет", fmt.Sprintf("ans_no_%d", reportStatus.ID))
		replyMarkup.Inline(replyMarkup.Row(btnYes, btnNo))

		err = s.telegramClient.SendMessage(t.TelegramID, messageText, &telebot.SendOptions{ReplyMarkup: replyMarkup, ParseMode: telebot.ModeDefault})
		if err != nil {
			teacherLogCtx.WithError(err).Errorf("Failed to send initial notification for Table 1 to Teacher %s", teacherName)
		} else {
			teacherLogCtx.Infof("Successfully sent initial notification for Table 1 to Teacher %s", teacherName)
			reportStatus.LastNotifiedAt = sql.NullTime{Time: now, Valid: true} // Use the 'now' from the beginning of status processing for this batch
			if errUpdate := s.notifRepo.UpdateReportStatus(ctx, reportStatus); errUpdate != nil {
				teacherLogCtx.WithError(errUpdate).WithField("report_status_id", reportStatus.ID).Error("Failed to update LastNotifiedAt")
			}
		}
	}
	return nil
}

func determineReportsForCycle(cycleType notification.CycleType) []notification.ReportKey {
	switch cycleType {
	case notification.CycleTypeMidMonth:
		return []notification.ReportKey{
			notification.ReportKeyTable1Lessons,
			notification.ReportKeyTable3Schedule,
		}
	case notification.CycleTypeEndMonth:
		return []notification.ReportKey{
			notification.ReportKeyTable1Lessons,
			notification.ReportKeyTable3Schedule,
			notification.ReportKeyTable2OTV,
		}
	default:
		return []notification.ReportKey{}
	}
}

func (s *NotificationServiceImpl) ProcessTeacherYesResponse(ctx context.Context, reportStatusID int64) error {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":        "ProcessTeacherYesResponse",
		"report_status_id": reportStatusID,
	})
	logCtx.Info("Processing 'Yes' response")

	// 1a. Fetch TeacherReportStatus
	currentReportStatus, err := s.notifRepo.GetReportStatusByID(ctx, reportStatusID)
	if err != nil {
		if err == idb.ErrReportStatusNotFound {
			logCtx.Warn("ReportStatusID not found. Possibly a stale callback.")
			return nil // Acknowledge callback, but nothing to process
		}
		logCtx.WithError(err).Error("Failed to get report status by ID")
		return fmt.Errorf("failed to get report status by ID %d: %w", reportStatusID, err)
	}
	logCtx = logCtx.WithFields(logrus.Fields{"teacher_id": currentReportStatus.TeacherID, "cycle_id": currentReportStatus.CycleID, "report_key": currentReportStatus.ReportKey})

	// If already answered 'Yes', to prevent reprocessing (e.g. double clicks)
	if currentReportStatus.Status == notification.StatusAnsweredYes {
		logCtx.Info("ReportStatusID already marked as ANSWERED_YES. No action needed.")
		return nil
	}

	// 1b. Update Status
	currentReportStatus.Status = notification.StatusAnsweredYes
	currentReportStatus.UpdatedAt = time.Now() // Service layer can set this before repo call
	if err := s.notifRepo.UpdateReportStatus(ctx, currentReportStatus); err != nil {
		logCtx.WithError(err).Error("Failed to update report status to ANSWERED_YES")
		return fmt.Errorf("failed to update report status ID %d to ANSWERED_YES: %w", reportStatusID, err)
	}
	logCtx.Info("ReportStatusID updated to ANSWERED_YES.")

	// 1c. Fetch Teacher and Cycle details
	teacherInfo, err := s.teacherRepo.GetByID(ctx, currentReportStatus.TeacherID)
	if err != nil {
		logCtx.WithError(err).Error("Failed to get teacher details")
		return fmt.Errorf("failed to get teacher %d: %w", currentReportStatus.TeacherID, err)
	}

	currentCycle, err := s.notifRepo.GetCycleByID(ctx, currentReportStatus.CycleID)
	if err != nil {
		logCtx.WithError(err).Error("Failed to get cycle details")
		return fmt.Errorf("failed to get cycle %d: %w", currentReportStatus.CycleID, err)
	}
	logCtx = logCtx.WithField("cycle_type", currentCycle.Type)

	// 1d. Determine Next Action
	allExpectedReportsForCycle := determineReportsForCycle(currentCycle.Type)

	allConfirmed, err := s.notifRepo.AreAllReportsConfirmedForTeacher(ctx, teacherInfo.ID, currentCycle.ID, allExpectedReportsForCycle)
	if err != nil {
		logCtx.WithError(err).Error("Failed to check if all reports confirmed for teacher")
		return fmt.Errorf("failed to check all reports confirmed for teacher %d, cycle %d: %w", teacherInfo.ID, currentCycle.ID, err)
	}

	if allConfirmed {
		logCtx.Info("All reports confirmed for teacher in this cycle.")
		return s.sendManagerConfirmationAndTeacherFinalReply(ctx, teacherInfo, currentCycle)
	} else {
		// Determine next report to ask
		nextReportKey, err := s.determineNextReportKey(ctx, teacherInfo.ID, currentCycle.ID, currentReportStatus.ReportKey, allExpectedReportsForCycle)
		if err != nil {
			logCtx.WithError(err).Error("Could not determine next report key")
			return err
		}

		// Should not happen if allConfirmed is false, but as a safeguard
		if nextReportKey == "" {
			logCtx.Warn("All reports appeared confirmed, but determineNextReportKey found no next key. Finalizing.")
			return s.sendManagerConfirmationAndTeacherFinalReply(ctx, teacherInfo, currentCycle)
		}

		logCtx.WithField("next_report_key", nextReportKey).Info("Determined next report to ask.")
		return s.sendSpecificReportQuestion(ctx, teacherInfo, currentCycle.ID, nextReportKey)
	}
}

// determineNextReportKey finds the next report in sequence that isn't 'ANSWERED_YES'.
// currentAnsweredKey is passed for context but the simpler logic iterates all keys.
func (s *NotificationServiceImpl) determineNextReportKey(ctx context.Context, teacherID int64, cycleID int32, _ notification.ReportKey, allCycleKeys []notification.ReportKey) (notification.ReportKey, error) {
	logCtx := s.log.WithFields(logrus.Fields{"operation": "determineNextReportKey", "teacher_id": teacherID, "cycle_id": cycleID})
	for _, key := range allCycleKeys {
		reportStatus, err := s.notifRepo.GetReportStatus(ctx, teacherID, cycleID, key)
		if err != nil {
			if err == idb.ErrReportStatusNotFound {
				// If it's missing, it's effectively pending.
				return key, nil
			}
			logCtx.WithError(err).WithField("report_key", key).Error("Error fetching status for key")
			return "", fmt.Errorf("error fetching status for key %s: %w", key, err)
		}
		if reportStatus.Status != notification.StatusAnsweredYes {
			return key, nil
		}
	}
	return "", nil // All confirmed
}

// sendSpecificReportQuestion sends a question for a given report key.
func (s *NotificationServiceImpl) sendSpecificReportQuestion(ctx context.Context, teacherInfo *teacher.Teacher, cycleID int32, reportKey notification.ReportKey) error {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":     "sendSpecificReportQuestion",
		"teacher_id":    teacherInfo.ID,
		"teacher_tg_id": teacherInfo.TelegramID,
		"cycle_id":      cycleID,
		"report_key":    reportKey,
	})
	reportStatus, err := s.notifRepo.GetReportStatus(ctx, teacherInfo.ID, cycleID, reportKey)
	if err != nil {
		logCtx.WithError(err).Error("Could not fetch report status for sending specific question")
		return fmt.Errorf("failed to fetch status for %s: %w", reportKey, err)
	}

	// Ensure the status is PendingQuestion before sending
	if reportStatus.Status != notification.StatusPendingQuestion {
		logCtx.WithField("status", reportStatus.Status).Warn("Attempted to send question for status not PENDING_QUESTION")
		// Maybe update status here if it's something unexpected, but for now, just log and return.
		return fmt.Errorf("cannot send question for status %s", reportStatus.Status)
	}

	var questionText string
	switch reportKey {
	case notification.ReportKeyTable1Lessons:
		questionText = "Заполнена ли Таблица 1: Проведенные уроки (отчёт за текущий период)?"
	case notification.ReportKeyTable3Schedule:
		questionText = "Отлично! Заполнена ли Таблица 3: Расписание (проверка актуальности)?"
	case notification.ReportKeyTable2OTV:
		questionText = "Супер! Заполнена ли Таблица 2: Таблица ОТВ (все проведенные уроки за всё время)?"
	default:
		logCtx.Error("Unknown report key")
		return fmt.Errorf("unknown report key: %s", reportKey)
	}

	fullMessage := fmt.Sprintf("Привет, %s! %s", teacherInfo.FirstName, questionText)

	replyMarkup := &telebot.ReplyMarkup{ResizeKeyboard: true}
	btnYes := replyMarkup.Data("Да", fmt.Sprintf("ans_yes_%d", reportStatus.ID))
	btnNo := replyMarkup.Data("Нет", fmt.Sprintf("ans_no_%d", reportStatus.ID))
	replyMarkup.Inline(replyMarkup.Row(btnYes, btnNo))

	err = s.telegramClient.SendMessage(teacherInfo.TelegramID, fullMessage, &telebot.SendOptions{ReplyMarkup: replyMarkup})
	if err != nil {
		logCtx.WithError(err).Errorf("Failed to send question for %s to Teacher %s", reportKey, teacherInfo.FirstName)
		return fmt.Errorf("failed to send question for %s: %w", reportKey, err)
	}
	logCtx.Infof("Successfully sent question for %s to Teacher %s", reportKey, teacherInfo.FirstName)

	reportStatus.LastNotifiedAt = sql.NullTime{Time: time.Now(), Valid: true}
	reportStatus.Status = notification.StatusPendingQuestion // Ensure it's marked as pending
	if errUpdate := s.notifRepo.UpdateReportStatus(ctx, reportStatus); errUpdate != nil {
		logCtx.WithError(errUpdate).WithField("report_status_id", reportStatus.ID).Error("Failed to update LastNotifiedAt/Status after sending question")
	}
	return nil
}

// sendManagerConfirmationAndTeacherFinalReply handles the final messages.
func (s *NotificationServiceImpl) sendManagerConfirmationAndTeacherFinalReply(ctx context.Context, teacherInfo *teacher.Teacher, cycleInfo *notification.Cycle) error {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":     "sendManagerConfirmationAndTeacherFinalReply",
		"teacher_id":    teacherInfo.ID,
		"teacher_tg_id": teacherInfo.TelegramID,
		"cycle_id":      cycleInfo.ID,
	})
	if s.managerTelegramID != 0 {
		managerLogCtx := logCtx.WithField("manager_tg_id", s.managerTelegramID)
		teacherFullName := teacherInfo.FirstName
		if teacherInfo.LastName.Valid {
			teacherFullName += " " + teacherInfo.LastName.String
		}
		managerMessage := fmt.Sprintf("Преподаватель %s подтвердил(а) все таблицы для цикла %s (%s).", teacherFullName, cycleInfo.Type, cycleInfo.CycleDate.Format("2006-01-02"))

		err := s.telegramClient.SendMessage(s.managerTelegramID, managerMessage, &telebot.SendOptions{})
		if err != nil {
			managerLogCtx.WithError(err).Errorf("Failed to send confirmation to manager for teacher %s", teacherFullName)
		} else {
			managerLogCtx.Infof("Confirmation sent to manager for teacher %s.", teacherFullName)
		}
	} else {
		logCtx.Warn("Manager Telegram ID not configured. Cannot send manager confirmation.")
	}

	teacherReplyMessage := "Спасибо! Все таблицы подтверждены."
	err := s.telegramClient.SendMessage(teacherInfo.TelegramID, teacherReplyMessage, &telebot.SendOptions{})
	if err != nil {
		logCtx.WithError(err).Errorf("Failed to send final confirmation to teacher %s", teacherInfo.FirstName)
		return fmt.Errorf("failed to send final reply to teacher: %w", err)
	}
	logCtx.Infof("Final confirmation sent to teacher %s.", teacherInfo.FirstName)
	return nil
}

func (s *NotificationServiceImpl) ProcessTeacherNoResponse(ctx context.Context, reportStatusID int64) error {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":        "ProcessTeacherNoResponse",
		"report_status_id": reportStatusID,
	})
	logCtx.Info("Processing 'No' response")

	// 1a. Fetch TeacherReportStatus
	currentReportStatus, err := s.notifRepo.GetReportStatusByID(ctx, reportStatusID)
	if err != nil {
		if err == idb.ErrReportStatusNotFound {
			logCtx.Warn("ReportStatusID not found processing 'No' response. Possibly a stale callback.")
			return nil // Acknowledge callback, but nothing to process
		}
		logCtx.WithError(err).Error("Failed to get report status by ID")
		return fmt.Errorf("failed to get report status by ID %d: %w", reportStatusID, err)
	}
	logCtx = logCtx.WithFields(logrus.Fields{"teacher_id": currentReportStatus.TeacherID, "cycle_id": currentReportStatus.CycleID, "report_key": currentReportStatus.ReportKey})

	// If already awaiting reminder (e.g. from a previous 'No' click), prevent reprocessing.
	if currentReportStatus.Status == notification.StatusAwaitingReminder1H {
		logCtx.Info("ReportStatusID already in AWAITING_REMINDER_1H. Ignoring duplicate 'No' response.")
		return nil
	}

	// Fetch Teacher details for sending confirmation message
	teacherInfo, err := s.teacherRepo.GetByID(ctx, currentReportStatus.TeacherID)
	if err != nil {
		logCtx.WithError(err).Error("Failed to get teacher details")
		return fmt.Errorf("failed to get teacher %d: %w", currentReportStatus.TeacherID, err)
	}

	// Calculate reminder time (1 hour from now)
	reminderTime := time.Now().Add(1 * time.Hour)

	// 1b. Update Status and set reminder time
	currentReportStatus.Status = notification.StatusAwaitingReminder1H
	currentReportStatus.RemindAt = sql.NullTime{Time: reminderTime, Valid: true}
	currentReportStatus.UpdatedAt = time.Now()

	if err := s.notifRepo.UpdateReportStatus(ctx, currentReportStatus); err != nil {
		logCtx.WithError(err).Error("Failed to update report status to AWAITING_REMINDER_1H")
		// Attempt to inform teacher of the error
		_ = s.telegramClient.SendMessage(teacherInfo.TelegramID, "Произошла ошибка при обработке вашего ответа. Пожалуйста, попробуйте позже или свяжитесь с администратором.", &telebot.SendOptions{})
		return fmt.Errorf("failed to update report status ID %d to AWAITING_REMINDER_1H: %w", reportStatusID, err)
	}
	logCtx.WithField("remind_at", currentReportStatus.RemindAt.Time.Format(time.RFC3339)).Info("ReportStatusID updated to AWAITING_REMINDER_1H.")

	// Send confirmation message to teacher
	teacherMessage := "Понял(а). Напомню через час. Если заполните таблицу раньше, это сообщение можно будет проигнорировать."
	err = s.telegramClient.SendMessage(teacherInfo.TelegramID, teacherMessage, nil)
	if err != nil {
		logCtx.WithError(err).WithField("teacher_tg_id", teacherInfo.TelegramID).Errorf("Failed to send 'No' response confirmation to teacher %s", teacherInfo.FirstName)
		// Log error but do not return an error for the main operation, as status update was successful.
	}

	// A "No" response means the current report is not done. Do not proceed to the next question.
	return nil
}

func (s *NotificationServiceImpl) ProcessScheduled1HourReminders(ctx context.Context) error {
	logCtx := s.log.WithField("operation", "ProcessScheduled1HourReminders")
	logCtx.Info("Processing scheduled 1-hour reminders...")
	now := time.Now()

	dueStatuses, err := s.notifRepo.ListDueReminders(ctx, notification.StatusAwaitingReminder1H, now)
	if err != nil {
		logCtx.WithError(err).Error("Failed to list due 1-hour reminders")
		return fmt.Errorf("failed to list due 1-hour reminders: %w", err)
	}

	if len(dueStatuses) == 0 {
		logCtx.Info("No 1-hour reminders due at this time.")
		return nil
	}
	logCtx.WithField("due_statuses_count", len(dueStatuses)).Info("Found status(es) needing a 1-hour reminder.")

	for _, rs := range dueStatuses {
		reminderLogCtx := logCtx.WithFields(logrus.Fields{
			"report_status_id": rs.ID,
			"teacher_id":       rs.TeacherID,
			"cycle_id":         rs.CycleID,
			"report_key":       rs.ReportKey,
		})
		reminderLogCtx.Info("Processing 1-hour reminder")

		teacherInfo, err := s.teacherRepo.GetByID(ctx, rs.TeacherID)
		if err != nil {
			reminderLogCtx.WithError(err).Error("Failed to get teacher for 1-hour reminder")
			continue // Skip this reminder
		}

		// Re-send the specific question. This function also updates LastNotifiedAt and sets status to StatusPendingQuestion.
		err = s.sendSpecificReportQuestion(ctx, teacherInfo, rs.CycleID, rs.ReportKey)
		if err != nil {
			reminderLogCtx.WithError(err).Error("Failed to send 1-hour reminder (re-ask question)")
			// If sendSpecificReportQuestion fails, the status in DB should still be AWAITING_REMINDER_1H
			// and RemindAt should still be set, so it will be picked up next time.
			continue
		}

		// After successfully sending the reminder, clear the RemindAt timestamp
		// Fetch the latest status again as sendSpecificReportQuestion modified it.
		updatedRs, fetchErr := s.notifRepo.GetReportStatusByID(ctx, rs.ID)
		if fetchErr != nil {
			reminderLogCtx.WithError(fetchErr).Error("Failed to re-fetch ReportStatusID after sending 1-hour reminder. RemindAt might not be cleared.")
			continue
		}

		updatedRs.RemindAt = sql.NullTime{Valid: false} // Clear the reminder time
		// The status is already PENDING_QUESTION due to sendSpecificReportQuestion.
		// We are just ensuring RemindAt is cleared.
		if errUpdate := s.notifRepo.UpdateReportStatus(ctx, updatedRs); errUpdate != nil {
			reminderLogCtx.WithError(errUpdate).Error("Failed to update ReportStatusID to clear RemindAt after 1-hour reminder")
		} else {
			reminderLogCtx.WithField("new_status", updatedRs.Status).Info("Successfully sent 1-hour reminder and updated status. RemindAt cleared.")
		}
	}
	return nil
}

func (s *NotificationServiceImpl) ProcessNextDayReminders(ctx context.Context) error {
	logCtx := s.log.WithField("operation", "ProcessNextDayReminders")
	logCtx.Info("Processing scheduled next-day reminders...")

	now := time.Now()
	// Define "previous day" range precisely, considering server's local timezone for consistency with cron.
	// Location should match the cron job's location.
	loc := time.Local // Or a specific configured timezone
	year, month, day := now.Date()
	startOfToday := time.Date(year, month, day, 0, 0, 0, 0, loc) // Today 00:00:00
	endOfPreviousDay := startOfToday.Add(-1 * time.Nanosecond)   // Yesterday 23:59:59.999...
	startOfPreviousDay := startOfToday.AddDate(0, 0, -1)         // Yesterday 00:00:00

	logCtx.WithFields(logrus.Fields{
		"check_range_start": startOfPreviousDay.Format(time.RFC3339),
		"check_range_end":   endOfPreviousDay.Format(time.RFC3339),
	}).Info("Checking for stalled statuses from previous day")

	statusesToConsider := []notification.InteractionStatus{
		notification.StatusPendingQuestion,
		notification.StatusAwaitingReminder1H,
	}

	stalledStatuses, err := s.notifRepo.ListStalledStatusesFromPreviousDay(ctx, statusesToConsider, startOfPreviousDay, endOfPreviousDay)
	if err != nil {
		logCtx.WithError(err).Error("Failed to list stalled statuses for next-day reminder")
		return fmt.Errorf("failed to list stalled statuses: %w", err)
	}

	if len(stalledStatuses) == 0 {
		logCtx.Info("No statuses found needing a next-day reminder.")
		return nil
	}
	logCtx.WithField("stalled_statuses_count", len(stalledStatuses)).Info("Found status(es) needing a next-day reminder.")

	for _, rs := range stalledStatuses {
		reminderLogCtx := logCtx.WithFields(logrus.Fields{
			"report_status_id": rs.ID,
			"teacher_id":       rs.TeacherID,
			"cycle_id":         rs.CycleID,
			"report_key":       rs.ReportKey,
			"current_status":   rs.Status,
		})
		reminderLogCtx.Info("Processing next-day reminder")

		teacherInfo, err := s.teacherRepo.GetByID(ctx, rs.TeacherID)
		if err != nil {
			reminderLogCtx.WithError(err).Error("Failed to get teacher for next-day reminder")
			continue // Skip this reminder
		}

		// Update status before sending to prevent re-processing if send fails temporarily
		rs.Status = notification.StatusNextDayReminderSent
		rs.ResponseAttempts++                    // Increment response attempts
		rs.RemindAt = sql.NullTime{Valid: false} // Clear any existing reminder time
		rs.UpdatedAt = time.Now()

		// Re-send the specific question
		if err := s.sendSpecificReportQuestion(ctx, teacherInfo, rs.CycleID, rs.ReportKey); err != nil {
			reminderLogCtx.WithError(err).Error("Failed to send next-day reminder")
			// If send fails, status is already NEXT_DAY_REMINDER_SENT in memory.
			// We update the DB status to NEXT_DAY_REMINDER_SENT to record the attempt.
			// LastNotifiedAt would not be updated by sendSpecificReportQuestion.
			rs.Status = notification.StatusNextDayReminderSent // Ensure this is the final status for this attempt
			if errUpdate := s.notifRepo.UpdateReportStatus(ctx, rs); errUpdate != nil {
				reminderLogCtx.WithError(errUpdate).Error("Failed to update ReportStatusID after FAILED next-day reminder send attempt")
			}

		} else {
			// sendSpecificReportQuestion on success would have updated rs.LastNotifiedAt (via its own UpdateReportStatus call for that status).
			// Now, we ensure the status is NEXT_DAY_REMINDER_SENT.
			// Fetch the latest version of rs as sendSpecificReportQuestion might have updated it (especially LastNotifiedAt).
			updatedRs, fetchErr := s.notifRepo.GetReportStatusByID(ctx, rs.ID)
			if fetchErr != nil {
				reminderLogCtx.WithError(fetchErr).Error("Failed to re-fetch ReportStatusID after successful next-day reminder send")
				updatedRs = rs // Fallback to original rs, LastNotifiedAt might be stale from this rs instance.
			}
			updatedRs.Status = notification.StatusNextDayReminderSent // Final status for this path
			updatedRs.ResponseAttempts = rs.ResponseAttempts          // Preserve incremented attempts from the in-memory rs

			if errUpdate := s.notifRepo.UpdateReportStatus(ctx, updatedRs); errUpdate != nil {
				reminderLogCtx.WithError(errUpdate).Error("Failed to update ReportStatusID after successful next-day reminder")
			} else {
				reminderLogCtx.Info("Successfully sent and updated status for next-day reminder")
			}
		}
	}
	return nil
}
