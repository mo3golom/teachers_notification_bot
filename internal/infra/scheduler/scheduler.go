package scheduler

import (
	"context"
	"log"
	"teacher_notification_bot/internal/app" // For NotificationService interface
	"teacher_notification_bot/internal/domain/notification"
	idb "teacher_notification_bot/internal/infra/database" // For ErrCycleNotFound
	"time"

	"github.com/robfig/cron/v3"
)

type NotificationScheduler struct {
	cronEngine            *cron.Cron
	notifService          app.NotificationService // Using the interface
	notifRepo             notification.Repository
	logger                *log.Logger
	cronSpec15th          string
	cronSpecLastDay       string // This will run daily, logic inside checks if it's the last day
	cronSpecReminderCheck string
	cronSpecNextDayCheck  string
}

func NewNotificationScheduler(
	notifService app.NotificationService,
	notifRepo notification.Repository,
	logger *log.Logger,
	cronSpec15th string, // e.g., "0 10 15 * *" (10:00 AM on 15th)
	cronSpecDailyCheckForLastDay string, // e.g., "0 10 * * *" (10:00 AM daily)
	cronSpecReminderCheck string, // e.g., "*/5 * * * *" (every 5 minutes)
	cronSpecNextDayCheck string, // e.g., "0 9 * * *" (9 AM daily)
) *NotificationScheduler {
	return &NotificationScheduler{
		cronEngine:            cron.New(cron.WithLocation(time.Local)), // Use server's local time for cron
		notifService:          notifService,
		notifRepo:             notifRepo,
		logger:                logger,
		cronSpec15th:          cronSpec15th,
		cronSpecLastDay:       cronSpecDailyCheckForLastDay,
		cronSpecReminderCheck: cronSpecReminderCheck,
		cronSpecNextDayCheck:  cronSpecNextDayCheck,
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

	// Job for processing 1-hour reminders
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

	// Job for processing next-day reminders
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

	s.cronEngine.Start()
	s.logger.Println("INFO: Notification scheduler started with jobs.")
}

// executeNotificationProcess is a helper to handle the common logic for both job types
func (s *NotificationScheduler) executeNotificationProcess(cycleType notification.CycleType) {
	ctx := context.Background() // Or a more specific context if available
	today := time.Now()
	// Normalize to just the date part for cycleDate consistency
	cycleDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	existingCycle, err := s.notifRepo.GetCycleByDateAndType(ctx, cycleDate, cycleType)
	if err != nil && err != idb.ErrCycleNotFound {
		s.logger.Printf("ERROR: Failed to check for existing notification cycle: %v", err)
		return
	}
	if existingCycle != nil {
		s.logger.Printf("INFO: Notification cycle for date %s and type %s already exists (ID: %d). Skipping creation.",
			cycleDate.Format("2006-01-02"), cycleType, existingCycle.ID)
	} else {
		s.logger.Printf("INFO: No existing cycle found for date %s and type %s. A new cycle will be implied by InitiateNotificationProcess.",
			cycleDate.Format("2006-01-02"), cycleType)
	}

	if err := s.notifService.InitiateNotificationProcess(ctx, cycleType, cycleDate); err != nil {
		s.logger.Printf("ERROR: Error during notification process for type %s: %v", cycleType, err)
	} else {
		s.logger.Printf("INFO: Notification process for type %s initiated successfully.", cycleType)
	}
}

func (s *NotificationScheduler) Stop() {
	s.logger.Println("INFO: Stopping notification scheduler...")
	ctx := s.cronEngine.Stop() // Stops the scheduler from adding new jobs, waits for running jobs.
	<-ctx.Done()               // Wait for graceful shutdown
	s.logger.Println("INFO: Notification scheduler gracefully stopped.")
}
