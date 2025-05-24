// internal/infra/telegram/client.go
package telegram

import (
	"gopkg.in/telebot.v3"
)

// TelebotAdapter implements the Client interface using the gopkg.in/telebot.v3 library.
type TelebotAdapter struct {
	bot *telebot.Bot
}

func NewTelebotAdapter(b *telebot.Bot) *TelebotAdapter {
	return &TelebotAdapter{bot: b}
}

// SendMessage sends a text message to the specified recipient.
func (tba *TelebotAdapter) SendMessage(recipientChatID int64, text string, options *telebot.SendOptions) error {
	if options == nil {
		options = &telebot.SendOptions{}
	}

	recipient := &telebot.User{ID: recipientChatID} // For teachers, it's a direct user chat
	_, err := tba.bot.Send(recipient, text, options)
	return err
}
