## Backend Task: B014 - Ensure Clean State Management and Isolation for New Notification Cycles

**Objective:**
To meticulously review, verify, and (if necessary) refine the system's mechanisms for handling the transition to new notification cycles (e.g., from a May cycle to a June cycle, or from mid-month to end-of-month). The goal is to confirm that new cycles start with a fresh set of `TeacherReportStatus` records for all active teachers, and that all ongoing bot operations (sending questions, processing responses, reminders) are correctly scoped to the current active notification cycle, effectively "archiving" data from previous, completed cycles by not operating on it.

**Background:**
The PRD (FR6.2) requires that "Состояние должно сбрасываться или архивироваться перед началом нового цикла уведомлений." Our current architecture, which uses a `cycle_id` to link `TeacherReportStatus` records to a specific `NotificationCycle`, provides logical separation and history preservation. This task ensures this design is robustly implemented and that new cycles initiate cleanly, without interference or data bleed from prior cycles.

**Tech Stack:**
* Go version: 1.24
* PostgreSQL
* Cron Library: `github.com/robfig/cron/v3`
* Existing components: `NotificationService`, `NotificationRepository`, `TeacherRepository`, Schedulers.

---

**Steps to Completion:**

1.  **Review `NotificationServiceImpl.InitiateNotificationProcess` (from Task B010):**
    * **Verification Point 1.1 (New Cycle Creation):** Confirm that when `InitiateNotificationProcess` is called for a new cycle instance (unique `cycleDate` and `cycleType` combination):
        * It reliably calls `notificationRepo.GetCycleByDateAndType()` first.
        * If `ErrCycleNotFound` is returned, it proceeds to create a *new* `NotificationCycle` record in the database.
        * If an existing cycle for that exact date and type is found, it uses that existing `CycleID`. This ensures idempotency for the cycle creation part itself if the scheduler trigger runs multiple times close together for the same conceptual cycle event.
    * **Verification Point 1.2 (Fresh Status Records):** Confirm that, for the determined `currentCycle.ID` (whether newly created or existing for that date/type):
        * It fetches *all currently active teachers*.
        * It iterates through these teachers and the `reportsForCycle` (determined by `cycleType`).
        * For each `teacher` and `reportKey` combination, it *attempts* to create a *new* `TeacherReportStatus` record linked to the `currentCycle.ID`.
        * Crucially, verify the logic that checks if a status *already exists* for that specific `teacher.ID`, `currentCycle.ID`, and `reportKey` (added in B010 for idempotency: `s.notifRepo.GetReportStatus(ctx, t.ID, currentCycle.ID, reportKey)`). This ensures that if the process restarts for the *same ongoing new cycle*, it doesn't create duplicate status entries but rather would pick up existing ones if they were partially processed.
        * Newly created statuses must start with `StatusPendingQuestion` and null/cleared reminder fields.

2.  **Review Teacher Response Handlers (`ProcessTeacherYesResponse`, `ProcessTeacherNoResponse` - from Tasks B011, B012):**
    * **Verification Point 2.1 (Scoped Operations):** Confirm that these methods operate on `TeacherReportStatus` records fetched by their unique `reportStatusID`. Since `reportStatusID` is a global primary key, actions are inherently tied to the specific status record, which in turn is tied to a specific `CycleID`.
    * **Verification Point 2.2 (Scoped Logic):** When determining "next steps" (e.g., `determineNextReportKey`) or checking if "all reports confirmed" (`AreAllReportsConfirmedForTeacher`), ensure these helper functions or repository calls are always made with the `CycleID` obtained from the `TeacherReportStatus` being processed. This ensures the logic is confined to the context of the cycle the teacher is currently interacting with. (This was generally part of their design).

