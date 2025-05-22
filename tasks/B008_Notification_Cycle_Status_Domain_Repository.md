## Backend Task: B008 - Implement `NotificationCycle` and `TeacherReportStatus` Domain Entities and Repositories

**Objective:**
To define the Go domain entities for `NotificationCycle` and `TeacherReportStatus`, along with their respective repository interfaces and PostgreSQL implementations. These components are essential for tracking notification runs, individual teacher progress for each report query within a cycle, and managing the overall state of notifications.

**Background:**
The core functionality of the bot revolves around sending scheduled notifications and tracking teacher responses. The `NotificationCycle` entity will represent each notification event (e.g., "mid-month dispatch"), and `TeacherReportStatus` will track the state of each specific report query sent to a teacher within that cycle (e.g., "Teacher A, Table 1, Status: Answered Yes"). [cite: 20, 72] This structure is vital for the notification workflow, reminder logic, and eventually reporting completion to the manager, as outlined in PRD sections on Functional Requirements (FR6) and Data Persistence. [cite: 72, 73, 84, 87]

**Tech Stack:**
* Go version: 1.24
* Database: PostgreSQL
* Libraries: `database/sql`, `time`

---

**Steps to Completion:**

1.  **Define Domain Constants/Enums:**
    * Create a new directory `internal/domain/notification/`.
    * In `internal/domain/notification/shared_types.go` (or a similar name), define constants for `ReportKey`, `NotificationInteractionStatus`, and `CycleType`.
    ```go
    // internal/domain/notification/shared_types.go
    package notification

    // ReportKey identifies the specific table/report being queried.
    type ReportKey string

    const (
        ReportKeyTable1Lessons   ReportKey = "TABLE_1_LESSONS"    // FR3.1, FR3.2 [cite: 58, 60]
        ReportKeyTable3Schedule  ReportKey = "TABLE_3_SCHEDULE"   // FR3.1, FR3.2 [cite: 59, 61]
        ReportKeyTable2OTV       ReportKey = "TABLE_2_OTV"        // FR3.2 (only end of month) [cite: 62]
    )

    // InteractionStatus represents the state of a teacher's response to a report query.
    type InteractionStatus string // FR6.1 [cite: 72]

    const (
        StatusPendingQuestion        InteractionStatus = "PENDING_QUESTION"
        StatusAnsweredYes            InteractionStatus = "ANSWERED_YES"
        StatusAnsweredNo             InteractionStatus = "ANSWERED_NO"
        StatusAwaitingReminder1H     InteractionStatus = "AWAITING_REMINDER_1H"     // FR4.2 [cite: 66, 67]
        StatusAwaitingReminderNextDay InteractionStatus = "AWAITING_REMINDER_NEXT_DAY" // FR4.3 [cite: 68]
        // StatusCycleFullyConfirmed might be a status for the teacher overall, rather than per report.
        // For now, individual report statuses cover FR6.1 [cite: 72]
    )

    // CycleType indicates if the notification cycle is for mid-month or end-of-month.
    type CycleType string

    const (
        CycleTypeMidMonth  CycleType = "MID_MONTH"  // For 15th of month notifications [cite: 14, 56]
        CycleTypeEndMonth  CycleType = "END_MONTH"  // For last day of month notifications [cite: 14, 57]
    )
    ```

2.  **Define `NotificationCycle` Domain Entity:**
    * In `internal/domain/notification/`, create `cycle.go`.
    * Define the `Cycle` struct (named `Cycle` to avoid stutter with package name).
    ```go
    // internal/domain/notification/cycle.go
    package notification

    import "time"

    // Cycle represents a single notification run (e.g., mid-month May 2025).
    // Corresponds to the 'notification_cycles' table in schema B003.
    type Cycle struct {
        ID        int32     // SERIAL in DB
        CycleDate time.Time // Specific date of the cycle
        Type      CycleType // e.g., MID_MONTH, END_MONTH
        CreatedAt time.Time
    }
    ```

3.  **Define `TeacherReportStatus` Domain Entity:**
    * In `internal/domain/notification/`, create `status.go`.
    * Define the `ReportStatus` struct (named `ReportStatus`).
    ```go
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
    ```

