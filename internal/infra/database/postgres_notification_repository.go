// internal/infra/database/postgres_notification_repository.go
package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"teacher_notification_bot/internal/domain/notification"
	"time"

	"github.com/lib/pq" // For pq.Array and driver registration
)

// Custom errors specific to notification repository
var ErrCycleNotFound = fmt.Errorf("notification cycle not found")
var ErrReportStatusNotFound = fmt.Errorf("teacher report status not found")
var ErrDuplicateReportStatus = fmt.Errorf("duplicate teacher report status (teacher_id, cycle_id, report_key)")

type PostgresNotificationRepository struct {
	db *sql.DB
}

func NewPostgresNotificationRepository(db *sql.DB) *PostgresNotificationRepository {
	return &PostgresNotificationRepository{db: db}
}

// --- NotificationCycle Methods ---

func (r *PostgresNotificationRepository) CreateCycle(ctx context.Context, cycle *notification.Cycle) error {
	query := `INSERT INTO notification_cycles (cycle_date, cycle_type)
               VALUES ($1, $2)
               RETURNING id, created_at`
	// Ensure CycleDate is just the date part if necessary, though DATE type handles it.
	err := r.db.QueryRowContext(ctx, query, cycle.CycleDate, cycle.Type).Scan(&cycle.ID, &cycle.CreatedAt)
	if err != nil {
		// Consider specific pq error for unique constraint if any added later
		return fmt.Errorf("error creating notification cycle: %w", err)
	}
	return nil
}

func (r *PostgresNotificationRepository) GetCycleByID(ctx context.Context, id int32) (*notification.Cycle, error) {
	query := `SELECT id, cycle_date, cycle_type, created_at FROM notification_cycles WHERE id = $1`
	cycle := notification.Cycle{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(&cycle.ID, &cycle.CycleDate, &cycle.Type, &cycle.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrCycleNotFound
		}
		return nil, fmt.Errorf("error getting notification cycle by ID: %w", err)
	}
	return &cycle, nil
}

func (r *PostgresNotificationRepository) GetCycleByDateAndType(ctx context.Context, cycleDate time.Time, cycleType notification.CycleType) (*notification.Cycle, error) {
	query := `SELECT id, cycle_date, cycle_type, created_at FROM notification_cycles WHERE cycle_date = $1 AND cycle_type = $2 ORDER BY created_at DESC LIMIT 1`
	cycle := notification.Cycle{}
	// Normalize cycleDate to just date part if it contains time
	dateOnly := time.Date(cycleDate.Year(), cycleDate.Month(), cycleDate.Day(), 0, 0, 0, 0, cycleDate.Location())
	err := r.db.QueryRowContext(ctx, query, dateOnly, cycleType).Scan(&cycle.ID, &cycle.CycleDate, &cycle.Type, &cycle.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrCycleNotFound
		}
		return nil, fmt.Errorf("error getting notification cycle by date and type: %w", err)
	}
	return &cycle, nil
}

// --- TeacherReportStatus Methods ---

func (r *PostgresNotificationRepository) CreateReportStatus(ctx context.Context, rs *notification.ReportStatus) error {
	query := `INSERT INTO teacher_report_statuses (teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts)
               VALUES ($1, $2, $3, $4, $5, $6)
               RETURNING id, created_at, updated_at`
	err := r.db.QueryRowContext(ctx, query, rs.TeacherID, rs.CycleID, rs.ReportKey, rs.Status, rs.LastNotifiedAt, rs.ResponseAttempts).Scan(&rs.ID, &rs.CreatedAt, &rs.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "teacher_cycle_report_unique") { // Check for unique constraint violation
			return ErrDuplicateReportStatus
		}
		return fmt.Errorf("error creating teacher report status: %w", err)
	}
	return nil
}

func (r *PostgresNotificationRepository) BulkCreateReportStatuses(ctx context.Context, statuses []*notification.ReportStatus) error {
	if len(statuses) == 0 {
		return nil
	}

	txn, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for bulk create: %w", err)
	}
	defer txn.Rollback() // Rollback if not committed

	stmt, err := txn.PrepareContext(ctx, `INSERT INTO teacher_report_statuses (teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at)
                                         VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement for bulk create: %w", err)
	}
	defer stmt.Close()

	for _, rs := range statuses {
		_, err := stmt.ExecContext(ctx, rs.TeacherID, rs.CycleID, rs.ReportKey, rs.Status, rs.LastNotifiedAt, rs.ResponseAttempts)
		if err != nil {
			if strings.Contains(err.Error(), "teacher_cycle_report_unique") {
				// Potentially log this or decide on overall failure/partial success
				return fmt.Errorf("error in bulk create (status for T:%d, C:%d, K:%s): %w, Detail: %w", rs.TeacherID, rs.CycleID, rs.ReportKey, ErrDuplicateReportStatus, err)
			}
			return fmt.Errorf("error executing statement for bulk create (status for T:%d, C:%d, K:%s): %w", rs.TeacherID, rs.CycleID, rs.ReportKey, err)
		}
	}

	return txn.Commit()
}

func (r *PostgresNotificationRepository) UpdateReportStatus(ctx context.Context, rs *notification.ReportStatus) error {
	query := `UPDATE teacher_report_statuses
               SET status = $1, last_notified_at = $2, response_attempts = $3, updated_at = NOW()
               WHERE id = $4
               RETURNING updated_at` // updated_at also set by trigger
	err := r.db.QueryRowContext(ctx, query, rs.Status, rs.LastNotifiedAt, rs.ResponseAttempts, rs.ID).Scan(&rs.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrReportStatusNotFound
		}
		return fmt.Errorf("error updating teacher report status: %w", err)
	}
	return nil
}

func (r *PostgresNotificationRepository) GetReportStatus(ctx context.Context, teacherID int64, cycleID int32, reportKey notification.ReportKey) (*notification.ReportStatus, error) {
	query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at
               FROM teacher_report_statuses
               WHERE teacher_id = $1 AND cycle_id = $2 AND report_key = $3`
	rs := notification.ReportStatus{}
	err := r.db.QueryRowContext(ctx, query, teacherID, cycleID, reportKey).Scan(
		&rs.ID, &rs.TeacherID, &rs.CycleID, &rs.ReportKey, &rs.Status,
		&rs.LastNotifiedAt, &rs.ResponseAttempts, &rs.CreatedAt, &rs.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrReportStatusNotFound
		}
		return nil, fmt.Errorf("error getting teacher report status: %w", err)
	}
	return &rs, nil
}