3.  **Review Reminder Logic (`ProcessScheduled1HourReminders`, `ProcessNextDayReminders` - from Tasks B012, B013):**
    * **Verification Point 3.1 (Targeting Relevant Statuses):**
        * Confirm that `ListDueReminders` (for 1-hour) correctly fetches statuses based on `status = StatusAnsweredNo` and `remind_at <= now`. Since `remind_at` is only set when a "No" is processed for a specific cycle's status, this should naturally be cycle-specific.
        * Confirm that `ListStalledStatusesFromPreviousDay` (for next-day) fetches statuses based on `last_notified_at` being within the "previous day" range and `status` being `StatusPendingQuestion` or `StatusAwaitingReminder1H`. This time-bound query is key to ensuring it doesn't pick up very old, unresolved statuses from many cycles ago.
    * **Verification Point 3.2 (No Cross-Cycle Interference):** Because reminder generation relies on specific statuses (`StatusAnsweredNo` for 1-hour, `StatusPendingQuestion`/`StatusAwaitingReminder1H` for next-day) and relevant timestamps, new cycles starting with fresh `StatusPendingQuestion` statuses should not be mistakenly targeted by reminder logic meant for older, different statuses from previous cycles.

4.  **Test Procedures for Cycle Transitions (Documentation & Manual Execution Plan):**
    * Document a clear set of manual test cases specifically designed to simulate and verify behavior across cycle boundaries. These tests are crucial for this task. (Covered in "Critical Tests" below).

5.  **Internal Documentation Update:**
    * Write a brief internal note or update existing architecture documentation explaining how FR6.2 ("State reset/archival") is achieved through the creation of new, isolated `TeacherReportStatus` records for each new `NotificationCycle`. Emphasize that historical data is preserved but operations are focused on the current/active cycle's statuses.

---

**Acceptance Criteria:**

* **AC1:** Code review of `NotificationServiceImpl.InitiateNotificationProcess` confirms:
    * A new `NotificationCycle` record is correctly created if one does not already exist for the precise `cycleDate` and `cycleType`.
    * A comprehensive set of new `TeacherReportStatus` records (initially `StatusPendingQuestion`) is created for all active teachers, for all relevant `ReportKey`s, correctly linked to the new/current `NotificationCycle.ID`.
    * Idempotency is maintained: if `InitiateNotificationProcess` is called again for the *same* new cycle instance (e.g., due to a quick scheduler restart), it does not create duplicate `NotificationCycle` or `TeacherReportStatus` entries but can resume or verify existing ones for that specific new cycle.
* **AC2:** Code review of teacher response handlers (`ProcessTeacherYesResponse`, `ProcessTeacherNoResponse`) confirms that all state updates and logic for determining next steps are strictly scoped to the `CycleID` of the `TeacherReportStatus` being processed.
* **AC3:** Code review of reminder processing logic (`ProcessScheduled1HourReminders`, `ProcessNextDayReminders`) confirms that queries for fetching statuses eligible for reminders (e.g., `ListDueReminders`, `ListStalledStatusesFromPreviousDay`) are constructed to target only relevant statuses from appropriate timeframes/states, preventing unintended reminders for very old or already completed/superseded cycles.
* **AC4:** Successful execution of the "Critical Tests for Cycle Transitions" (detailed below) demonstrates that the system correctly handles the start of new notification cycles, isolates their state, and ensures teachers receive the correct sequence of prompts for the new cycle, irrespective of their status in any previous cycle.
* **AC5:** A brief internal documentation note clarifying the "state reset/archival" mechanism (as per FR6.2) is produced.

---

**Critical Tests (Manual, Focused on Cycle Transitions):**

These tests require the ability to simulate scheduler triggers or manually invoke `InitiateNotificationProcess` for specific dates/types.

