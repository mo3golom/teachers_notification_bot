package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"teacher_notification_bot/internal/app" // Your app service package
	idb "teacher_notification_bot/internal/infra/database"

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

		newTeacher, err := adminService.AddTeacher(context.Background(), c.Sender().ID, teacherTelegramID, firstName, lastName)
		if err != nil {
			switch err {
			case app.ErrAdminNotAuthorized: // This check is technically redundant here due to the initial sender check
				return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
			case app.ErrTeacherAlreadyExists:
				return c.Send(fmt.Sprintf("Ошибка: Преподаватель с Telegram ID %d уже существует.", teacherTelegramID))
			default:
				c.Bot().OnError(err, c) // Log the full error for internal review
				return c.Send(fmt.Sprintf("Произошла ошибка при добавлении преподавателя: %s", err.Error()))
			}
		}

		successMsg := fmt.Sprintf("Преподаватель %s (ID: %d) успешно добавлен.", newTeacher.FirstName, newTeacher.TelegramID)
		if newTeacher.LastName.Valid {
			successMsg = fmt.Sprintf("Преподаватель %s %s (ID: %d) успешно добавлен.", newTeacher.FirstName, newTeacher.LastName.String, newTeacher.TelegramID)
		}
		return c.Send(successMsg)
	})

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

		removedTeacher, err := adminService.RemoveTeacher(context.Background(), c.Sender().ID, teacherTelegramID)
		if err != nil {
			switch err {
			case app.ErrAdminNotAuthorized: // Redundant here
				return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
			case idb.ErrTeacherNotFound:
				return c.Send(fmt.Sprintf("Преподаватель с таким Telegram ID %d не найден.", teacherTelegramID))
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

		var teacherName strings.Builder
		teacherName.WriteString(removedTeacher.FirstName)
		if removedTeacher.LastName.Valid && removedTeacher.LastName.String != "" {
			teacherName.WriteString(" ")
			teacherName.WriteString(removedTeacher.LastName.String)
		}
		successMsg := fmt.Sprintf("Преподаватель %s (ID: %d) успешно удален (деактивирован).", teacherName.String(), removedTeacher.TelegramID)
		return c.Send(successMsg)
	})
}
