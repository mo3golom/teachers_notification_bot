## Backend Task: B005 - Implement Admin Functionality: Add Teacher

**Objective:**
To implement the application service logic and the corresponding Telegram command handler that allows a designated Administrator to add new teachers to the system. This involves parsing the `/add_teacher` command, validating inputs, utilizing the `TeacherRepository` to persist the new teacher, and providing appropriate feedback messages to the Administrator.

**Background:**
The Product Requirements Document (PRD) specifies that Administrators must be able to manage the list of teachers[cite: 13, 34]. The `/add_teacher` command is a key part of this, enabling the addition of new teachers who will then be included in the notification system[cite: 48, 51]. This functionality directly supports User Story US6 and Use Case UC3[cite: 34, 48].

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Dependencies: `teacher.Repository`, `AppConfig`

---

**Steps to Completion:**

1.  **Define `AdminService`:**
    * Create a new file: `internal/app/admin_service.go`.
    * Define the `AdminService` struct and its constructor. It will depend on the `teacher.Repository` and the `AdminTelegramID` from the application's configuration.
    ```go
    // internal/app/admin_service.go
    package app

    import (
        "context"
        "database/sql"
        "fmt"
        "teacher_notification_bot/internal/domain/teacher"
        idb "teacher_notification_bot/internal/infra/database" // Assuming custom errors like ErrDuplicateTelegramID are here
    )

    // Custom application-level errors for admin service
    var ErrAdminNotAuthorized = fmt.Errorf("performing user is not authorized as an admin")
    var ErrTeacherAlreadyExists = fmt.Errorf("teacher with this Telegram ID already exists")

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
    ```

2.  **Implement `/add_teacher` Telegram Command Handler:**
    * Create a new file: `internal/infra/telegram/admin_handlers.go` (or add to an existing `handlers.go`).
    * Define the handler function for the `/add_teacher` command.
    ```go
    // internal/infra/telegram/admin_handlers.go
    package telegram

    import (
        "fmt"
        "strconv"
        "strings"
        "teacher_notification_bot/internal/app" // Your app service package

        "gopkg.in/telebot.v3"
    )

    // RegisterAdminHandlers registers handlers for admin commands.
    // It requires the bot instance, admin service, and the configured admin Telegram ID.
    func RegisterAdminHandlers(b *telebot.Bot, adminService *app.AdminService, adminTelegramID int64) {
        b.Handle("/add_teacher", func(c telebot.Context) error {
            if c.Sender().ID != adminTelegramID {
                return c.Send("Ошибка: У вас нет прав для выполнения этой команды.") // Unauthorized
            }

            args := c.Args() // c.Args() returns []string
            // Expected format: /add_teacher <TelegramID> <FirstName> [LastName]
            if len(args) < 2 || len(args) > 3 {
                return c.Send("Неверный формат команды. Используйте: /add_teacher <TelegramID> <Имя> [Фамилия]")
            }

            teacherTelegramID, err := strconv.ParseInt(args[0], 10, 64)
            if err != nil {
                return c.Send("Ошибка: Telegram ID должен быть числом.")
            }

            firstName := args[1]
            if strings.TrimSpace(firstName) == "" {
                return c.Send("Ошибка: Имя не может быть пустым.")
            }
            
            var lastName string
            if len(args) == 3 {
                lastName = args[2]
            }

            newTeacher, err := adminService.AddTeacher(c.Request().Context(), c.Sender().ID, teacherTelegramID, firstName, lastName)
            if err != nil {
                switch err {
                case app.ErrAdminNotAuthorized: // This check is technically redundant here due to the initial sender check
                    return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
                case app.ErrTeacherAlreadyExists:
                    return c.Send(fmt.Sprintf("Ошибка: Преподаватель с Telegram ID %d уже существует.", teacherTelegramID)) // [cite: 48]
                default:
                    c.Bot().OnError(err, c) // Log the full error for internal review
                    return c.Send(fmt.Sprintf("Произошла ошибка при добавлении преподавателя: %s", err.Error()))
                }
            }
            
            successMsg := fmt.Sprintf("Преподаватель %s %s (ID: %d) успешно добавлен.", newTeacher.FirstName, newTeacher.LastName.String, newTeacher.TelegramID) // [cite: 48]
            if !newTeacher.LastName.Valid {
                 successMsg = fmt.Sprintf("Преподаватель %s (ID: %d) успешно добавлен.", newTeacher.FirstName, newTeacher.TelegramID)
            }
            return c.Send(successMsg)
        })
    }
    ```

