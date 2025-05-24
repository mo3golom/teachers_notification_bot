package teacher

import (
	"database/sql"
	"time"
)

// Teacher represents a teacher in the system.
type Teacher struct {
	ID         int64
	TelegramID int64
	FirstName  string
	LastName   sql.NullString // To handle optional last name
	IsActive   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
