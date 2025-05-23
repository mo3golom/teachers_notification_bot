// internal/app/notification_service.go
package app

import (
	"context"
	"log"
	"teacher_notification_bot/internal/domain/notification" // Adjust import path
	"time"
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
	m.logger.Printf("INFO (MockNotificationService): InitiateNotificationProcess called for CycleType: %s, Date: %s", cycleType, cycleDate.Format("2006-01-02"))
	return nil
}
