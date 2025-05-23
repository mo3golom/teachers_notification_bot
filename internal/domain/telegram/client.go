package telegram

import "gopkg.in/telebot.v3"

// Client defines an interface for sending messages via a Telegram bot.
// This helps in decoupling the application logic from the specific bot library.
type Client interface {
	SendMessage(recipientChatID int64, text string, options *telebot.SendOptions) error
}