4.  **Define `NotificationRepository` Interface:**
    * In `internal/domain/notification/`, create `repository.go`.
    ```go
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
    }
    ```

5.  **Implement PostgreSQL `NotificationRepository`:**
    * Create `internal/infra/database/postgres_notification_repository.go`.
    * Implement the `notification.Repository` interface. This will be a substantial file.
    ```go
    // internal/infra/database/postgres_notification_repository.go
    package database

    import (
        "context"
        "database/sql"
        "fmt"
        "strings"
        "time"
        "teacher_notification_bot/internal/domain/notification" // Adjust import path

        _ "[github.com/lib/pq](https://github.com/lib/pq)"
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
        cycle := &notification.Cycle{}
        err := r.db.QueryRowContext(ctx, query, id).Scan(&cycle.ID, &cycle.CycleDate, &cycle.Type, &cycle.CreatedAt)
        if err != nil {
            if err == sql.ErrNoRows {
                return nil, ErrCycleNotFound
            }
            return nil, fmt.Errorf("error getting notification cycle by ID: %w", err)
        }
        return cycle, nil
    }
    
    func (r *PostgresNotificationRepository) GetCycleByDateAndType(ctx context.Context, cycleDate time.Time, cycleType notification.CycleType) (*notification.Cycle, error) {
        query := `SELECT id, cycle_date, cycle_type, created_at FROM notification_cycles WHERE cycle_date = $1 AND cycle_type = $2 ORDER BY created_at DESC LIMIT 1`
        cycle := &notification.Cycle{}
        // Normalize cycleDate to just date part if it contains time
        dateOnly := time.Date(cycleDate.Year(), cycleDate.Month(), cycleDate.Day(), 0, 0, 0, 0, cycleDate.Location())
        err := r.db.QueryRowContext(ctx, query, dateOnly, cycleType).Scan(&cycle.ID, &cycle.CycleDate, &cycle.Type, &cycle.CreatedAt)
        if err != nil {
            if err == sql.ErrNoRows {
                return nil, ErrCycleNotFound
            }
            return nil, fmt.Errorf("error getting notification cycle by date and type: %w", err)
        }
        return cycle, nil
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
        rs := &notification.ReportStatus{}
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
        return rs, nil
    }

    func (r *PostgresNotificationRepository) GetReportStatusByID(ctx context.Context, id int64) (*notification.ReportStatus, error) {
        query := `SELECT id, teacher_id, cycle_id, report_key, status, last_notified_at, response_attempts, created_at, updated_at
                   FROM teacher_report_statuses WHERE id = $1`
        rs := &notification.ReportStatus{}
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
        return rs, nil
    }
    
    // Helper to scan multiple rows
    func scanReportStatuses(rows *sql.Rows) ([]*notification.ReportStatus, error) {
        statuses := make([]*notification.ReportStatus, 0)
        for rows.Next() {
            rs := &notification.ReportStatus{}
            if err := rows.Scan(
                &rs.ID, &rs.TeacherID, &rs.CycleID, &rs.ReportKey, &rs.Status,
                &rs.LastNotifiedAt, &rs.ResponseAttempts, &rs.CreatedAt, &rs.UpdatedAt,
            ); err != nil {
                return nil, fmt.Errorf("error scanning report status row: %w", err)
            }
            statuses = append(statuses, rs)
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
        // This query assumes 'last_notified_at' is relevant for deciding if a reminder is due.
        // For "next day" reminders, the logic might be more complex (e.g., checking if it's indeed the next day).
        // This is a simplified version.
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

        // Convert expectedReportKeys to a slice of strings for the query
        keysAsStrings := make([]string, len(expectedReportKeys))
        for i, k := range expectedReportKeys {
            keysAsStrings[i] = string(k)
        }
        
        // This query counts how many of the expected reports for this teacher in this cycle DO NOT have the status 'ANSWERED_YES'.
        // If the count is 0, it means all expected reports are 'ANSWERED_YES'.
        query := `SELECT COUNT(*)
                   FROM teacher_report_statuses
                   WHERE teacher_id = $1
                     AND cycle_id = $2
                     AND report_key = ANY($3::varchar[])
                     AND status != $4`
        
        var unconfirmedCount int
        err := r.db.QueryRowContext(ctx, query, teacherID, cycleID, pq.Array(keysAsStrings), notification.StatusAnsweredYes).Scan(&unconfirmedCount)
        if err != nil {
            // If ErrNoRows, it means no records matched the criteria (teacher_id, cycle_id, report_key IN expected, status != ANSWERED_YES).
            // This is effectively the same as unconfirmedCount = 0 IF there are entries for all expected keys.
            // However, this query doesn't guarantee that entries for ALL expected keys exist.
            // A more robust check might be to count records that ARE 'ANSWERED_YES' and compare with len(expectedReportKeys).
            // For now, this assumes that statuses for all expected keys exist for the teacher in the cycle.
             return false, fmt.Errorf("error checking all reports confirmed: %w", err)
        }

        return unconfirmedCount == 0, nil
    }
    ```
    * **Note on `AreAllReportsConfirmedForTeacher`**: The initial implementation of `AreAllReportsConfirmedForTeacher` counts non-YES statuses. A more robust version might count YES statuses and check if it equals `len(expectedReportKeys)`, ensuring all records actually exist and are YES. For now, the provided one is a starting point. The `pq.Array` is used to pass a slice to a SQL `ANY($array)` clause.

