// internal/domain/notification/repository.go
package notification

import (
	"context"
	"time"
)

// Repository defines operations for NotificationCycle and TeacherReportStatus.
type Repository interface {
	// NotificationCycle methods
	CreateCycle(ctx context.Context, cycle *Cycle) error
	GetCycleByID(ctx context.Context, id int32) (*Cycle, error)
	GetCycleByDateAndType(ctx context.Context, cycleDate time.Time, cycleType CycleType) (*Cycle, error)

	// TeacherReportStatus methods
	CreateReportStatus(ctx context.Context, rs *ReportStatus) error
	BulkCreateReportStatuses(ctx context.Context, statuses []*ReportStatus) error // For initializing a cycle
	UpdateReportStatus(ctx context.Context, rs *ReportStatus) error
	GetReportStatus(ctx context.Context, teacherID int64, cycleID int32, reportKey ReportKey) (*ReportStatus, error)
	GetReportStatusByID(ctx context.Context, id int64) (*ReportStatus, error) // Useful for direct updates from reminders
	ListReportStatusesByCycleAndTeacher(ctx context.Context, cycleID int32, teacherID int64) ([]*ReportStatus, error)
	ListReportStatusesByCycle(ctx context.Context, cycleID int32) ([]*ReportStatus, error) // For admin/overview
	ListReportStatusesByStatusAndCycle(ctx context.Context, cycleID int32, status InteractionStatus) ([]*ReportStatus, error)
	ListReportStatusesForReminders(ctx context.Context, cycleID int32, status InteractionStatus, notifiedBefore time.Time) ([]*ReportStatus, error)

	// AreAllReportsConfirmedForTeacher checks if a teacher has confirmed all required reports for a cycle.
	// expectedReportKeys are the keys relevant for the given cycle type.
	AreAllReportsConfirmedForTeacher(ctx context.Context, teacherID int64, cycleID int32, expectedReportKeys []ReportKey) (bool, error)
	// ListDueReminders fetches report statuses that are due for a reminder.
	ListDueReminders(ctx context.Context, targetStatus InteractionStatus, remindAtOrBefore time.Time) ([]*ReportStatus, error)
}
