## Backend Task: B007 - Implement Admin Functionality: List Teachers

**Objective:**
To implement the application service logic and the corresponding Telegram command handler that allows a designated Administrator to list teachers registered in the system. The command should support filtering by status (all teachers or only active teachers).

**Background:**
For effective system management, Administrators need a way to view the current list of teachers and their statuses. While not a primary "In Scope" command in the PRD's initial list[cite: 12, 13, 14, 15, 16, 17, 18, 19, 20, 21], it's mentioned as part of the first milestone [cite: 96] and as a risk mitigation strategy[cite: 121], making it a valuable administrative tool. This functionality provides transparency and aids in verifying the results of add/remove operations.

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Dependencies: `teacher.Repository`, `AppConfig`, `AdminService`

---

**Steps to Completion:**

1.  **Enhance `AdminService` with Listing Methods:**
    * Open `internal/app/admin_service.go`.
    * Add `ListAllTeachers` and `ListActiveTeachers` methods to the `AdminService`.
    ```go
    // internal/app/admin_service.go
    // ... (existing imports and AdminService struct) ...

    // ListAllTeachers retrieves all teachers from the repository.
    // It ensures the action is performed by an authorized admin.
    func (s *AdminService) ListAllTeachers(ctx context.Context, performingAdminID int64) ([]*teacher.Teacher, error) {
        if performingAdminID != s.adminTelegramID {
            return nil, ErrAdminNotAuthorized
        }
        teachers, err := s.teacherRepo.ListAll(ctx)
        if err != nil {
            return nil, fmt.Errorf("failed to list all teachers from repository: %w", err)
        }
        return teachers, nil
    }

    // ListActiveTeachers retrieves only active teachers from the repository.
    // It ensures the action is performed by an authorized admin.
    func (s *AdminService) ListActiveTeachers(ctx context.Context, performingAdminID int64) ([]*teacher.Teacher, error) {
        if performingAdminID != s.adminTelegramID {
            return nil, ErrAdminNotAuthorized
        }
        activeTeachers, err := s.teacherRepo.ListActive(ctx)
        if err != nil {
            return nil, fmt.Errorf("failed to list active teachers from repository: %w", err)
        }
        return activeTeachers, nil
    }
    ```

2.  **Implement `/list_teachers` Telegram Command Handler:**
    * Open `internal/infra/telegram/admin_handlers.go`.
    * Add a new handler function for the `/list_teachers` command within the `RegisterAdminHandlers` function.
    ```go
    // internal/infra/telegram/admin_handlers.go
    // ... (existing imports) ...

    // Inside RegisterAdminHandlers function:
    // func RegisterAdminHandlers(b *telebot.Bot, adminService *app.AdminService, adminTelegramID int64) {
    //     // ... existing handlers for /add_teacher, /remove_teacher ...

        b.Handle("/list_teachers", func(c telebot.Context) error {
            if c.Sender().ID != adminTelegramID {
                return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
            }

            args := c.Args()
            listType := "active" // Default to active
            if len(args) > 0 {
                listType = strings.ToLower(args[0])
            }

            var teachersList []*teacher.Teacher
            var err error
            var title string

            switch listType {
            case "active":
                title = "Активные преподаватели"
                teachersList, err = adminService.ListActiveTeachers(c.Request().Context(), c.Sender().ID)
            case "all":
                title = "Все преподаватели"
                teachersList, err = adminService.ListAllTeachers(c.Request().Context(), c.Sender().ID)
            default:
                return c.Send("Неверный аргумент. Используйте 'active' или 'all', или оставьте пустым для отображения активных преподавателей.")
            }

            if err != nil {
                if err == app.ErrAdminNotAuthorized { // Should be caught by the initial check, but good for defense
                     return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
                }
                c.Bot().OnError(err, c) // Log full error
                return c.Send(fmt.Sprintf("Произошла ошибка при получении списка преподавателей: %s", err.Error()))
            }

            if len(teachersList) == 0 {
                if listType == "active" {
                    return c.Send("Активных преподавателей не найдено.")
                }
                return c.Send("Список преподавателей пуст.")
            }

            var response strings.Builder
            response.WriteString(fmt.Sprintf("--- %s ---\n", title))
            for _, t := range teachersList {
                status := "Активен"
                if !t.IsActive {
                    status = "Неактивен"
                }
                lastNameStr := ""
                if t.LastName.Valid {
                    lastNameStr = " " + t.LastName.String
                }
                response.WriteString(fmt.Sprintf("ID: %d, TelegramID: %d, Имя: %s%s, Статус: %s\n",
                    t.ID, t.TelegramID, t.FirstName, lastNameStr, status))
            }
            // For very long lists, Telegram might truncate. Consider splitting messages or pagination for >4096 chars.
            // For V1, we send as one message.
            return c.Send(response.String(), &telebot.SendOptions{ParseMode: telebot.ModeDefault}) // ModeDefault to ensure no markdown issues with names
        })
    // } // End of RegisterAdminHandlers
    ```

