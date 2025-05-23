package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"teacher_notification_bot/internal/domain/teacher"
	"teacher_notification_bot/internal/infra/config"
	idb "teacher_notification_bot/internal/infra/database"
)

func main() {
	fmt.Println("Teacher Notification Bot starting...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: Could not load application configuration: %v", err)
	}

	log.Printf("INFO: Configuration loaded successfully. LogLevel: %s, Environment: %s", cfg.LogLevel, cfg.Environment)

	// Initialize Database Connection
	db, err := idb.NewPostgresConnection(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("FATAL: Could not connect to database: %v", err)
	}
	defer db.Close()
	log.Println("INFO: Database connection established successfully.")

	// Initialize Repositories
	teacherRepo := idb.NewPostgresTeacherRepository(db)
	log.Println("INFO: Teacher repository initialized.")

	// --- Example Usage (Temporary, for testing this task) ---
	// This section should be removed or moved to an appropriate service layer / test file later.
	exampleCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Example: Create a new teacher
	newTeacher := &teacher.Teacher{
		TelegramID: 999999, // Use a unique ID for testing
		FirstName:  "Test",
		LastName:   sql.NullString{String: "UserRepo", Valid: true},
		IsActive:   true,
	}
	err = teacherRepo.Create(exampleCtx, newTeacher)
	if err != nil {
		if err == idb.ErrDuplicateTelegramID {
			log.Printf("WARN: Test teacher with TelegramID %d already exists.", newTeacher.TelegramID)
			// If it already exists, try to fetch for subsequent tests
			existingTeacher, fetchErr := teacherRepo.GetByTelegramID(exampleCtx, newTeacher.TelegramID)
			if fetchErr != nil {
				log.Printf("ERROR: Could not fetch existing test teacher: %v", fetchErr)
			} else {
				newTeacher = existingTeacher // Use existing teacher for further example operations
				log.Printf("INFO: Using existing test teacher: %s %s", newTeacher.FirstName, newTeacher.LastName.String)
			}
		} else {
			log.Printf("ERROR: Failed to create test teacher: %v", err)
		}
	} else {
		log.Printf("INFO: Test teacher created with ID: %d", newTeacher.ID)
	}

	if newTeacher.ID > 0 { // Proceed if teacher exists or was created
		// Example: Get teacher by ID
		fetchedTeacher, err := teacherRepo.GetByID(exampleCtx, newTeacher.ID)
		if err != nil {
			log.Printf("ERROR: Failed to get test teacher by ID: %v", err)
		} else {
			log.Printf("INFO: Fetched teacher by ID: %d, Name: %s", fetchedTeacher.ID, fetchedTeacher.FirstName)
		}

		// Example: List active teachers
		activeTeachers, err := teacherRepo.ListActive(exampleCtx)
		if err != nil {
			log.Printf("ERROR: Failed to list active teachers: %v", err)
		} else {
			log.Printf("INFO: Found %d active teachers.", len(activeTeachers))
			for _, tt := range activeTeachers {
				if tt.ID == newTeacher.ID { // Log our test teacher if active
					log.Printf("INFO: Test teacher %s found in active list.", tt.FirstName)
				}
			}
		}
	}
	// --- End Example Usage ---

	log.Println("INFO: Application setup complete. Bot logic would start here.")
	// Placeholder for bot startup logic
	// select {} // Keep alive for bot (actual bot loop will be different)
}
