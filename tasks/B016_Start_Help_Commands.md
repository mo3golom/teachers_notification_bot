## Backend Task: B016 - Implement Basic `/start` and `/help` Commands

**Objective:**
To implement the standard Telegram bot commands `/start` and `/help`. The `/start` command will provide a context-aware welcome message depending on whether the user is an Administrator, a known Teacher (active or inactive), or an unknown user. The `/help` command will display a list of available commands or guidance relevant to the user's role.

**Background:**
The `/start` and `/help` commands are fundamental for user interaction with any Telegram bot. They provide an initial point of contact and guidance on how to use the bot. While the PRD V1 scope for teachers doesn't involve self-registration via `/start`[cite: 12], this command can still serve to acknowledge recognized users and guide new ones. A help command improves usability by listing available functionalities.

**Tech Stack:**
* Go version: 1.24
* Telegram Bot Library: `gopkg.in/telebot.v3`
* Dependencies: `config.AppConfig`, `teacher.Repository`, Logger

---

**Steps to Completion:**

1.  **Create General Bot Command Handlers File:**
    * Create a new file: `internal/infra/telegram/bot_commands_handler.go`.
    * This file will contain handlers for general commands like `/start` and `/help`.

2.  **Implement `/start` Command Handler:**
    * In `internal/infra/telegram/bot_commands_handler.go`:
    ```go
    // internal/infra/telegram/bot_commands_handler.go
    package telegram

    import (
        "fmt"
        "strings"
        "teacher_notification_bot/internal/domain/teacher"
        "teacher_notification_bot/internal/infra/config"
        idb "teacher_notification_bot/internal/infra/database" // For ErrTeacherNotFound

        "[github.com/sirupsen/logrus](https://github.com/sirupsen/logrus)"
        "gopkg.in/telebot.v3"
    )

    func RegisterBotCommands(
        b *telebot.Bot,
        cfg *config.AppConfig, // For AdminTelegramID
        teacherRepo teacher.Repository,
        baseLogger *logrus.Entry, // For contextual logging
    ) {
        startHelpLogger := baseLogger.WithField("handler_group", "start_help")

        b.Handle("/start", func(c telebot.Context) error {
            senderID := c.Sender().ID
            logCtx := startHelpLogger.WithField("command", "/start").WithField("sender_id", senderID)
            logCtx.Info("Processing /start command")

            // Check if Admin
            if senderID == cfg.AdminTelegramID {
                logCtx.Info("User identified as Admin")
                return c.Send(fmt.Sprintf("Привет, Администратор %s! Я готов к работе. Используйте /help для списка команд.", c.Sender().FirstName))
            }

            // Check if Teacher
            userAsTeacher, err := teacherRepo.GetByTelegramID(c.Request().Context(), senderID)
            if err == nil { // Teacher found
                if userAsTeacher.IsActive {
                    logCtx.WithField("teacher_id", userAsTeacher.ID).Info("User identified as Active Teacher")
                    return c.Send(fmt.Sprintf("Привет, %s! Я бот для напоминаний о заполнении таблиц. Я сообщу вам, когда придет время.", userAsTeacher.FirstName))
                }
                logCtx.WithField("teacher_id", userAsTeacher.ID).Info("User identified as Inactive Teacher")
                return c.Send("Ваш аккаунт преподавателя неактивен. Пожалуйста, свяжитесь с администратором.")
            } else if err != idb.ErrTeacherNotFound { // Some other DB error
                logCtx.WithError(err).Error("Error checking teacher status for /start command")
                return c.Send("Произошла ошибка при проверке вашего статуса. Пожалуйста, попробуйте позже.")
            }

            // Unknown user
            logCtx.Info("User is unknown")
            return c.Send("Привет! Я бот для напоминаний преподавателям. Если вы преподаватель, пожалуйста, попросите администратора добавить вас в систему.")
        })

        // ... /help handler will be added next
    }
    ```

3.  **Implement `/help` Command Handler:**
    * In `internal/infra/telegram/bot_commands_handler.go`, continue within `RegisterBotCommands`:
    ```go
    // internal/infra/telegram/bot_commands_handler.go
    // Continuing inside RegisterBotCommands function:

        b.Handle("/help", func(c telebot.Context) error {
            senderID := c.Sender().ID
            logCtx := startHelpLogger.WithField("command", "/help").WithField("sender_id", senderID)
            logCtx.Info("Processing /help command")

            // Admin Help
            if senderID == cfg.AdminTelegramID {
                logCtx.Info("User identified as Admin, sending admin help.")
                var helpText strings.Builder
                helpText.WriteString("Доступные команды Администратора:\n\n")
                helpText.WriteString("`/add_teacher <TelegramID> <Имя> [Фамилия]`\n - Добавить нового преподавателя в систему.\n\n") //
                helpText.WriteString("`/remove_teacher <TelegramID>`\n - Деактивировать преподавателя (он перестанет получать уведомления).\n\n") //
                helpText.WriteString("`/list_teachers [active|all]`\n - Показать список преподавателей. По умолчанию показывает активных. 'all' - для всех.\n\n") //
                helpText.WriteString("`/help`\n - Показать это справочное сообщение.")
                // Using MarkdownV2 requires escaping special characters, or use HTML/ModeDefault
                return c.Send(helpText.String(), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown}) // Be careful with Markdown parsing
            }

            // Teacher Help
            userAsTeacher, err := teacherRepo.GetByTelegramID(c.Request().Context(), senderID)
            if err == nil { // Teacher found
                if userAsTeacher.IsActive {
                    logCtx.WithField("teacher_id", userAsTeacher.ID).Info("User identified as Active Teacher, sending teacher help.")
                    return c.Send("Я буду присылать вам напоминания и вопросы о заполнении таблиц дважды в месяц (15-го числа и в последний день месяца). Пожалуйста, отвечайте на них с помощью кнопок 'Да' или 'Нет', которые появятся под сообщениями.\n\nЕсли вы случайно ответили 'Нет', я напомню вам через час. Если вы не ответите, я напомню на следующий день.\n\n`/help` - Показать это сообщение.")
                }
                // Inactive Teacher Help (same as unknown for commands)
                logCtx.WithField("teacher_id", userAsTeacher.ID).Info("User identified as Inactive Teacher, sending restricted help.")
                return c.Send("Ваш аккаунт преподавателя неактивен. Для получения помощи или активации обратитесь к администратору.")
            } else if err != idb.ErrTeacherNotFound {
                logCtx.WithError(err).Error("Error checking teacher status for /help command")
                return c.Send("Произошла ошибка при проверке вашего статуса. Пожалуйста, попробуйте позже.")
            }

            // Unknown User Help
            logCtx.Info("User is unknown, sending restricted help.")
            return c.Send("Доступных команд для вас нет. Если вы преподаватель и ожидаете уведомлений, пожалуйста, обратитесь к администратору для добавления вас в систему.")
        })
    }
    ```

