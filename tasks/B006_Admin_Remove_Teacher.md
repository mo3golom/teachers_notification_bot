## Backend Task: B006 - Implement Admin Functionality: Remove Teacher

**Objective:**
To implement the application service logic and the corresponding Telegram command handler that allows a designated Administrator to "remove" (deactivate by setting `IsActive = false`) teachers from the system. This includes parsing the `/remove_teacher` command, validating input, updating the teacher's status via the `TeacherRepository`, and providing appropriate feedback messages.

**Background:**
The Product Requirements Document (PRD) mandates that Administrators can manage the teacher list, which includes removing teachers who are no longer active or relevant[cite: 13, 35, 53]. The `/remove_teacher` command provides this capability by deactivating the teacher's record, preventing them from receiving further notifications. This aligns with User Story US7 and Use Case UC3[cite: 35, 49, 50].

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Dependencies: `teacher.Repository`, `AppConfig`, `AdminService`

---

**Steps to Completion:**

1.  **Enhance `AdminService` with `RemoveTeacher` Method:**
    * Open `internal/app/admin_service.go`.
    * Add the `RemoveTeacher` method to the `AdminService` struct.
    ```go
    // internal/app/admin_service.go
    // ... (existing imports and AdminService struct) ...

    // Add new application-level error
    var ErrTeacherAlreadyInactive = fmt.Errorf("teacher is already inactive")

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
    ```

2.  **Implement `/remove_teacher` Telegram Command Handler:**
    * Open `internal/infra/telegram/admin_handlers.go`.
    * Add a new handler function for the `/remove_teacher` command within the `RegisterAdminHandlers` function (or an equivalent setup).
    ```go
    // internal/infra/telegram/admin_handlers.go
    // ... (existing imports) ...

    // Inside RegisterAdminHandlers function:
    // func RegisterAdminHandlers(b *telebot.Bot, adminService *app.AdminService, adminTelegramID int64) {
    //     b.Handle("/add_teacher", func(c telebot.Context) error { ... }) // Existing handler

        b.Handle("/remove_teacher", func(c telebot.Context) error {
            if c.Sender().ID != adminTelegramID {
                return c.Send("Ошибка: У вас нет прав для выполнения этой команды.") // Unauthorized
            }

            args := c.Args() // c.Args() returns []string
            // Expected format: /remove_teacher <TelegramID>
            if len(args) != 1 {
                return c.Send("Неверный формат команды. Используйте: /remove_teacher <TelegramID>")
            }

            teacherTelegramID, err := strconv.ParseInt(args[0], 10, 64)
            if err != nil {
                return c.Send("Ошибка: Telegram ID должен быть числом.")
            }

            removedTeacher, err := adminService.RemoveTeacher(c.Request().Context(), c.Sender().ID, teacherTelegramID)
            if err != nil {
                switch err {
                case app.ErrAdminNotAuthorized: // Redundant here
                    return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
                case idb.ErrTeacherNotFound: // idb is alias for internal/infra/database
                    return c.Send(fmt.Sprintf("Преподаватель с таким Telegram ID %d не найден.", teacherTelegramID)) // [cite: 50]
                case app.ErrTeacherAlreadyInactive:
                     if removedTeacher != nil {
                        return c.Send(fmt.Sprintf("Преподаватель %s %s (ID: %d) уже был деактивирован.", removedTeacher.FirstName, removedTeacher.LastName.String, removedTeacher.TelegramID))
                     }
                     return c.Send(fmt.Sprintf("Преподаватель с Telegram ID %d уже был деактивирован.", teacherTelegramID))
                default:
                    c.Bot().OnError(err, c) // Log the full error
                    return c.Send(fmt.Sprintf("Произошла ошибка при удалении преподавателя: %s", err.Error()))
                }
            }
            
            // Construct name string carefully due to optional LastName
            var teacherName strings.Builder
            teacherName.WriteString(removedTeacher.FirstName)
            if removedTeacher.LastName.Valid && removedTeacher.LastName.String != "" {
                teacherName.WriteString(" ")
                teacherName.WriteString(removedTeacher.LastName.String)
            }

            successMsg := fmt.Sprintf("Преподаватель %s (ID: %d) успешно удален (деактивирован).", teacherName.String(), removedTeacher.TelegramID) // [cite: 49] (clarified "деактивирован")
            return c.Send(successMsg)
        })
    // } // End of RegisterAdminHandlers
    ```
    * **Note:** Ensure `idb "teacher_notification_bot/internal/infra/database"` is imported in `admin_service.go` if you refer to `idb.ErrTeacherNotFound`.

