sequenceDiagram
    participant DevTeam as Development Team
    autonumber

    activate DevTeam
    DevTeam->>DevTeam: B001: Project Setup & Dockerization (completed)
    DevTeam->>DevTeam: B002: Configuration Module Implementation (completed)
    DevTeam->>DevTeam: B003: DB Schema & Initial Migration Setup (completed)
    DevTeam->>DevTeam: B004: Teacher Domain Entity & Repository (completed)
    DevTeam->>DevTeam: B005: Admin - Add Teacher Functionality (completed)
    DevTeam->>DevTeam: B006: Admin - Remove Teacher Functionality (completed)
    DevTeam->>DevTeam: B007: Admin - List Teachers Functionality (completed)
    DevTeam->>DevTeam: B008: NotificationCycle & TeacherReportStatus Domain & Repos (completed)
    DevTeam->>DevTeam: B009: Core Notification Scheduler Setup (completed)
    DevTeam->>DevTeam: B010: Initial Notification Sending Logic (First Question) (completed)
    DevTeam->>DevTeam: B011: Handling Teacher's "Yes" Response (completed)
    DevTeam->>DevTeam: B012: Handling Teacher's "No" Response & 1-Hour Reminder (completed)
    DevTeam->>DevTeam: B013: Next-Day Reminder Logic (completed)
    DevTeam->>DevTeam: B014: New Cycle State Management Verification (completed)
    DevTeam->>DevTeam: B015: Structured Logging Implementation
    DevTeam->>DevTeam: B016: Implement /start and /help Commands
    deactivate DevTeam