## Backend Task: B004 - Implement `Teacher` Domain Entity and Repository

**Objective:**
To define the `Teacher` domain entity in Go, create a `TeacherRepository` interface outlining data access methods, and implement this interface using PostgreSQL. This implementation will cover CRUD (Create, Read, Update, Delete - specifically soft delete via an `is_active` flag) operations for teachers.

**Background:**
The `Teacher` entity is a cornerstone of the bot's functionality, representing the individuals who will receive notifications and interact with the system[cite: 28]. A well-defined repository pattern will abstract database interactions, enhancing modularity, testability, and maintainability of the application. This task directly addresses the functional requirements for managing the list of teachers (FR1.1, FR1.2, FR1.3)[cite: 52, 53].

**Tech Stack:**
* Go version: 1.24
* Database: PostgreSQL
* Libraries: `database/sql`, `github.com/lib/pq`

---

**Steps to Completion:**

1.  **Define `Teacher` Domain Entity:**
    * Create the directory `internal/domain/teacher/`.
    * Inside this directory, create a file named `teacher.go`.
    * Define the `Teacher` struct. It should align with the `teachers` table schema created in task `B003`. Use `sql.NullString` for `LastName` to correctly handle its optionality.
        ```go
        // internal/domain/teacher/teacher.go
        package teacher

        import (
            "database/sql"
            "time"
        )

        // Teacher represents a teacher in the system.
        type Teacher struct {
            ID          int64
            TelegramID  int64
            FirstName   string
            LastName    sql.NullString // To handle optional last name
            IsActive    bool
            CreatedAt   time.Time
            UpdatedAt   time.Time
        }
        ```

2.  **Define `TeacherRepository` Interface:**
    * In the `internal/domain/teacher/` directory, create a file named `repository.go`.
    * Define the `Repository` interface (conventionally named `Repository` within its own package, or `TeacherRepository` if preferred for explicitness elsewhere).
        ```go
        // internal/domain/teacher/repository.go
        package teacher

        import (
            "context"
        )

        // Repository defines the operations for persisting and retrieving Teacher entities.
        type Repository interface {
            Create(ctx context.Context, teacher *Teacher) error
            GetByID(ctx context.Context, id int64) (*Teacher, error)
            GetByTelegramID(ctx context.Context, telegramID int64) (*Teacher, error)
            Update(ctx context.Context, teacher *Teacher) error // Should handle updates to FirstName, LastName, IsActive
            ListActive(ctx context.Context) ([]*Teacher, error)
            ListAll(ctx context.Context) ([]*Teacher, error) // For admin purposes
        }
        ```
    * *(Self-correction: The PRD implies admins "удаление неактуальных" [cite: 13] and "удалять преподавателей"[cite: 36, 53]. The `Update` method should be capable of setting `IsActive` to `false` effectively performing a soft delete. A separate `Delete` method for hard delete is not required for V1.)*

3.  **Implement PostgreSQL `TeacherRepository`:**
    * Navigate to `internal/infra/database/`.
    * Create a new file named `postgres_teacher_repository.go`.
    * Define a struct `PostgresTeacherRepository` that embeds `*sql.DB`.
    * Implement all methods from the `teacher.Repository` interface.

    ```go
    // internal/infra/database/postgres_teacher_repository.go
    package database

    import (
        "context"
        "database/sql"
        "fmt" // For error wrapping
        "time"

        "teacher_notification_bot/internal/domain/teacher" // Adjust import path

        _ "[github.com/lib/pq](https://github.com/lib/pq)" // PostgreSQL driver
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
            if err.Error().Contains("unique constraint") && err.Error().Contains("teachers_telegram_id_key") { // Example check
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
    ```

4.  **Database Connection Setup:**
    * Create a file `internal/infra/database/db.go` (if it doesn't already exist from a previous step, though it wasn't explicitly created in B001-B003 task descriptions, so creating it now).
    * Add a function `NewPostgresConnection` to establish and verify the database connection.
        ```go
        // internal/infra/database/db.go
        package database

        import (
            "database/sql"
            "fmt"
            "time"

            _ "[github.com/lib/pq](https://github.com/lib/pq)" // PostgreSQL driver
        )

        const (
            defaultMaxOpenConns    = 25
            defaultMaxIdleConns    = 25
            defaultConnMaxLifetime = 5 * time.Minute
            defaultConnMaxIdleTime = 1 * time.Minute
        )
        
        // NewPostgresConnection creates and returns a new PostgreSQL database connection.
        // It also pings the database to ensure connectivity.
        func NewPostgresConnection(dataSourceName string) (*sql.DB, error) {
            db, err := sql.Open("postgres", dataSourceName)
            if err != nil {
                return nil, fmt.Errorf("failed to open database connection: %w", err)
            }

            // Set connection pool settings
            db.SetMaxOpenConns(defaultMaxOpenConns)
            db.SetMaxIdleConns(defaultMaxIdleConns)
            db.SetConnMaxLifetime(defaultConnMaxLifetime)
            db.SetConnMaxIdleTime(defaultConnMaxIdleTime)

            // Verify the connection with a Ping
            if err = db.Ping(); err != nil {
                db.Close() // Close the connection if ping fails
                return nil, fmt.Errorf("failed to ping database: %w", err)
            }

            return db, nil
        }
        ```
    * This `*sql.DB` instance will eventually be passed to `NewPostgresTeacherRepository` when wiring up the application in `main.go`.

