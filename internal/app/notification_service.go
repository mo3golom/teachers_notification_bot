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
}

// NotificationServiceImpl implements the NotificationService interface.
type NotificationServiceImpl struct {
	teacherRepo    teacher.Repository
	notifRepo      notification.Repository
	telegramClient domainTelegram.Client // Use the interface from the domain package
	logger         *log.Logger
}

func NewNotificationServiceImpl(
	tr teacher.Repository,
	nr notification.Repository,
	tc domainTelegram.Client, // Use the interface from the domain package
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
