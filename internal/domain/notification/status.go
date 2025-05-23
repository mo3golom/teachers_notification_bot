// internal/domain/notification/status.go
package notification

import (
	"database/sql"
	"time"
	// "teacher_notification_bot/internal/domain/teacher" // If you need to embed Teacher struct later
)

// ReportStatus tracks the status of a specific report query for a teacher within a cycle.
// Corresponds to the 'teacher_report_statuses' table in schema B003.
type ReportStatus struct {
	ID               int64
	TeacherID        int64             // Foreign Key to teachers.id
	CycleID          int32             // Foreign Key to notification_cycles.id
	ReportKey        ReportKey         // e.g., TABLE_1_LESSONS
	Status           InteractionStatus // e.g., PENDING_QUESTION, ANSWERED_YES
	LastNotifiedAt   sql.NullTime      // When the last notification/reminder for this item was sent
	ResponseAttempts int               // Number of reminders or "No" responses for this item
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
