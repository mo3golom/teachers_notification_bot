// internal/infra/telegram/teacher_response_handlers.go
package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"teacher_notification_bot/internal/app" // For NotificationService interface

	"gopkg.in/telebot.v3"
)

func RegisterTeacherResponseHandlers(ctx context.Context, b *telebot.Bot, notificationService app.NotificationService) {
	b.Handle(telebot.OnCallback, func(c telebot.Context) error {
		data := c.Callback().Data

		if strings.HasPrefix(data, "ans_yes_") {
			parts := strings.Split(data, "_") // ans_yes_123
			if len(parts) != 3 {
				c.Bot().OnError(fmt.Errorf("invalid callback data format for 'yes': %s", data), c)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка обработки ответа."})
			}
			reportStatusIDStr := parts[2]
			reportStatusID, err := strconv.ParseInt(reportStatusIDStr, 10, 64)
			if err != nil {
				c.Bot().OnError(fmt.Errorf("invalid reportStatusID '%s' in callback: %w", reportStatusIDStr, err), c)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка ID отчета."})
			}

			err = notificationService.ProcessTeacherYesResponse(ctx, reportStatusID)
			if err != nil {
				c.Bot().OnError(fmt.Errorf("error processing 'Yes' response for statusID %d: %w", reportStatusID, err), c)
				return c.Respond(&telebot.CallbackResponse{Text: "Произошла ошибка."})
			}
			return c.Respond(&telebot.CallbackResponse{Text: "Ответ 'Да' принят!"})

		} else if strings.HasPrefix(data, "ans_no_") {
			parts := strings.Split(data, "_") // ans_no_123
			if len(parts) != 3 {
				c.Bot().OnError(fmt.Errorf("invalid callback data format for 'no': %s", data), c)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка обработки ответа."})
			}
			reportStatusIDStr := parts[2]
			reportStatusID, err := strconv.ParseInt(reportStatusIDStr, 10, 64)
			if err != nil {
				c.Bot().OnError(fmt.Errorf("invalid reportStatusID '%s' in 'no' callback: %w", reportStatusIDStr, err), c)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка ID отчета."})
			}
			err = notificationService.ProcessTeacherNoResponse(ctx, reportStatusID)
			if err != nil {
				c.Bot().OnError(fmt.Errorf("error processing 'No' response for statusID %d: %w", reportStatusID, err), c)
				return c.Respond(&telebot.CallbackResponse{Text: "Произошла ошибка."})
			}
			// The service sends the textual "Понял(а)..." message.
			// Callback an ack to remove the "processing" state on the button.
			return c.Respond() // Minimal ack, or use text "Напоминание установлено."
		}

		// Fallback for unhandled callbacks by this specific handler.
		c.Bot().OnError(fmt.Errorf("unhandled callback data by teacher_response_handler: %s", data), c)
		return c.Respond(&telebot.CallbackResponse{Text: "Неизвестное действие."})
	})
}