4.  **Update `main.go` to Register Bot Commands:**
    * Call `telegram.RegisterBotCommands` after the bot, config, teacher repository, and logger are initialized.
    ```go
    // cmd/bot/main.go
    // ... (after bot, cfg, teacherRepo, logger.Log are initialized)
        // ...
        // telegram.RegisterAdminHandlers(bot, adminService, cfg.AdminTelegramID, logger.Log.WithField("handler_group", "admin"))
        // telegram.RegisterTeacherResponseHandlers(bot, notificationService, logger.Log.WithField("handler_group", "teacher_response")) // Assuming logger passed here too

        // Register general bot commands
        telegram.RegisterBotCommands(bot, cfg, teacherRepo, logger.Log.WithField("handler_group", "general_bot_commands")) //
        logger.Log.Info("Admin, Teacher Response, and General Bot command handlers registered.")
    // ...
    ```

---

**Acceptance Criteria:**

* A new `internal/infra/telegram/bot_commands_handler.go` file is created containing the `/start` and `/help` command handlers, registered via a `RegisterBotCommands` function.
* **`/start` Command Behavior:**
    * An Administrator sending `/start` receives a specific welcome message acknowledging their admin role (e.g., "Привет, Администратор [FirstName]! Я готов к работе...").
    * An active, registered Teacher sending `/start` receives a personalized welcome message (e.g., "Привет, [TeacherFirstName]! Я бот для напоминаний...").
    * An inactive, registered Teacher sending `/start` receives a message indicating their account is inactive and to contact an administrator.
    * An unknown user (neither Admin nor registered Teacher) sending `/start` receives a generic welcome and guidance to contact an administrator if they are a teacher.
* **`/help` Command Behavior:**
    * An Administrator sending `/help` receives a formatted list of available admin commands (`/add_teacher`, `/remove_teacher`, `/list_teachers`, `/help`) with brief descriptions.
    * An active, registered Teacher sending `/help` receives a message explaining the bot's purpose for them (receiving reminders, responding with buttons) and general interaction flow.
    * An inactive Teacher or an unknown user sending `/help` receives a message indicating that no specific commands are available to them or to contact an administrator for assistance/registration.
* All command handlers use the structured logger (`logrus`) with appropriate contextual information (e.g., `command`, `sender_id`, identified role).
* The `telegram.RegisterBotCommands` function is called in `main.go`, successfully registering the `/start` and `/help` handlers.
* The commands are functional as described when tested with different user roles.

---

**Critical Tests (Manual Verification):**

1.  **Admin `/start` Test:**
    * Using the configured Admin Telegram account, send `/start` to the bot.
    * **Verify:** The bot responds with the admin-specific welcome message. Logs show admin identification.
2.  **Active Teacher `/start` Test:**
    * **Setup:** Ensure an active teacher is registered in the system (use `/add_teacher` if needed).
    * Using that teacher's Telegram account, send `/start` to the bot.
    * **Verify:** The bot responds with the personalized teacher welcome message, including their first name. Logs show active teacher identification.
3.  **Inactive Teacher `/start` Test:**
    * **Setup:** Ensure a teacher is registered but marked as inactive (use `/remove_teacher`).
    * Using that teacher's Telegram account, send `/start` to the bot.
    * **Verify:** The bot responds with the message indicating their account is inactive. Logs show inactive teacher identification.
4.  **Unknown User `/start` Test:**
    * Using a Telegram account that is neither the Admin nor a registered teacher, send `/start` to the bot.
    * **Verify:** The bot responds with the generic welcome message for unknown users. Logs show unknown user identification.
5.  **Admin `/help` Test:**
    * As Admin, send `/help`.
    * **Verify:** The bot responds with a list of admin commands: `/add_teacher`, `/remove_teacher`, `/list_teachers`, `/help`, with their descriptions. The formatting should be readable (e.g., Markdown).
6.  **Active Teacher `/help` Test:**
    * As an active, registered teacher, send `/help`.
    * **Verify:** The bot responds with an explanatory message about receiving reminders and using "Да"/"Нет" buttons.
7.  **Inactive Teacher `/help` Test:**
    * As an inactive, registered teacher, send `/help`.
    * **Verify:** The bot responds with a message indicating no commands are available or to contact an administrator.
8.  **Unknown User `/help` Test:**
    * As an unknown user, send `/help`.
    * **Verify:** The bot responds with a message indicating no commands are available or to contact an administrator for registration.