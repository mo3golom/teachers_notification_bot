package scheduler

import (
	"context"
	"teacher_notification_bot/internal/app" // For NotificationService interface
	"teacher_notification_bot/internal/domain/notification"
	idb "teacher_notification_bot/internal/infra/database" // For ErrCycleNotFound
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

type NotificationScheduler struct {
	cronEngine            *cron.Cron
	notifService          app.NotificationService // Using the interface
	notifRepo             notification.Repository
	log                   *logrus.Entry
	cronSpec15th          string
	cronSpecLastDay       string // This will run daily, logic inside checks if it's the last day
	cronSpecReminderCheck string
	cronSpecNextDayCheck  string
}

func NewNotificationScheduler(
	notifService app.NotificationService,
	notifRepo notification.Repository,
	baseLogger *logrus.Entry,
	cronSpec15th string, // e.g., "0 10 15 * *" (10:00 AM on 15th)
	cronSpecDailyCheckForLastDay string, // e.g., "0 10 * * *" (10:00 AM daily)
	cronSpecReminderCheck string, // e.g., "*/5 * * * *" (every 5 minutes)
	cronSpecNextDayCheck string, // e.g., "0 11 * * *" (11:00 AM daily)
) *NotificationScheduler {
	return &NotificationScheduler{
		cronEngine:            cron.New(cron.WithLocation(time.Local)), // Use server's local time for cron
		notifService:          notifService,
		notifRepo:             notifRepo,
		log:                   baseLogger,
		cronSpec15th:          cronSpec15th,
		cronSpecLastDay:       cronSpecDailyCheckForLastDay,
		cronSpecReminderCheck: cronSpecReminderCheck,
		cronSpecNextDayCheck:  cronSpecNextDayCheck,
	}
}

func (s *NotificationScheduler) Start() {
	s.log.Info("Starting notification scheduler...")

	// Job for the 15th of the month
	_, err := s.cronEngine.AddFunc(s.cronSpec15th, func() {
		jobLog := s.log.WithField("job_name", "15th_of_month_notification")
		jobLog.Info("Cron job triggered")
		s.executeNotificationProcess(jobLog, notification.CycleTypeMidMonth)
	})
	if err != nil {
		s.log.WithError(err).Fatal("Could not add 15th of month cron job")
	}

	// Job that runs daily but only proceeds if it's the last day of the month
	_, err = s.cronEngine.AddFunc(s.cronSpecLastDay, func() {
		jobLog := s.log.WithField("job_name", "last_day_of_month_check")
		jobLog.Info("Daily cron job triggered for last day check")
		now := time.Now()
		// Calculate the first day of the next month, then subtract one day to get the last day of the current month.
		firstOfNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
		lastDayOfCurrentMonth := firstOfNextMonth.AddDate(0, 0, -1)

		if now.Day() == lastDayOfCurrentMonth.Day() {
			jobLog.Info("Today is the last day of the month. Executing end-of-month notification process.")
			s.executeNotificationProcess(jobLog, notification.CycleTypeEndMonth)
		} else {
			jobLog.WithFields(logrus.Fields{"current_day": now.Day(), "last_day_of_month": lastDayOfCurrentMonth.Day()}).Info("Today is not the last day of the month. Skipping end-of-month process.")
		}
	})
	if err != nil {
		s.log.WithError(err).Fatal("Could not add last day of month cron job")
	}

	// Job for processing 1-hour reminders
	_, err = s.cronEngine.AddFunc(s.cronSpecReminderCheck, func() {
		jobLog := s.log.WithField("job_name", "1_hour_reminder_processing")
		jobLog.Info("Cron job triggered")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute) // Context for the job
		defer cancel()
		if err := s.notifService.ProcessScheduled1HourReminders(ctx); err != nil {
			jobLog.WithError(err).Error("Error during 1-hour reminder processing")
		}
	})
	if err != nil {
		s.log.WithError(err).Fatal("Could not add 1-hour reminder processing cron job")
	}

	// Job for processing next-day reminders
	_, err = s.cronEngine.AddFunc(s.cronSpecNextDayCheck, func() {
		jobLog := s.log.WithField("job_name", "next_day_reminder_processing")
		jobLog.Info("Cron job triggered")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) // Longer timeout for potentially more items
		defer cancel()
		if err := s.notifService.ProcessNextDayReminders(ctx); err != nil {
			jobLog.WithError(err).Error("Error during next-day reminder processing")
		}
	})
	if err != nil {
		s.log.WithError(err).Fatal("Could not add next-day reminder processing cron job")
	}

	s.cronEngine.Start()
	s.log.Info("Notification scheduler started with jobs.")
}

// executeNotificationProcess is a helper to handle the common logic for both job types
func (s *NotificationScheduler) executeNotificationProcess(jobLog *logrus.Entry, cycleType notification.CycleType) {
	ctx := context.Background() // Or a more specific context if available
	today := time.Now()
	// Normalize to just the date part for cycleDate consistency
	cycleDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	logCtx := jobLog.WithFields(logrus.Fields{"cycle_type": cycleType, "cycle_date": cycleDate.Format("2006-01-02")})

	existingCycle, err := s.notifRepo.GetCycleByDateAndType(ctx, cycleDate, cycleType)
	if err != nil && err != idb.ErrCycleNotFound {
		logCtx.WithError(err).Error("Failed to check for existing cycle before initiating process")
		return
	}
	if existingCycle != nil {
		logCtx.WithField("existing_cycle_id", existingCycle.ID).Info("Notification cycle already exists. Skipping creation within InitiateNotificationProcess if it checks.")
	} else {
		logCtx.Info("No existing cycle found. A new cycle will be created by InitiateNotificationProcess.")
	}

	if err := s.notifService.InitiateNotificationProcess(ctx, cycleType, cycleDate); err != nil {
		logCtx.WithError(err).Error("Error during notification process initiation")
	} else {
		logCtx.Info("Notification process initiated successfully.")
	}
}

func (s *NotificationScheduler) Stop() {
	s.log.Info("Stopping notification scheduler...")
	ctx := s.cronEngine.Stop() // Stops the scheduler from adding new jobs, waits for running jobs.
	<-ctx.Done()               // Wait for graceful shutdown
	s.log.Info("Notification scheduler gracefully stopped.")
}