5.  **Integrate DB Connection in `main.go` (Basic Example):**
    * Modify `cmd/bot/main.go` to establish the DB connection using the loaded configuration and initialize the repository.
        ```go
        // cmd/bot/main.go
        package main

        import (
            "context" // Added
            "database/sql" // Added
            "fmt"
            "log"
            "os"
            "time" // Added for example usage

            "teacher_notification_bot/internal/domain/teacher" // Added
            "teacher_notification_bot/internal/infra/config"
            idb "teacher_notification_bot/internal/infra/database" // Renamed to avoid conflict with sql package
        )

        func main() {
            fmt.Println("Teacher Notification Bot starting...")

            cfg, err := config.Load()
            if err != nil {
                log.Fatalf("FATAL: Could not load application configuration: %v", err)
                // os.Exit(1) // log.Fatalf already exits
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
        ```

---

**Acceptance Criteria:**

* The `teacher.Teacher` struct is correctly defined in `internal/domain/teacher/teacher.go`.
* The `teacher.Repository` interface is defined in `internal/domain/teacher/repository.go` with methods: `Create`, `GetByID`, `GetByTelegramID`, `Update`, `ListActive`, `ListAll`.
* The `database.PostgresTeacherRepository` struct in `internal/infra/database/postgres_teacher_repository.go` implements all methods of the `teacher.Repository` interface.
* The `Create` method successfully inserts a new teacher record into the PostgreSQL `teachers` table and correctly populates the input `Teacher` struct's `ID`, `CreatedAt`, and `UpdatedAt` fields. It also handles potential duplicate `telegram_id` errors.
* `GetByID` and `GetByTelegramID` methods retrieve the correct teacher data or return `ErrTeacherNotFound` (or a wrapped version) if no teacher matches the criteria.
* The `Update` method successfully modifies a teacher's `FirstName`, `LastName`, and `IsActive` status in the database and updates their `UpdatedAt` field.
* `ListActive` method returns a slice containing only teachers where `is_active = true`.
* `ListAll` method returns a slice of all teachers from the database, irrespective of their `is_active` status.
* The `database.NewPostgresConnection` function in `internal/infra/database/db.go` successfully establishes and pings a PostgreSQL connection using the provided DSN.
* All repository methods implement basic error handling, checking for and returning/wrapping errors from database operations (e.g., `sql.ErrNoRows` mapped to `ErrTeacherNotFound`).
* The `main.go` is updated to initialize the database connection and the `PostgresTeacherRepository`. The temporary example usage code in `main.go` runs without critical errors (allowing for duplicate creation for idempotency in testing).

---

**Critical Tests (Manual Verification & Basis for Future Unit/Integration Tests):**

1.  **Run `main.go`:**
    * Ensure your PostgreSQL container is running and accessible with the schema from B003 applied.
    * Ensure your `.env` file (or environment variables) has the correct `DATABASE_URL`.
    * Execute `go run ./cmd/bot/main.go`.
    * Observe the logs for:
        * Successful configuration loading.
        * Successful database connection.
        * Successful repository initialization.
        * Logs from the example usage: teacher creation (or warning if duplicate), fetching, and listing.
2.  **Database Inspection (Manual):**
    * After running `main.go`, connect to your PostgreSQL database.
    * **Verify Creation:** Check the `teachers` table for the "Test UserRepo" record (TelegramID 999999). Confirm `id`, `created_at`, and `updated_at` are populated.
    * **Verify Listing:** If you manually add more teachers (some active, some inactive), test the `ListActive` and `ListAll` logic by calling them (e.g., temporarily adding more calls in `main.go` or via a DB client and then verifying).
    * **Verify Update (manual or extend example):**
        * Manually update the test teacher's `first_name` or set `is_active = false` in the DB.
        * Call `GetByID` or `GetByTelegramID` (via extended example in `main.go` or another tool) to see if the changes are reflected.
        * Verify the `updated_at` column was modified by the trigger.
3.  **Error Case - Not Found (extend example or manual):**
    * Try to `GetByID` or `GetByTelegramID` with a non-existent ID. Verify that `ErrTeacherNotFound` is handled or logged appropriately in the example.
4.  **Error Case - Duplicate Telegram ID (covered by example):**
    * Run `main.go` multiple times. The first time it should create the teacher. Subsequent times, it should log the "WARN: Test teacher ... already exists" message due to the unique constraint on `telegram_id`.

*(Note: Full unit and integration tests for the repository will be part of a dedicated testing task later. This task focuses on the implementation and basic verification.)*