// internal/infra/telegram/teacher_response_handlers.go
package telegram

import (
	"context"
	"strconv"
	"strings"
	"teacher_notification_bot/internal/app" // For NotificationService interface

	"github.com/sirupsen/logrus"
	"gopkg.in/telebot.v3"
)

func RegisterTeacherResponseHandlers(ctx context.Context, b *telebot.Bot, notificationService app.NotificationService, baseLogger *logrus.Entry) {
	b.Handle(telebot.OnCallback, func(c telebot.Context) error {
		callback := c.Callback()
		if callback == nil {
			baseLogger.Error("Callback object is nil in OnCallback handler")
			return c.Respond(&telebot.CallbackResponse{Text: "Ошибка: Некорректный запрос."})
		}
		data := callback.Data
		data = strings.TrimSpace(data) // Trim leading/trailing whitespace

		handlerLogger := baseLogger.WithFields(logrus.Fields{
			"handler":       "teacher_response_callback",
			"sender_id":     c.Sender().ID,
			"callback_data": data,
		})
		handlerLogger.Info("Callback received")

		if strings.HasPrefix(data, "ans_yes_") {
			parts := strings.Split(data, "_") // ans_yes_123
			if len(parts) != 3 {
				handlerLogger.Errorf("Invalid callback data format for 'yes': %s", data)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка обработки ответа."})
			}
			reportStatusIDStr := parts[2]
			reportStatusID, err := strconv.ParseInt(reportStatusIDStr, 10, 64)
			if err != nil {
				handlerLogger.WithError(err).Errorf("Invalid reportStatusID '%s' in 'yes' callback", reportStatusIDStr)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка ID отчета."})
			}
			handlerLogger = handlerLogger.WithField("report_status_id", reportStatusID)

			err = notificationService.ProcessTeacherYesResponse(ctx, reportStatusID)
			if err != nil {
				handlerLogger.WithError(err).Error("Error processing 'Yes' response")
				return c.Respond(&telebot.CallbackResponse{Text: "Произошла ошибка."})
			}
			handlerLogger.Info("Successfully processed 'Yes' response")
			return c.Respond(&telebot.CallbackResponse{Text: "Ответ 'Да' принят!"})

		} else if strings.HasPrefix(data, "ans_no_") {
			parts := strings.Split(data, "_") // ans_no_123
			if len(parts) != 3 {
				handlerLogger.Errorf("Invalid callback data format for 'no': %s", data)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка обработки ответа."})
			}
			reportStatusIDStr := parts[2]
			reportStatusID, err := strconv.ParseInt(reportStatusIDStr, 10, 64)
			if err != nil {
				handlerLogger.WithError(err).Errorf("Invalid reportStatusID '%s' in 'no' callback", reportStatusIDStr)
				return c.Respond(&telebot.CallbackResponse{Text: "Ошибка ID отчета."})
			}
			handlerLogger = handlerLogger.WithField("report_status_id", reportStatusID)

			err = notificationService.ProcessTeacherNoResponse(ctx, reportStatusID)
			if err != nil {
				handlerLogger.WithError(err).Error("Error processing 'No' response")
				return c.Respond(&telebot.CallbackResponse{Text: "Произошла ошибка."})
			}
			// The service sends the textual "Понял(а)..." message.
			handlerLogger.Info("Successfully processed 'No' response")
			return c.Respond(&telebot.CallbackResponse{Text: ""}) // Respond with empty text to dismiss loading, service sends the actual reply
		}

		// Fallback for unhandled callbacks by this specific handler.
		handlerLogger.Warnf("Unhandled callback data by teacher_response_handler: %s", data)
		return c.Respond(&telebot.CallbackResponse{Text: "Неизвестное действие."})
	})
}
