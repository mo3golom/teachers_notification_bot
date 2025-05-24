package database

import (
	"context"
	"database/sql"
	"fmt" // For error wrapping
	"strings"

	"teacher_notification_bot/internal/domain/teacher" // Adjust import path

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Custom errors
var ErrTeacherNotFound = fmt.Errorf("teacher not found")
var ErrDuplicateTelegramID = fmt.Errorf("teacher with this Telegram ID already exists")

type PostgresTeacherRepository struct {
	db *sql.DB
}

func NewPostgresTeacherRepository(db *sql.DB) *PostgresTeacherRepository {
	return &PostgresTeacherRepository{db: db}
}

func (r *PostgresTeacherRepository) Create(ctx context.Context, t *teacher.Teacher) error {
	query := `INSERT INTO teachers (telegram_id, first_name, last_name, is_active)
               VALUES ($1, $2, $3, $4)
               RETURNING id, created_at, updated_at`

	// Ensure IsActive is set, default to true if not explicitly provided for a new teacher.
	// For this Create method, IsActive is typically true.
	if !t.IsActive { // If creating, default to active unless specified.
		// However, the table has DEFAULT TRUE, so this might not be needed if t.IsActive is always set before calling.
		// For clarity, let's assume t.IsActive is set by the caller (e.g. application service).
	}

	err := r.db.QueryRowContext(ctx, query, t.TelegramID, t.FirstName, t.LastName, t.IsActive).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		// Basic check for unique violation on telegram_id.
		// More robust check might involve specific pq error codes.
		if strings.Contains(err.Error(), "unique constraint") && strings.Contains(err.Error(), "teachers_telegram_id_key") { // Example check
			return ErrDuplicateTelegramID
		}
		return fmt.Errorf("error creating teacher: %w", err)
	}
	return nil
}

func (r *PostgresTeacherRepository) GetByID(ctx context.Context, id int64) (*teacher.Teacher, error) {
	query := `SELECT id, telegram_id, first_name, last_name, is_active, created_at, updated_at
               FROM teachers WHERE id = $1`
	t := &teacher.Teacher{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(&t.ID, &t.TelegramID, &t.FirstName, &t.LastName, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTeacherNotFound
		}
		return nil, fmt.Errorf("error getting teacher by ID: %w", err)
	}
	return t, nil
}

func (r *PostgresTeacherRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*teacher.Teacher, error) {
	query := `SELECT id, telegram_id, first_name, last_name, is_active, created_at, updated_at
               FROM teachers WHERE telegram_id = $1`
	t := &teacher.Teacher{}
	err := r.db.QueryRowContext(ctx, query, telegramID).Scan(&t.ID, &t.TelegramID, &t.FirstName, &t.LastName, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTeacherNotFound
		}
		return nil, fmt.Errorf("error getting teacher by Telegram ID: %w", err)
	}
	return t, nil
}

func (r *PostgresTeacherRepository) Update(ctx context.Context, t *teacher.Teacher) error {
	query := `UPDATE teachers
               SET first_name = $1, last_name = $2, is_active = $3, updated_at = NOW()
               WHERE id = $4
               RETURNING updated_at` // updated_at is handled by trigger too, but RETURNING ensures we get the value

	err := r.db.QueryRowContext(ctx, query, t.FirstName, t.LastName, t.IsActive, t.ID).Scan(&t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows { // Should not happen if ID is valid, but good check
			return ErrTeacherNotFound
		}
		return fmt.Errorf("error updating teacher: %w", err)
	}
	return nil
}

func (r *PostgresTeacherRepository) ListActive(ctx context.Context) ([]*teacher.Teacher, error) {
	query := `SELECT id, telegram_id, first_name, last_name, is_active, created_at, updated_at
               FROM teachers WHERE is_active = TRUE ORDER BY first_name, last_name`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error listing active teachers: %w", err)
	}
	defer rows.Close()

	teachers := make([]*teacher.Teacher, 0)
	for rows.Next() {
		t := &teacher.Teacher{}
		if err := rows.Scan(&t.ID, &t.TelegramID, &t.FirstName, &t.LastName, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("error scanning active teacher: %w", err)
		}
		teachers = append(teachers, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating active teachers: %w", err)
	}
	return teachers, nil
}

func (r *PostgresTeacherRepository) ListAll(ctx context.Context) ([]*teacher.Teacher, error) {
	query := `SELECT id, telegram_id, first_name, last_name, is_active, created_at, updated_at
               FROM teachers ORDER BY id`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error listing all teachers: %w", err)
	}
	defer rows.Close()

	teachers := make([]*teacher.Teacher, 0)
	for rows.Next() {
		t := &teacher.Teacher{}
		if err := rows.Scan(&t.ID, &t.TelegramID, &t.FirstName, &t.LastName, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("error scanning teacher from all list: %w", err)
		}
		teachers = append(teachers, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating all teachers: %w", err)
	}
	return teachers, nil
}
