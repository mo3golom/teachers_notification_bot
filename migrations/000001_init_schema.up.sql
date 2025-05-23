-- migrations/000001_init_schema.up.sql

BEGIN;

-- Function to automatically update 'updated_at' columns
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Teachers Table
CREATE TABLE IF NOT EXISTS teachers (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT UNIQUE NOT NULL,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255), -- Optional last name
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_teachers_telegram_id ON teachers(telegram_id);
CREATE INDEX IF NOT EXISTS idx_teachers_is_active ON teachers(is_active);

CREATE TRIGGER set_timestamp_teachers
BEFORE UPDATE ON teachers
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

-- Notification Cycles Table
-- Stores information about each notification run (e.g., mid-month, end-month)
CREATE TABLE IF NOT EXISTS notification_cycles (
    id SERIAL PRIMARY KEY,
    -- Date for which the cycle is relevant, e.g., 2023-05-15 or 2023-05-31
    cycle_date DATE NOT NULL,
    -- Type of cycle, e.g., 'MID_MONTH', 'END_MONTH'
    cycle_type VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
    -- No updated_at needed if records are immutable after creation
);

CREATE INDEX IF NOT EXISTS idx_notification_cycles_cycle_date_type ON notification_cycles(cycle_date, cycle_type);

-- Teacher Report Statuses Table
-- Tracks the status of each report type for each teacher within a specific notification cycle
CREATE TABLE IF NOT EXISTS teacher_report_statuses (
    id BIGSERIAL PRIMARY KEY,
    teacher_id BIGINT NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
    cycle_id INTEGER NOT NULL REFERENCES notification_cycles(id) ON DELETE CASCADE,
    -- Key identifying the report/table, e.g., 'TABLE_1_LESSONS', 'TABLE_2_OTV', 'TABLE_3_SCHEDULE'
    report_key VARCHAR(100) NOT NULL,
    -- Status of the report: 'PENDING_QUESTION', 'ANSWERED_YES', 'ANSWERED_NO',
    -- 'AWAITING_REMINDER_1H', 'AWAITING_REMINDER_NEXT_DAY', 'FINAL_CONFIRMED_CYCLE'
    status VARCHAR(50) NOT NULL,
    last_notified_at TIMESTAMPTZ, -- When the last notification/reminder for this specific report was sent
    response_attempts INTEGER DEFAULT 0 NOT NULL, -- Number of times a reminder was sent or response was 'No'
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT teacher_cycle_report_unique UNIQUE (teacher_id, cycle_id, report_key)
);

CREATE INDEX IF NOT EXISTS idx_teacher_report_statuses_teacher_cycle ON teacher_report_statuses(teacher_id, cycle_id);
CREATE INDEX IF NOT EXISTS idx_teacher_report_statuses_status ON teacher_report_statuses(status);

CREATE TRIGGER set_timestamp_teacher_report_statuses
BEFORE UPDATE ON teacher_report_statuses
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

COMMIT; 