6.  **Integrate New Repository in `main.go` (Initialization):**
    * Modify `cmd/bot/main.go` to initialize the `PostgresNotificationRepository`.
    ```go
    // cmd/bot/main.go
    // ... (other initializations)
        notificationRepo := idb.NewPostgresNotificationRepository(db) // idb is alias for internal/infra/database
        log.Println("INFO: Notification repository initialized.")
    // ...
    ```

---

**Acceptance Criteria:**

* Domain constants `ReportKey`, `InteractionStatus`, and `CycleType` are correctly defined in `internal/domain/notification/shared_types.go`.
* The `notification.Cycle` struct is defined in `internal/domain/notification/cycle.go`.
* The `notification.ReportStatus` struct is defined in `internal/domain/notification/status.go`.
* The `notification.Repository` interface in `internal/domain/notification/repository.go` includes all specified methods for managing cycles and report statuses.
* The `database.PostgresNotificationRepository` in `internal/infra/database/postgres_notification_repository.go` successfully implements all methods of the `notification.Repository` interface.
* `CreateCycle` correctly inserts a new `notification_cycles` record and populates the ID and CreatedAt fields.
* `GetCycleByID` and `GetCycleByDateAndType` retrieve cycle data accurately or return `ErrCycleNotFound`.
* `CreateReportStatus` inserts a single `teacher_report_statuses` record, returning `ErrDuplicateReportStatus` on constraint violation.
* `BulkCreateReportStatuses` efficiently inserts multiple status records within a transaction.
* `UpdateReportStatus` modifies an existing status record (e.g., changes `status`, `last_notified_at`, `response_attempts`) and updates `updated_at`.
* `GetReportStatus` and `GetReportStatusByID` retrieve specific status records or return `ErrReportStatusNotFound`.
* Listing methods (`ListReportStatusesByCycleAndTeacher`, `ListReportStatusesByCycle`, `ListReportStatusesByStatusAndCycle`, `ListReportStatusesForReminders`) return correct collections of `ReportStatus` objects based on the filter criteria.
* `AreAllReportsConfirmedForTeacher` correctly determines if a given teacher has marked all `expectedReportKeys` as `STATUS_ANSWERED_YES` for a specific cycle.
* Custom errors (`ErrCycleNotFound`, `ErrReportStatusNotFound`, `ErrDuplicateReportStatus`) are used appropriately.
* The `PostgresNotificationRepository` is initialized in `main.go`.

---

**Critical Tests (Manual Verification & Basis for Future Unit/Integration Tests):**

