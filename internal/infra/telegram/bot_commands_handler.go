// internal/infra/telegram/bot_commands_handler.go
package telegram

import (
	"context"
	"fmt"
	"strings"
	"teacher_notification_bot/internal/domain/teacher"
	"teacher_notification_bot/internal/infra/config"
	idb "teacher_notification_bot/internal/infra/database" // For ErrTeacherNotFound

	"github.com/sirupsen/logrus"
	"gopkg.in/telebot.v3"
)

func RegisterBotCommands(
	ctx context.Context,
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
		userAsTeacher, err := teacherRepo.GetByTelegramID(ctx, senderID)
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

	b.Handle("/help", func(c telebot.Context) error {
		senderID := c.Sender().ID
		logCtx := startHelpLogger.WithField("command", "/help").WithField("sender_id", senderID)
		logCtx.Info("Processing /help command")

		// Admin Help
		if senderID == cfg.AdminTelegramID {
			logCtx.Info("User identified as Admin, sending admin help.")
			var helpText strings.Builder
			helpText.WriteString("Доступные команды Администратора:\n\n")
			helpText.WriteString("`/add_teacher <TelegramID> <Имя> [Фамилия]`\n - Добавить нового преподавателя в систему.\n\n")
			helpText.WriteString("`/remove_teacher <TelegramID>`\n - Деактивировать преподавателя (он перестанет получать уведомления).\n\n")
			helpText.WriteString("`/list_teachers [active|all]`\n - Показать список преподавателей. По умолчанию показывает активных. 'all' - для всех.\n\n")
			helpText.WriteString("`/help`\n - Показать это справочное сообщение.")
			return c.Send(helpText.String(), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
		}

		// Teacher Help
		userAsTeacher, err := teacherRepo.GetByTelegramID(ctx, senderID)
		if err == nil {
			if userAsTeacher.IsActive {
				logCtx.WithField("teacher_id", userAsTeacher.ID).Info("User identified as Active Teacher, sending teacher help.")
				return c.Send("Я буду присылать вам напоминания и вопросы о заполнении таблиц дважды в месяц (15-го числа и в последний день месяца). Пожалуйста, отвечайте на них с помощью кнопок 'Да' или 'Нет', которые появятся под сообщениями.\n\nЕсли вы случайно ответили 'Нет', я напомню вам через час. Если вы не ответите, я напомню на следующий день.\n\n`/help` - Показать это сообщение.")
			}
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