3.  **Ensure Handler Registration:**
    * The `/remove_teacher` handler is added within the existing `telegram.RegisterAdminHandlers` function in `internal/infra/telegram/admin_handlers.go`.
    * No changes are needed in `main.go` for registration itself if `RegisterAdminHandlers` is already called, as the new handler is part of that group.

---

**Acceptance Criteria:**

* The `AdminService` in `internal/app/admin_service.go` is enhanced with a `RemoveTeacher` method.
* The `AdminService.RemoveTeacher` method correctly:
    * Verifies the `performingAdminID` against the configured `AdminTelegramID`, returning `ErrAdminNotAuthorized` if mismatched.
    * Fetches the teacher using `TeacherRepository.GetByTelegramID`. If the teacher is not found, it returns `idb.ErrTeacherNotFound`.
    * If the fetched teacher's `IsActive` status is already `false`, it returns `ErrTeacherAlreadyInactive`.
    * If the teacher is found and active, it sets `IsActive` to `false` and calls `TeacherRepository.Update()` to persist the change.
* A Telegram command handler for `/remove_teacher` is implemented within `internal/infra/telegram/admin_handlers.go`.
* The `/remove_teacher` command handler:
    * Restricts command execution to the configured Admin Telegram ID, sending an error message for unauthorized attempts.
    * Correctly parses the `TelegramID` (integer) from the command arguments.
    * Validates that the `TelegramID` is a number and that exactly one argument is provided.
    * Invokes `AdminService.RemoveTeacher` with the parsed `TelegramID` and the sender's ID.
    * Sends a success message "Преподаватель [Имя Фамилия] (ID: [Telegram ID]) успешно удален (деактивирован)." upon successful deactivation[cite: 49].
    * Sends an error message "Преподаватель с таким Telegram ID [ID] не найден." if the teacher does not exist[cite: 50].
    * Sends an appropriate message like "Преподаватель [Имя Фамилия] (ID: [Telegram ID]) уже был деактивирован." if the teacher was already inactive.
    * Provides clear error messages for invalid argument formats or other service-layer failures.
* The `/remove_teacher` command is functional within the running bot application when triggered by the Admin.

---

**Critical Tests (Manual Verification & Basis for Future Automated Tests):**

1.  **Prerequisite: Add a Test Teacher:**
    * Using the Admin account, add a teacher who will be removed: `/add_teacher 777001 RemoveMe TestSurname`.
    * Verify successful addition and check the DB (`is_active = true`).
2.  **Successful Teacher Deactivation:**
    * As Admin, send: `/remove_teacher 777001`.
    * **Verify Bot Response:** Bot replies with "Преподаватель RemoveMe TestSurname (ID: 777001) успешно удален (деактивирован)."[cite: 49].
    * **Verify DB:** Check the `teachers` table for the record with `telegram_id = 777001`. The `is_active` column must now be `false`. The `updated_at` column should also reflect a recent timestamp.
3.  **Attempt to Remove Non-Existent Teacher:**
    * As Admin, send: `/remove_teacher 999999999` (an ID that is highly unlikely to exist).
    * **Verify Bot Response:** Bot replies with "Преподаватель с таким Telegram ID 999999999 не найден."[cite: 50].
4.  **Attempt to Remove Already Inactive Teacher:**
    * As Admin, send the command from Test 2 again: `/remove_teacher 777001`.
    * **Verify Bot Response:** Bot replies with a message indicating the teacher is already inactive, e.g., "Преподаватель RemoveMe TestSurname (ID: 777001) уже был деактивирован.".
5.  **Invalid Argument - Non-numeric Telegram ID:**
    * As Admin, send: `/remove_teacher NotaNumber`.
    * **Verify Bot Response:** Bot replies with an error message like "Ошибка: Telegram ID должен быть числом.".
6.  **Invalid Argument - Incorrect Number of Arguments:**
    * As Admin, send: `/remove_teacher` (no ID).
    * **Verify Bot Response:** Bot replies with the format error message "Неверный формат команды. Используйте: /remove_teacher <TelegramID>".
    * As Admin, send: `/remove_teacher 123 TooManyArgs`.
    * **Verify Bot Response:** Bot replies with the same format error message.
7.  **Unauthorized Access Attempt:**
    * Using a Telegram account that is *not* the configured Admin, send: `/remove_teacher 777001`.
    * **Verify Bot Response:** Bot replies with "Ошибка: У вас нет прав для выполнения этой команды." or similar. No change should occur in the database.
8.  **Impact on `ListActive` (Conceptual):**
    * Although not directly testable via a user command in this task, internally, after a teacher is deactivated, a call to `teacherRepo.ListActive()` should no longer include this teacher. This will be important for the notification logic.