// internal/app/notification_service.go
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"teacher_notification_bot/internal/domain/notification" // Adjust import path
	"teacher_notification_bot/internal/domain/teacher"
	domainTelegram "teacher_notification_bot/internal/domain/telegram" // Import from domain
	idb "teacher_notification_bot/internal/infra/database"             // Alias for your DB errors
	"time"

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
}

// NotificationServiceImpl implements the NotificationService interface.
type NotificationServiceImpl struct {
	teacherRepo       teacher.Repository
	notifRepo         notification.Repository
	telegramClient    domainTelegram.Client // Use the interface from the domain package
	logger            *log.Logger
	managerTelegramID int64 // Added
}

func NewNotificationServiceImpl(
	tr teacher.Repository,
	nr notification.Repository,
	tc domainTelegram.Client, // Use the interface from the domain package
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

// InitiateNotificationProcess starts the notification workflow.
func (s *NotificationServiceImpl) InitiateNotificationProcess(ctx context.Context, cycleType notification.CycleType, cycleDate time.Time) error {
	s.logger.Printf("INFO: Initiating notification process for CycleType: %s, Date: %s", cycleType, cycleDate.Format("2006-01-02"))

	// 1. Find or Create NotificationCycle
	currentCycle, err := s.notifRepo.GetCycleByDateAndType(ctx, cycleDate, cycleType)
	if err != nil {
		if err == idb.ErrCycleNotFound {
			s.logger.Printf("INFO: No existing cycle for %s on %s. Creating new cycle.", cycleType, cycleDate.Format("2006-01-02"))
			newCycle := &notification.Cycle{ // Create as a pointer
				CycleDate: cycleDate,
				Type:      cycleType,
			}
			if err := s.notifRepo.CreateCycle(ctx, newCycle); err != nil {
				s.logger.Printf("ERROR: Failed to create notification cycle: %v", err)
				return fmt.Errorf("failed to create notification cycle: %w", err)
			}
			currentCycle = newCycle // Assign the pointer
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
	now := time.Now() // Use a consistent time for this batch of operations
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
				LastNotifiedAt:   sql.NullTime{}, // Will be set after successful send for the specific notification
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
	firstReportKey := notification.ReportKeyTable1Lessons // Always start with Table 1
	for _, t := range activeTeachers {
		reportStatus, err := s.notifRepo.GetReportStatus(ctx, t.ID, currentCycle.ID, firstReportKey)
		if err != nil {
			s.logger.Printf("ERROR: Could not fetch report status for TeacherID %d, ReportKey %s for sending initial notification: %v", t.ID, firstReportKey, err)
			continue // Skip this teacher if their initial status record is missing
		}

		if reportStatus.Status != notification.StatusPendingQuestion {
			s.logger.Printf("INFO: Initial notification for TeacherID %d, ReportKey %s skipped, status is %s.", t.ID, firstReportKey, reportStatus.Status)
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
			s.logger.Printf("ERROR: Failed to send initial notification for Table 1 to Teacher %s (TG_ID: %d): %v", teacherName, t.TelegramID, err)
		} else {
			s.logger.Printf("INFO: Successfully sent initial notification for Table 1 to Teacher %s (TG_ID: %d)", teacherName, t.TelegramID)
			reportStatus.LastNotifiedAt = sql.NullTime{Time: now, Valid: true} // Use the 'now' from the beginning of status processing for this batch
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
	allExpectedReportsForCycle := determineReportsForCycle(currentCycle.Type)

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
			return err
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
// currentAnsweredKey is passed for context but the simpler logic iterates all keys.
func (s *NotificationServiceImpl) determineNextReportKey(ctx context.Context, teacherID int64, cycleID int32, _ notification.ReportKey, allCycleKeys []notification.ReportKey) (notification.ReportKey, error) {
	for _, key := range allCycleKeys {
		reportStatus, err := s.notifRepo.GetReportStatus(ctx, teacherID, cycleID, key)
		if err != nil {
			if err == idb.ErrReportStatusNotFound {
				s.logger.Printf("WARN: Report status for TeacherID %d, CycleID %d, Key %s not found. This might be an issue or an uninitialized report. Treating as PENDING.", teacherID, cycleID, key)
				// This implies the record should have been created in InitiateNotificationProcess.
				// If it's missing, it's effectively pending.
				return key, nil
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
	case notification.ReportKeyTable1Lessons:
		questionText = "Заполнена ли Таблица 1: Проведенные уроки (отчёт за текущий период)?"
	case notification.ReportKeyTable3Schedule:
		questionText = "Отлично! Заполнена ли Таблица 3: Расписание (проверка актуальности)?"
	case notification.ReportKeyTable2OTV:
		questionText = "Супер! Заполнена ли Таблица 2: Таблица ОТВ (все проведенные уроки за всё время)?"
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
	reportStatus.Status = notification.StatusPendingQuestion // Ensure it's marked as pending
	if errUpdate := s.notifRepo.UpdateReportStatus(ctx, reportStatus); errUpdate != nil {
		s.logger.Printf("ERROR: Failed to update LastNotifiedAt/Status for ReportStatusID %d after sending question: %v", reportStatus.ID, errUpdate)
	}
	return nil
}

// sendManagerConfirmationAndTeacherFinalReply handles the final messages.
func (s *NotificationServiceImpl) sendManagerConfirmationAndTeacherFinalReply(ctx context.Context, teacherInfo *teacher.Teacher, cycleInfo *notification.Cycle) error {
	if s.managerTelegramID != 0 {
		teacherFullName := teacherInfo.FirstName
		if teacherInfo.LastName.Valid {
			teacherFullName += " " + teacherInfo.LastName.String
		}
		managerMessage := fmt.Sprintf("Преподаватель %s подтвердил заполнение всех таблиц. Можно выплачивать ЗП.", teacherFullName)

		err := s.telegramClient.SendMessage(s.managerTelegramID, managerMessage, nil)
		if err != nil {
			s.logger.Printf("ERROR: Failed to send confirmation to manager (ID: %d) for teacher %s: %v", s.managerTelegramID, teacherFullName, err)
		} else {
			s.logger.Printf("INFO: Confirmation sent to manager (ID: %d) for teacher %s.", s.managerTelegramID, teacherFullName)
		}
	} else {
		s.logger.Printf("WARN: Manager Telegram ID not configured in NotificationService. Cannot send manager confirmation.")
	}

	teacherReplyMessage := "Спасибо! Все таблицы подтверждены."
	err := s.telegramClient.SendMessage(teacherInfo.TelegramID, teacherReplyMessage, nil)
	if err != nil {
		s.logger.Printf("ERROR: Failed to send final confirmation to teacher %s (TG_ID: %d): %v", teacherInfo.FirstName, teacherInfo.TelegramID, err)
		return fmt.Errorf("failed to send final reply to teacher: %w", err)
	}
	s.logger.Printf("INFO: Final confirmation sent to teacher %s (TG_ID: %d).", teacherInfo.FirstName, teacherInfo.TelegramID)
	return nil
}

func (s *NotificationServiceImpl) ProcessTeacherNoResponse(ctx context.Context, reportStatusID int64) error {
	s.logger.Printf("INFO: Processing 'No' response for ReportStatusID: %d", reportStatusID)

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

	// If already answered 'No', to prevent reprocessing (e.g. double clicks)
	if currentReportStatus.Status == notification.StatusAnsweredNo {
		s.logger.Printf("INFO: ReportStatusID %d already marked as ANSWERED_NO. No action needed.", reportStatusID)
		return nil
	}

	// 1b. Update Status
	currentReportStatus.Status = notification.StatusAnsweredNo
	currentReportStatus.UpdatedAt = time.Now() // Service layer can set this before repo call
	if err := s.notifRepo.UpdateReportStatus(ctx, currentReportStatus); err != nil {
		s.logger.Printf("ERROR: Failed to update report status ID %d to ANSWERED_NO: %v", reportStatusID, err)
		return fmt.Errorf("failed to update report status ID %d: %w", reportStatusID, err)
	}
	s.logger.Printf("INFO: ReportStatusID %d updated to ANSWERED_NO.", reportStatusID)

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
	allExpectedReportsForCycle := determineReportsForCycle(currentCycle.Type)

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
			return err
		}

		if nextReportKey == "" { // Should be caught by allConfirmed, but as a safeguard
			s.logger.Printf("WARN: All reports appeared confirmed, but determineNextReportKey found no next key for TeacherID %d, CycleID %d. Finalizing.", teacherInfo.ID, currentCycle.ID)
			return s.sendManagerConfirmationAndTeacherFinalReply(ctx, teacherInfo, currentCycle)
		}

		s.logger.Printf("INFO: Next report for TeacherID %d in CycleID %d is %s.", teacherInfo.ID, currentCycle.ID, nextReportKey)
		return s.sendSpecificReportQuestion(ctx, teacherInfo, currentCycle.ID, nextReportKey)
	}
}

func (s *NotificationServiceImpl) ProcessScheduled1HourReminders(ctx context.Context) error {
	s.logger.Println("INFO: Processing scheduled 1-hour reminders...")
	// ... existing code ...
	return nil
}