3.  **Integrate Handler and Service in `main.go`:**
    * Modify `cmd/bot/main.go` to initialize the `AdminService` and register the admin command handlers.
    ```go
    // cmd/bot/main.go
    package main

    import (
        "context"
        "database/sql"
        "fmt"
        "log"
        "os"
        "time"

        "teacher_notification_bot/internal/app" // Added for AdminService
        "teacher_notification_bot/internal/domain/teacher"
        "teacher_notification_bot/internal/infra/config"
        idb "teacher_notification_bot/internal/infra/database"
        "teacher_notification_bot/internal/infra/telegram" // Added for handlers

        "gopkg.in/telebot.v3"
    )

    func main() {
        fmt.Println("Teacher Notification Bot starting...")

        cfg, err := config.Load()
        if err != nil {
            log.Fatalf("FATAL: Could not load application configuration: %v", err)
        }
        log.Printf("INFO: Configuration loaded. Admin ID: %d", cfg.AdminTelegramID)


        db, err := idb.NewPostgresConnection(cfg.DatabaseURL)
        if err != nil {
            log.Fatalf("FATAL: Could not connect to database: %v", err)
        }
        defer db.Close()
        log.Println("INFO: Database connection established successfully.")

        teacherRepo := idb.NewPostgresTeacherRepository(db)
        log.Println("INFO: Teacher repository initialized.")

        // Initialize AdminService
        adminService := app.NewAdminService(teacherRepo, cfg.AdminTelegramID)
        log.Println("INFO: Admin service initialized.")


        // Initialize Telegram Bot
        pref := telebot.Settings{
            Token: cfg.TelegramToken,
            Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
            OnError: func(err error, c telebot.Context) { // Global error handler
                log.Printf("ERROR (telebot): %v", err)
                if c != nil {
                    log.Printf("ERROR (telebot context): Message: %s, Sender: %d", c.Text(), c.Sender().ID)
                }
            },
        }
        bot, err := telebot.NewBot(pref)
        if err != nil {
            log.Fatalf("FATAL: Could not create Telegram bot: %v", err)
        }

        // Register Handlers
        telegram.RegisterAdminHandlers(bot, adminService, cfg.AdminTelegramID) // Pass configured Admin ID
        log.Println("INFO: Admin command handlers registered.")

        // --- Remove or comment out B004 example usage ---
        // ... (B004 example code was here)

        log.Println("INFO: Application setup complete. Bot is starting...")
        bot.Start() // Start the bot
    }

    ```

---

**Acceptance Criteria:**

* An `AdminService` is implemented in `internal/app/admin_service.go` with an `AddTeacher` method.
* The `AdminService.AddTeacher` method:
    * Correctly verifies that the `performingAdminID` matches the configured `AdminTelegramID` from `AppConfig`, returning `ErrAdminNotAuthorized` otherwise.
    * Checks for pre-existing teachers by `newTeacherTelegramID` using the `TeacherRepository`. If a teacher exists, it returns `ErrTeacherAlreadyExists`. [cite: 48]
    * If validations pass, it successfully creates a new teacher (marked as `is_active = true`) via `TeacherRepository.Create()`.