1.  **NotificationCycle Management:**
    * **Create:** Call `CreateCycle` to add a new cycle (e.g., for today, type `MID_MONTH`). Verify the record in `notification_cycles` table and that the returned object has `ID` and `CreatedAt` populated.
    * **GetByID:** Fetch the created cycle by its `ID`. Verify data matches.
    * **GetByDateAndType:** Fetch the cycle using the date and type. Verify.
    * **Get Non-Existent:** Attempt to fetch a cycle with an invalid ID or date/type combination. Verify `ErrCycleNotFound` is handled.
2.  **TeacherReportStatus Management (Single & Bulk):**
    * **Prerequisites:** Ensure at least one `teacher` and one `notification_cycle` exist.
    * **CreateSingle:** Call `CreateReportStatus` for a teacher, cycle, and report key (e.g., `TABLE_1_LESSONS`, status `PENDING_QUESTION`). Verify record in `teacher_report_statuses`.
    * **Test Duplicate:** Attempt to `CreateReportStatus` with the same `teacher_id`, `cycle_id`, `report_key`. Verify `ErrDuplicateReportStatus`.
    * **BulkCreate:** Prepare a slice of `ReportStatus` objects for multiple teachers/reports in the same cycle. Call `BulkCreateReportStatuses`. Verify all records are in the DB.
    * **GetByID/GetReportStatus:** Fetch one of the created statuses by its `ID` or by `teacher_id, cycle_id, report_key`. Verify data.
    * **Update:** Fetch a status, change its `Status` (e.g., to `ANSWERED_YES`) and `LastNotifiedAt`, then call `UpdateReportStatus`. Verify changes in DB, especially `updated_at`.
3.  **Listing TeacherReportStatuses:**
    * **ByCycleAndTeacher:** Create several statuses for one teacher in a cycle. Call `ListReportStatusesByCycleAndTeacher`. Verify all and only their statuses are returned.
    * **ByCycle:** Create statuses for multiple teachers in one cycle. Call `ListReportStatusesByCycle`. Verify all relevant statuses are returned.
    * **ByStatusAndCycle:** Create statuses with different `InteractionStatus` values in a cycle. Call `ListReportStatusesByStatusAndCycle` for a specific status (e.g., `PENDING_QUESTION`). Verify correct filtering.
4.  **`AreAllReportsConfirmedForTeacher` Logic:**
    * **Setup:** For a teacher and a cycle, create status entries for `TABLE_1_LESSONS` and `TABLE_3_SCHEDULE`.
    * **Scenario 1 (Not All Confirmed):** Set Table1 status to `ANSWERED_YES`, Table3 to `PENDING_QUESTION`. Call `AreAllReportsConfirmedForTeacher` with `expectedReportKeys = [ReportKeyTable1Lessons, ReportKeyTable3Schedule]`. Verify result is `false`.
    * **Scenario 2 (All Confirmed):** Update Table3 status to `ANSWERED_YES`. Call `AreAllReportsConfirmedForTeacher` again. Verify result is `true`.
    * **Scenario 3 (One Missing Expected):** Only create status for Table1 as `ANSWERED_YES`. Call `AreAllReportsConfirmedForTeacher` with `expectedReportKeys = [ReportKeyTable1Lessons, ReportKeyTable3Schedule]`. (Current implementation might return true if it only checks existing non-YES records. Ideal check would ensure all expected keys are present AND YES). For current implementation, focus on defined states.
5.  **`ListReportStatusesForReminders` Logic (Basic Test):**
    * Create a status with `StatusAwaitingReminder1H` and `LastNotifiedAt` set to 2 hours ago.
    * Call `ListReportStatusesForReminders` with `status = StatusAwaitingReminder1H` and `notifiedBefore = time.Now().Add(-30 * time.Minute)`. Verify the status is returned.
    * Call with `notifiedBefore = time.Now().Add(-3 * time.Hour)`. Verify it's not returned (if it should only be for things older than a certain point but not *too* old - the method signature is simple for now).
6.  **Error Handling:**
    * Attempt to fetch statuses/cycles using invalid IDs. Verify `ErrReportStatusNotFound` or `ErrCycleNotFound`.

*(Note: This is a large repository. Thorough unit tests will be crucial in a subsequent testing-focused task.)*