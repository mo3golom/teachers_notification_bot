package app

import (
	"context"
	"database/sql"
	"fmt"
	"teacher_notification_bot/internal/domain/teacher"
	idb "teacher_notification_bot/internal/infra/database"

	"github.com/sirupsen/logrus"
)

// Custom application-level errors for admin service
var (
	ErrAdminNotAuthorized     = fmt.Errorf("admin not authorized")
	ErrTeacherAlreadyExists   = fmt.Errorf("teacher with this telegram ID already exists")
	ErrTeacherAlreadyInactive = fmt.Errorf("teacher is already inactive")
)

type AdminService struct {
	teacherRepo     teacher.Repository
	adminTelegramID int64
	log             *logrus.Entry
}

func NewAdminService(tr teacher.Repository, adminID int64, baseLogger *logrus.Entry) *AdminService {
	return &AdminService{
		teacherRepo:     tr,
		adminTelegramID: adminID,
		log:             baseLogger,
	}
}

// AddTeacher handles the business logic for adding a new teacher.
func (s *AdminService) AddTeacher(ctx context.Context, performingAdminID int64, newTeacherTelegramID int64, firstName string, lastNameValue string) (*teacher.Teacher, error) {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":             "AddTeacher",
		"performing_admin_id":   performingAdminID,
		"new_teacher_tg_id":     newTeacherTelegramID,
		"new_teacher_firstname": firstName,
		"new_teacher_lastname":  lastNameValue,
	})
	logCtx.Info("Attempting to add teacher")

	if performingAdminID != s.adminTelegramID {
		logCtx.Warn("Unauthorized attempt to add teacher")
		return nil, ErrAdminNotAuthorized
	}

	// Check if teacher already exists by Telegram ID
	_, err := s.teacherRepo.GetByTelegramID(ctx, newTeacherTelegramID)
	if err == nil { // Teacher found, so already exists
		logCtx.Warn("Teacher with this Telegram ID already exists")
		return nil, ErrTeacherAlreadyExists
	}
	if err != idb.ErrTeacherNotFound { // Another error occurred during lookup
		logCtx.WithError(err).Error("Error checking for existing teacher by Telegram ID")
		return nil, fmt.Errorf("error checking for existing teacher: %w", err)
	}

	// Create new teacher entity
	newTeacher := &teacher.Teacher{
		TelegramID: newTeacherTelegramID,
		FirstName:  firstName,
		LastName: sql.NullString{
			String: lastNameValue,
			Valid:  lastNameValue != "",
		},
		IsActive: true, // New teachers are active by default
	}

	// Persist to database
	err = s.teacherRepo.Create(ctx, newTeacher)
	if err != nil {
		if err == idb.ErrDuplicateTelegramID { // Redundant check if GetByTelegramID is perfect, but good for safety
			logCtx.WithError(err).Warn("Teacher with this Telegram ID already exists (duplicate on create)")
			return nil, ErrTeacherAlreadyExists
		}
		logCtx.WithError(err).Error("Failed to create teacher in repository")
		return nil, fmt.Errorf("failed to create teacher in repository: %w", err)
	}

	logCtx.WithFields(logrus.Fields{
		"teacher_id":        newTeacher.ID,
		"teacher_tg_id":     newTeacher.TelegramID,
		"teacher_is_active": newTeacher.IsActive,
	}).Info("Teacher added successfully")
	return newTeacher, nil
}

// RemoveTeacher handles the business logic for deactivating a teacher.
func (s *AdminService) RemoveTeacher(ctx context.Context, performingAdminID int64, teacherTelegramIDToRemove int64) (*teacher.Teacher, error) {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":               "RemoveTeacher",
		"performing_admin_id":     performingAdminID,
		"teacher_tg_id_to_remove": teacherTelegramIDToRemove,
	})
	logCtx.Info("Attempting to remove (deactivate) teacher")
	if performingAdminID != s.adminTelegramID {
		logCtx.Warn("Unauthorized attempt to remove teacher")
		return nil, ErrAdminNotAuthorized
	}

	// Find the teacher by Telegram ID
	targetTeacher, err := s.teacherRepo.GetByTelegramID(ctx, teacherTelegramIDToRemove)
	if err != nil {
		if err == idb.ErrTeacherNotFound { // idb is alias for internal/infra/database
			logCtx.Warn("Teacher to remove not found by Telegram ID")
			return nil, idb.ErrTeacherNotFound // Propagate specific error
		}
		logCtx.WithError(err).Error("Failed to get teacher by Telegram ID for removal")
		return nil, fmt.Errorf("failed to get teacher by Telegram ID for removal: %w", err)
	}

	// Check if already inactive
	if !targetTeacher.IsActive {
		logCtx.WithField("teacher_id", targetTeacher.ID).Warn("Teacher is already inactive")
		return targetTeacher, ErrTeacherAlreadyInactive
	}

	// Deactivate the teacher
	targetTeacher.IsActive = false
	err = s.teacherRepo.Update(ctx, targetTeacher)
	if err != nil {
		logCtx.WithError(err).WithField("teacher_id", targetTeacher.ID).Error("Failed to update teacher to inactive in repository")
		return nil, fmt.Errorf("failed to update teacher to inactive in repository: %w", err)
	}

	logCtx.WithFields(logrus.Fields{
		"teacher_id":    targetTeacher.ID,
		"teacher_tg_id": targetTeacher.TelegramID,
	}).Info("Teacher removed (deactivated) successfully")
	return targetTeacher, nil
}

// ListAllTeachers retrieves all teachers from the repository.
// It ensures the action is performed by an authorized admin.
func (s *AdminService) ListAllTeachers(ctx context.Context, performingAdminID int64) ([]*teacher.Teacher, error) {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":           "ListAllTeachers",
		"performing_admin_id": performingAdminID,
	})
	logCtx.Info("Attempting to list all teachers")
	if performingAdminID != s.adminTelegramID {
		logCtx.Warn("Unauthorized attempt to list all teachers")
		return nil, ErrAdminNotAuthorized
	}
	teachers, err := s.teacherRepo.ListAll(ctx)
	if err != nil {
		logCtx.WithError(err).Error("Failed to list all teachers from repository")
		return nil, fmt.Errorf("failed to list all teachers from repository: %w", err)
	}
	logCtx.WithField("count", len(teachers)).Info("Successfully listed all teachers")
	return teachers, nil
}

// ListActiveTeachers retrieves only active teachers from the repository.
// It ensures the action is performed by an authorized admin.
func (s *AdminService) ListActiveTeachers(ctx context.Context, performingAdminID int64) ([]*teacher.Teacher, error) {
	logCtx := s.log.WithFields(logrus.Fields{
		"operation":           "ListActiveTeachers",
		"performing_admin_id": performingAdminID,
	})
	logCtx.Info("Attempting to list active teachers")
	if performingAdminID != s.adminTelegramID {
		logCtx.Warn("Unauthorized attempt to list active teachers")
		return nil, ErrAdminNotAuthorized
	}
	activeTeachers, err := s.teacherRepo.ListActive(ctx)
	if err != nil {
		logCtx.WithError(err).Error("Failed to list active teachers from repository")
		return nil, fmt.Errorf("failed to list active teachers from repository: %w", err)
	}
	logCtx.WithField("count", len(activeTeachers)).Info("Successfully listed active teachers")
	return activeTeachers, nil
}