3.  **Ensure Handler Registration:**
    * The `/list_teachers` handler is added within the existing `telegram.RegisterAdminHandlers` function in `internal/infra/telegram/admin_handlers.go`.
    * No changes are needed in `main.go` for registration itself if `RegisterAdminHandlers` is already called.

---

**Acceptance Criteria:**

* The `AdminService` in `internal/app/admin_service.go` is enhanced with `ListAllTeachers` and `ListActiveTeachers` methods.
* These service methods correctly authorize the performing admin by checking against `cfg.AdminTelegramID` and then call the appropriate `TeacherRepository.ListAll()` or `TeacherRepository.ListActive()` methods.
* A Telegram command handler for `/list_teachers` is implemented in `internal/infra/telegram/admin_handlers.go`.
* The `/list_teachers` command handler:
    * Restricts command execution to the configured Admin Telegram ID.
    * Defaults to listing active teachers if no argument is provided or if the argument is "active".
    * Lists all teachers (active and inactive) if the argument "all" is provided.
    * Sends an error message for any other argument.
    * Invokes the corresponding `AdminService` method (`ListActiveTeachers` or `ListAllTeachers`).
    * Formats the list of teachers clearly in the response message, including their internal `ID`, `TelegramID`, `FirstName`, `LastName` (if present), and `Status` ("Активен" or "Неактивен").
    * Sends a specific message like "Активных преподавателей не найдено." if no active teachers are found (when listing active).
    * Sends a message like "Список преподавателей пуст." if no teachers are found at all (when listing all and table is empty).
* The `/list_teachers` command is functional within the running bot application when triggered by the Admin.

---

**Critical Tests (Manual Verification & Basis for Future Automated Tests):**

1.  **Prerequisite: Populate Teachers:**
    * Ensure there are a few teachers in the database:
        * At least two active teachers.
        * At least one inactive teacher.
    * Example:
        * `/add_teacher 101 Active One`
        * `/add_teacher 102 Active Two UserTwo`
        * `/add_teacher 103 ToBeInactive TestInactive` -> then `/remove_teacher 103` to make inactive.
2.  **List Active Teachers (Default Behavior):**
    * As Admin, send the command: `/list_teachers`.
    * **Verify Bot Response:** The bot replies with a list containing only "Active One" and "Active Two UserTwo", formatted correctly, showing their status as "Активен". "ToBeInactive TestInactive" should not be present.
3.  **List Active Teachers (Explicit Argument):**
    * As Admin, send: `/list_teachers active`.
    * **Verify Bot Response:** Same output as Test 2.
4.  **List All Teachers:**
    * As Admin, send: `/list_teachers all`.
    * **Verify Bot Response:** The bot replies with a list containing all three teachers ("Active One", "Active Two UserTwo", "ToBeInactive TestInactive"). Their respective statuses ("Активен" or "Неактивен") should be correctly displayed.
5.  **List with No Active Teachers:**
    * Deactivate "Active One" and "Active Two UserTwo" (e.g., `/remove_teacher 101`, `/remove_teacher 102`). Now all teachers are inactive.
    * As Admin, send: `/list_teachers` (or `/list_teachers active`).
    * **Verify Bot Response:** Bot replies with "Активных преподавателей не найдено.".
6.  **List with No Teachers At All (requires clearing the table):**
    * **Setup:** Manually ensure the `teachers` table is empty (or all teachers are hard deleted if that functionality existed). For this test, you might need to manually TRUNCATE the table in a dev environment.
    * As Admin, send: `/list_teachers all`.
    * **Verify Bot Response:** Bot replies with "Список преподавателей пуст.".
    * As Admin, send: `/list_teachers active`.
    * **Verify Bot Response:** Bot replies with "Активных преподавателей не найдено.".
    * **Cleanup:** Re-populate teachers for subsequent tests if needed.
7.  **Invalid Argument:**
    * As Admin, send: `/list_teachers some_invalid_arg`.
    * **Verify Bot Response:** Bot replies with "Неверный аргумент. Используйте 'active' или 'all', или оставьте пустым для отображения активных преподавателей.".
8.  **Unauthorized Access Attempt:**
    * Using a Telegram account that is *not* the configured Admin, send: `/list_teachers`.
    * **Verify Bot Response:** Bot replies with "Ошибка: У вас нет прав для выполнения этой команды." or similar.