* A Telegram command handler for `/add_teacher` is implemented in `internal/infra/telegram/admin_handlers.go` (or similar).
* The `/add_teacher` command handler:
    * Strictly limits access to users whose Telegram ID matches the `cfg.AdminTelegramID`. [cite: 53, 81] Unauthorized attempts receive an error message.
    * Correctly parses `TelegramID` (integer), `FirstName` (string), and an optional `LastName` (string) from the command arguments (`c.Args()`). [cite: 48]
    * Validates that `TelegramID` is a number and `FirstName` is not empty.
    * Invokes `AdminService.AddTeacher` with the parsed data and the sender's ID.
    * Sends a success message "Преподаватель [Имя Фамилия] (ID: [Telegram ID]) успешно добавлен." (or "Преподаватель [Имя] (ID: [Telegram ID]) успешно добавлен." if no last name) to the Admin on successful addition. [cite: 48]
    * Sends an error message "Ошибка: Преподаватель с Telegram ID [ID] уже существует." if the teacher already exists. [cite: 48]
    * Sends clear, user-friendly error messages for invalid arguments (e.g., non-numeric ID, missing first name) or other service-layer failures.
* The `/add_teacher` handler is correctly registered with the `telebot.Bot` instance in `main.go`, and the `AdminService` and `AdminTelegramID` are properly injected.
* The bot application runs, and the `/add_teacher` command is functional as described.

---

**Critical Tests (Manual Verification & Basis for Future Automated Tests):**

1.  **Successful Teacher Addition (With Last Name):**
    * Start the bot application.
    * Using the configured Admin Telegram account, send the command: `/add_teacher 123456701 TestOne UserOne` (use a unique `TelegramID`).
    * **Verify:** The bot responds with "Преподаватель TestOne UserOne (ID: 123456701) успешно добавлен."[cite: 48].
    * **Verify DB:** Check the `teachers` table. A new record should exist with `telegram_id = 123456701`, `first_name = 'TestOne'`, `last_name = 'UserOne'`, and `is_active = true`.
2.  **Successful Teacher Addition (Without Last Name):**
    * As Admin, send: `/add_teacher 123456702 TestTwo`
    * **Verify:** Bot responds with "Преподаватель TestTwo (ID: 123456702) успешно добавлен.".
    * **Verify DB:** Check `teachers` table for `telegram_id = 123456702`, `first_name = 'TestTwo'`, `last_name = NULL`, and `is_active = true`.
3.  **Attempt to Add Existing Teacher:**
    * As Admin, send the same command as in test 1 again: `/add_teacher 123456701 TestOne UserOne`.
    * **Verify:** Bot responds with "Ошибка: Преподаватель с Telegram ID 123456701 уже существует."[cite: 48]. No new record should be added to the DB.
4.  **Invalid Argument - Non-numeric Telegram ID:**
    * As Admin, send: `/add_teacher abcde TestThree UserThree`.
    * **Verify:** Bot responds with an error message like "Ошибка: Telegram ID должен быть числом.".
5.  **Invalid Argument - Missing First Name:**
    * As Admin, send: `/add_teacher 123456703`.
    * **Verify:** Bot responds with an error message like "Неверный формат команды. Используйте: /add_teacher <TelegramID> <Имя> [Фамилия]".
6.  **Invalid Argument - Too many/few parts:**
    * As Admin, send: `/add_teacher 123456704 TestFour UserFour ExtraPart`.
    * **Verify:** Bot responds with the format error message.
7.  **Unauthorized Access Attempt:**
    * Using a Telegram account whose ID is *not* the `cfg.AdminTelegramID`, send: `/add_teacher 123456705 NonAdmin Test`.
    * **Verify:** Bot responds with "Ошибка: У вас нет прав для выполнения этой команды." or a similar authorization error message. No teacher should be added.
8.  **Database `is_active` status:**
    * For any successfully added teacher (e.g., from Test 1 or 2), confirm in the `teachers` database table that the `is_active` column is `TRUE`.