1.  **Test Case: Full Mid-Month Cycle Completion, then Clean Start of End-Month Cycle (Same Month):**
    * **Setup:**
        * Have at least one active teacher (e.g., Teacher A).
        * Simulate the scheduler trigger for "15th of current virtual month" (e.g., May 15th) by calling `InitiateNotificationProcess(ctx, notification.CycleTypeMidMonth, may15thDate)`.
        * Interact as Teacher A: Answer "Да" to "Таблица 1", then "Да" to "Таблица 3".
        * **Verify Mid-Cycle Completion:** Manager is notified for Teacher A for May mid-month. Teacher A receives final confirmation. All statuses for Teacher A in the May 15th cycle are `StatusAnsweredYes`.
    * **Action:**
        * Simulate the scheduler trigger for "last day of current virtual month" (e.g., May 31st) by calling `InitiateNotificationProcess(ctx, notification.CycleTypeEndMonth, may31stDate)`.
    * **Verify New End-Month Cycle:**
        * A *new* `NotificationCycle` record exists for `may31stDate` with type `END_MONTH`.
        * *New* `TeacherReportStatus` records exist for Teacher A, linked to this new (May 31st) cycle ID, for `TABLE_1_LESSONS`, `TABLE_3_SCHEDULE`, and `TABLE_2_OTV`. All these new statuses should be `StatusPendingQuestion`.
        * **Telegram (Teacher A):** Teacher A receives the question for "Таблица 1" *again*, this time clearly associated with the new end-of-month cycle (the callback data on its buttons will point to the new status IDs).
        * No interference from the completed May 15th cycle statuses.

2.  **Test Case: Incomplete Previous Cycle, then Clean Start of New Cycle:**
    * **Setup:**
        * Have an active teacher (e.g., Teacher B).
        * Trigger `InitiateNotificationProcess` for "May 15th" (`MID_MONTH`). Teacher B receives "Таблица 1" question.
        * Teacher B clicks "Нет". Status for Table 1 becomes `StatusAnsweredNo`, `remind_at` is set.
        * (Optional) Let the 1-hour reminder trigger and send. Status becomes `StatusAwaitingReminder1H`. Teacher B *does not respond further* to this May 15th cycle's Table 1 question.
    * **Action:**
        * Trigger `InitiateNotificationProcess` for "May 31st" (`END_MONTH`).
    * **Verify New End-Month Cycle:**
        * Same verifications as Test 1 for the new May 31st cycle: New cycle record, new `TeacherReportStatus` records for Teacher B (for Table 1, 2, 3 - all `StatusPendingQuestion`), and Teacher B receives the "Таблица 1" question for this *new* May 31st cycle.
    * **Verify Isolation for Reminders:**
        * If the next-day reminder job for "May 16th" runs (related to the May 15th cycle's Table 1 non-response), it should process the May 15th cycle's status for Teacher B.
        * This should *not* affect or be confused with the new `StatusPendingQuestion` for Table 1 in the May 31st cycle. The reminder logic's queries (e.g., `ListStalledStatusesFromPreviousDay`) are based on `last_notified_at` dates and specific statuses, which should correctly isolate the May 15th cycle's stalled status.

3.  **Test Case: Transition to a New Month (e.g., End-of-May to Mid-June):**
    * **Setup:**
        * Have an active teacher (e.g., Teacher C).
        * Successfully complete (or leave partially incomplete) an "End-of-May" cycle for Teacher C.
    * **Action:**
        * Simulate time passing to June 15th.
        * Trigger `InitiateNotificationProcess` for "June 15th" (`MID_MONTH`) by calling `InitiateNotificationProcess(ctx, notification.CycleTypeMidMonth, june15thDate)`.
    * **Verify New June Cycle:**
        * A new `NotificationCycle` record for June 15th (`MID_MONTH`) is created.
        * New `TeacherReportStatus` records for Teacher C, linked to this June 15th cycle, are created (for `TABLE_1_LESSONS`, `TABLE_3_SCHEDULE` - all `StatusPendingQuestion`).
        * **Telegram (Teacher C):** Teacher C receives the "Таблица 1" question for the new June 15th cycle.
        * Any pending reminders or unresolved statuses from the May cycle should not interfere with the processing of the new June cycle.