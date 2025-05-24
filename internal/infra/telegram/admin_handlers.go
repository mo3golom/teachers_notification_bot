package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"teacher_notification_bot/internal/app"
	teacher "teacher_notification_bot/internal/domain/teacher"
	idb "teacher_notification_bot/internal/infra/database"

	"github.com/sirupsen/logrus"
	"gopkg.in/telebot.v3"
)

// RegisterAdminHandlers registers handlers for admin commands.
// It requires the bot instance, admin service, and the configured admin Telegram ID.
func RegisterAdminHandlers(ctx context.Context, b *telebot.Bot, adminService *app.AdminService, adminTelegramID int64, baseLogger *logrus.Entry) {
	b.Handle("/add_teacher", func(c telebot.Context) error {
		handlerLogger := baseLogger.WithFields(logrus.Fields{
			"handler":   "/add_teacher",
			"sender_id": c.Sender().ID,
		})
		handlerLogger.Info("Command received")

		if c.Sender().ID != adminTelegramID {
			handlerLogger.Warn("Unauthorized access attempt")
			return c.Send("Ошибка: У вас нет прав для выполнения этой команды.") // Unauthorized
		}

		args := c.Args() // c.Args() returns []string
		// Expected format: /add_teacher <TelegramID> <FirstName> [LastName]
		if len(args) < 2 || len(args) > 3 {
			handlerLogger.WithField("args_count", len(args)).Warn("Invalid command format")
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

		handlerLogger = handlerLogger.WithFields(logrus.Fields{
			"teacher_telegram_id": teacherTelegramID,
			"first_name":          firstName,
			"last_name":           lastName,
		})

		newTeacher, err := adminService.AddTeacher(ctx, c.Sender().ID, teacherTelegramID, firstName, lastName)
		if err != nil {
			logWithError := handlerLogger.WithError(err)
			switch err {
			case app.ErrAdminNotAuthorized: // This check is technically redundant here due to the initial sender check
				logWithError.Warn("Admin not authorized (service level)")
				return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
			case app.ErrTeacherAlreadyExists:
				logWithError.Warn("Teacher already exists")
				return c.Send(fmt.Sprintf("Ошибка: Преподаватель с Telegram ID %d уже существует.", teacherTelegramID))
			default:
				logWithError.Error("Failed to add teacher")
				return c.Send(fmt.Sprintf("Произошла ошибка при добавлении преподавателя: %s", err.Error()))
			}
		}

		handlerLogger.WithFields(logrus.Fields{
			"new_teacher_id": newTeacher.ID,
		}).Info("Teacher added successfully")

		successMsg := fmt.Sprintf("Преподаватель %s (ID: %d) успешно добавлен.", newTeacher.FirstName, newTeacher.TelegramID)
		if newTeacher.LastName.Valid {
			successMsg = fmt.Sprintf("Преподаватель %s %s (ID: %d) успешно добавлен.", newTeacher.FirstName, newTeacher.LastName.String, newTeacher.TelegramID)
		}
		return c.Send(successMsg)
	})

	b.Handle("/remove_teacher", func(c telebot.Context) error {
		handlerLogger := baseLogger.WithFields(logrus.Fields{
			"handler":   "/remove_teacher",
			"sender_id": c.Sender().ID,
		})
		handlerLogger.Info("Command received")

		if c.Sender().ID != adminTelegramID {
			handlerLogger.Warn("Unauthorized access attempt")
			return c.Send("Ошибка: У вас нет прав для выполнения этой команды.") // Unauthorized
		}

		args := c.Args() // c.Args() returns []string
		// Expected format: /remove_teacher <TelegramID>
		if len(args) != 1 {
			return c.Send("Неверный формат команды. Используйте: /remove_teacher <TelegramID>")
		}

		teacherTelegramID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			handlerLogger.WithField("arg", args[0]).Warn("Invalid Telegram ID format")
			return c.Send("Ошибка: Telegram ID должен быть числом.")
		}
		handlerLogger = handlerLogger.WithField("teacher_telegram_id", teacherTelegramID)

		removedTeacher, err := adminService.RemoveTeacher(ctx, c.Sender().ID, teacherTelegramID)
		if err != nil {
			logWithError := handlerLogger.WithError(err)
			switch err {
			case app.ErrAdminNotAuthorized: // Redundant here
				logWithError.Warn("Admin not authorized (service level)")
				return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
			case idb.ErrTeacherNotFound:
				logWithError.Warn("Teacher to remove not found")
				return c.Send(fmt.Sprintf("Преподаватель с таким Telegram ID %d не найден.", teacherTelegramID))
			case app.ErrTeacherAlreadyInactive:
				logWithError.Warn("Teacher already inactive")
				if removedTeacher != nil {
					return c.Send(fmt.Sprintf("Преподаватель %s %s (ID: %d) уже был деактивирован.", removedTeacher.FirstName, removedTeacher.LastName.String, removedTeacher.TelegramID))
				}
				return c.Send(fmt.Sprintf("Преподаватель с Telegram ID %d уже был деактивирован.", teacherTelegramID))
			default:
				logWithError.Error("Failed to remove teacher")
				return c.Send(fmt.Sprintf("Произошла ошибка при удалении преподавателя: %s", err.Error()))
			}
		}

		handlerLogger.WithFields(logrus.Fields{
			"removed_teacher_id": removedTeacher.ID,
		}).Info("Teacher removed (deactivated) successfully")

		var teacherName strings.Builder
		teacherName.WriteString(removedTeacher.FirstName)
		if removedTeacher.LastName.Valid && removedTeacher.LastName.String != "" {
			teacherName.WriteString(" ")
			teacherName.WriteString(removedTeacher.LastName.String)
		}
		return c.Send(fmt.Sprintf("Преподаватель %s (ID: %d) успешно деактивирован.", teacherName.String(), removedTeacher.TelegramID))
	})

	b.Handle("/list_teachers", func(c telebot.Context) error {
		handlerLogger := baseLogger.WithFields(logrus.Fields{
			"handler":   "/list_teachers",
			"sender_id": c.Sender().ID,
		})
		if c.Sender().ID != adminTelegramID {
			handlerLogger.Warn("Unauthorized access attempt")
			return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
		}

		args := c.Args() // c.Args() returns []string
		// Optional argument: 'active' or 'all'
		listType := "active" // Default to active
		if len(args) > 0 {
			listType = strings.ToLower(args[0])
		}
		handlerLogger = handlerLogger.WithField("list_type", listType)

		var teachersList []*teacher.Teacher
		var err error
		var title string

		switch listType {
		case "active":
			title = "Активные преподаватели"
			teachersList, err = adminService.ListActiveTeachers(ctx, c.Sender().ID)
		case "all":
			title = "Все преподаватели"
			teachersList, err = adminService.ListAllTeachers(ctx, c.Sender().ID)
		default:
			handlerLogger.Warn("Invalid list type argument")
			return c.Send("Неверный аргумент. Используйте 'active' или 'all', или оставьте пустым для отображения активных преподавателей.")
		}

		if err != nil {
			logWithError := handlerLogger.WithError(err)
			if err == app.ErrAdminNotAuthorized {
				logWithError.Warn("Admin not authorized (service level)")
				return c.Send("Ошибка: У вас нет прав для выполнения этой команды.")
			}
			logWithError.Error("Failed to get list of teachers")
			return c.Send(fmt.Sprintf("Произошла ошибка при получении списка преподавателей: %s", err.Error()))
		}

		if len(teachersList) == 0 {
			handlerLogger.Info("No teachers found for the specified list type")
			if listType == "active" {
				return c.Send("Активных преподавателей не найдено.")
			}
			return c.Send("Список преподавателей пуст.")
		}

		handlerLogger.WithField("teachers_count", len(teachersList)).Info("Successfully retrieved teacher list")

		var response strings.Builder
		response.WriteString(fmt.Sprintf("---	%s	---\n", title))
		for _, t := range teachersList {
			status := "Деактивирован"
			if t.IsActive {
				status = "Активен"
			}
			response.WriteString(fmt.Sprintf("ID: %d, Telegram ID: %d, Имя: %s, Фамилия: %s, Статус: %s\n",
				t.ID,
				t.TelegramID,
				t.FirstName,
				t.LastName.String,
				status))
		}
		return c.Send(response.String())
	})
}