func (r *PostgresNotificationRepository) GetReportStatusByID(ctx context.Context, id int64) (*notification.ReportStatus, error) {
	query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at
               FROM teacher_report_statuses WHERE id = $1`
	rs := notification.ReportStatus{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&rs.ID, &rs.TeacherID, &rs.CycleID, &rs.ReportKey, &rs.Status,
		&rs.LastNotifiedAt, &rs.ResponseAttempts, &rs.CreatedAt, &rs.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrReportStatusNotFound
		}
		return nil, fmt.Errorf("error getting teacher report status by ID: %w", err)
	}
	return &rs, nil
}

// Helper to scan multiple rows
func scanReportStatuses(rows *sql.Rows) ([]*notification.ReportStatus, error) {
	statuses := make([]*notification.ReportStatus, 0)
	for rows.Next() {
		rs := notification.ReportStatus{}
		if err := rows.Scan(
			&rs.ID, &rs.TeacherID, &rs.CycleID, &rs.ReportKey, &rs.Status,
			&rs.LastNotifiedAt, &rs.ResponseAttempts, &rs.CreatedAt, &rs.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning report status row: %w", err)
		}
		statuses = append(statuses, &rs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating report status rows: %w", err)
	}
	return statuses, nil
}

func (r *PostgresNotificationRepository) ListReportStatusesByCycleAndTeacher(ctx context.Context, cycleID int32, teacherID int64) ([]*notification.ReportStatus, error) {
	query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at
               FROM teacher_report_statuses
               WHERE cycle_id = $1 AND teacher_id = $2 ORDER BY report_key` // Order for consistent processing
	rows, err := r.db.QueryContext(ctx, query, cycleID, teacherID)
	if err != nil {
		return nil, fmt.Errorf("error querying report statuses by cycle and teacher: %w", err)
	}
	defer rows.Close()
	return scanReportStatuses(rows)
}

func (r *PostgresNotificationRepository) ListReportStatusesByCycle(ctx context.Context, cycleID int32) ([]*notification.ReportStatus, error) {
	query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at
               FROM teacher_report_statuses
               WHERE cycle_id = $1 ORDER BY teacher_id, report_key`
	rows, err := r.db.QueryContext(ctx, query, cycleID)
	if err != nil {
		return nil, fmt.Errorf("error querying report statuses by cycle: %w", err)
	}
	defer rows.Close()
	return scanReportStatuses(rows)
}

func (r *PostgresNotificationRepository) ListReportStatusesByStatusAndCycle(ctx context.Context, cycleID int32, status notification.InteractionStatus) ([]*notification.ReportStatus, error) {
	query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at
               FROM teacher_report_statuses
               WHERE cycle_id = $1 AND status = $2 ORDER BY teacher_id, report_key`
	rows, err := r.db.QueryContext(ctx, query, cycleID, status)
	if err != nil {
		return nil, fmt.Errorf("error querying report statuses by status and cycle: %w", err)
	}
	defer rows.Close()
	return scanReportStatuses(rows)
}

func (r *PostgresNotificationRepository) ListReportStatusesForReminders(ctx context.Context, cycleID int32, status notification.InteractionStatus, notifiedBefore time.Time) ([]*notification.ReportStatus, error) {
	query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at
               FROM teacher_report_statuses
               WHERE cycle_id = $1 AND status = $2 AND last_notified_at < $3 
               ORDER BY last_notified_at ASC` // Process older ones first
	rows, err := r.db.QueryContext(ctx, query, cycleID, status, notifiedBefore)
	if err != nil {
		return nil, fmt.Errorf("error querying report statuses for reminders: %w", err)
	}
	defer rows.Close()
	return scanReportStatuses(rows)
}

func (r *PostgresNotificationRepository) AreAllReportsConfirmedForTeacher(ctx context.Context, teacherID int64, cycleID int32, expectedReportKeys []notification.ReportKey) (bool, error) {
	if len(expectedReportKeys) == 0 {
		return true, nil // No reports expected, so all are "confirmed"
	}

	keysAsStrings := make([]string, len(expectedReportKeys))
	for i, k := range expectedReportKeys {
		keysAsStrings[i] = string(k)
	}

	query := `SELECT COUNT(*)
               FROM teacher_report_statuses
               WHERE teacher_id = $1
                 AND cycle_id = $2
                 AND report_key = ANY($3::varchar[])
                 AND status != $4`

	var unconfirmedCount int
	err := r.db.QueryRowContext(ctx, query, teacherID, cycleID, pq.Array(keysAsStrings), notification.StatusAnsweredYes).Scan(&unconfirmedCount)
	if err != nil {
		// COUNT(*) should always return a row. If sql.ErrNoRows occurs, it's an unexpected DB error.
		return false, fmt.Errorf("error checking all reports confirmed: %w", err)
	}

	return unconfirmedCount == 0, nil
}
