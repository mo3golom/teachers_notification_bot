// internal/domain/notification/shared_types.go
package notification

// ReportKey identifies the specific table/report being queried.
type ReportKey string

const (
	ReportKeyTable1Lessons  ReportKey = "TABLE_1_LESSONS"  // FR3.1, FR3.2 [cite: 58, 60]
	ReportKeyTable3Schedule ReportKey = "TABLE_3_SCHEDULE" // FR3.1, FR3.2 [cite: 59, 61]
	ReportKeyTable2OTV      ReportKey = "TABLE_2_OTV"      // FR3.2 (only end of month) [cite: 62]
)

// InteractionStatus represents the state of a teacher's response to a report query.
type InteractionStatus string // FR6.1 [cite: 72]

const (
	StatusPendingQuestion         InteractionStatus = "PENDING_QUESTION"
	StatusAnsweredYes             InteractionStatus = "ANSWERED_YES"
	StatusAnsweredNo              InteractionStatus = "ANSWERED_NO"
	StatusAwaitingReminder1H      InteractionStatus = "AWAITING_REMINDER_1H"       // FR4.2 [cite: 66, 67]
	StatusAwaitingReminderNextDay InteractionStatus = "AWAITING_REMINDER_NEXT_DAY" // FR4.3 [cite: 68]
	// StatusCycleFullyConfirmed might be a status for the teacher overall, rather than per report.
	// For now, individual report statuses cover FR6.1 [cite: 72]
)

// CycleType indicates if the notification cycle is for mid-month or end-of-month.
type CycleType string

const (
	CycleTypeMidMonth CycleType = "MID_MONTH" // For 15th of month notifications [cite: 14, 56]
	CycleTypeEndMonth CycleType = "END_MONTH" // For last day of month notifications [cite: 14, 57]
)
