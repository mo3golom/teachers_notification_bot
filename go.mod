module teacher_notification_bot

go 1.24

require (
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	github.com/robfig/cron/v3 v3.0.1
	github.com/sirupsen/logrus v1.9.3
	gopkg.in/telebot.v3 v3.2.1
)

require golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect

// Indirect dependencies would be listed here by `go mod tidy`
// For example, logrus might require golang.org/x/sys
