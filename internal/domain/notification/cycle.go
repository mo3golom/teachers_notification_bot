// internal/domain/notification/cycle.go
package notification

import "time"

// Cycle represents a single notification run (e.g., mid-month May 2025).
// Corresponds to the 'notification_cycles' table in schema B003.
type Cycle struct {
	ID        int32     // SERIAL in DB
	CycleDate time.Time // Specific date of the cycle
	Type      CycleType // e.g., MID_MONTH, END_MONTH
	CreatedAt time.Time
}
