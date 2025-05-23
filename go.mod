module teacher_notification_bot

go 1.24

require (
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	github.com/robfig/cron/v3 v3.0.1
	github.com/sirupsen/logrus v1.9.3
	gopkg.in/tucnak/telebot.v3 v3.2.1
)

// Indirect dependencies would be listed here by `go mod tidy`
// For example, logrus might require golang.org/x/sys
