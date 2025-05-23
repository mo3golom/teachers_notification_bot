package app

import (
	"context"
	"database/sql"
	"fmt"
	"teacher_notification_bot/internal/domain/teacher"
	idb "teacher_notification_bot/internal/infra/database" // Assuming custom errors like ErrTeacherNotFound are here
)

// Custom application-level errors for admin service
var ErrAdminNotAuthorized = fmt.Errorf("performing user is not authorized as an admin")
var ErrTeacherAlreadyExists = fmt.Errorf("teacher with this Telegram ID already exists")
var ErrTeacherAlreadyInactive = fmt.Errorf("teacher is already inactive")

type AdminService struct {
	teacherRepo     teacher.Repository
	adminTelegramID int64
}

func NewAdminService(tr teacher.Repository, adminID int64) *AdminService {
	return &AdminService{
		teacherRepo:     tr,
		adminTelegramID: adminID,
	}
}

// AddTeacher handles the business logic for adding a new teacher.
func (s *AdminService) AddTeacher(ctx context.Context, performingAdminID int64, newTeacherTelegramID int64, firstName string, lastNameValue string) (*teacher.Teacher, error) {
	if performingAdminID != s.adminTelegramID {
		return nil, ErrAdminNotAuthorized
	}

	// Check if teacher already exists by Telegram ID
	_, err := s.teacherRepo.GetByTelegramID(ctx, newTeacherTelegramID)
	if err == nil { // Teacher found, so already exists
		return nil, ErrTeacherAlreadyExists
	}
	if err != idb.ErrTeacherNotFound { // Another error occurred during lookup
		return nil, fmt.Errorf("failed to check existing teacher: %w", err)
	}

	// Prepare LastName
	var lastName sql.NullString
	if lastNameValue != "" {
		lastName.String = lastNameValue
		lastName.Valid = true
	}

	newTeacher := &teacher.Teacher{
		TelegramID: newTeacherTelegramID,
		FirstName:  firstName,
		LastName:   lastName,
		IsActive:   true, // New teachers are active by default
	}

	err = s.teacherRepo.Create(ctx, newTeacher)
	if err != nil {
		if err == idb.ErrDuplicateTelegramID { // Redundant check if GetByTelegramID is perfect, but good for safety
			return nil, ErrTeacherAlreadyExists
		}
		return nil, fmt.Errorf("failed to create teacher in repository: %w", err)
	}

	return newTeacher, nil
}

// RemoveTeacher handles the business logic for deactivating a teacher.
func (s *AdminService) RemoveTeacher(ctx context.Context, performingAdminID int64, teacherTelegramIDToRemove int64) (*teacher.Teacher, error) {
	if performingAdminID != s.adminTelegramID {
		return nil, ErrAdminNotAuthorized
	}

	// Fetch the teacher by Telegram ID
	targetTeacher, err := s.teacherRepo.GetByTelegramID(ctx, teacherTelegramIDToRemove)
	if err != nil {
		if err == idb.ErrTeacherNotFound { // idb is alias for internal/infra/database
			return nil, idb.ErrTeacherNotFound // Propagate specific error
		}
		return nil, fmt.Errorf("failed to get teacher by Telegram ID for removal: %w", err)
	}

	// Check if already inactive
	if !targetTeacher.IsActive {
		return targetTeacher, ErrTeacherAlreadyInactive
	}

	// Deactivate the teacher
	targetTeacher.IsActive = false
	err = s.teacherRepo.Update(ctx, targetTeacher)
	if err != nil {
		return nil, fmt.Errorf("failed to update teacher to inactive in repository: %w", err)
	}

	return targetTeacher, nil
}
