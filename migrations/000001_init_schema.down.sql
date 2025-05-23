-- migrations/000001_init_schema.down.sql

BEGIN;

DROP TRIGGER IF EXISTS set_timestamp_teacher_report_statuses ON teacher_report_statuses;
DROP TABLE IF EXISTS teacher_report_statuses;

DROP TRIGGER IF EXISTS set_timestamp_teachers ON teachers;
DROP TABLE IF EXISTS teachers;

DROP TABLE IF EXISTS notification_cycles;

DROP FUNCTION IF EXISTS trigger_set_timestamp();

COMMIT; 