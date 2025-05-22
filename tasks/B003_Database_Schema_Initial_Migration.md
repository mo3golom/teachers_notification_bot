## Backend Task: B003 - Define Database Schema and Set Up Initial Migration

**Objective:**
To define the SQL schema for the core database tables (`teachers`, `notification_cycles`, `teacher_report_statuses`) as outlined in the PRD [cite: 87] and our architectural plan. This task also involves setting up a database migration tool (`golang-migrate/migrate`) and creating the initial migration files to establish this schema within the PostgreSQL database.

**Background:**
A well-structured database schema is fundamental for the application's data integrity and persistence[cite: 84]. Database migrations provide a version-controlled and systematic way to apply and roll back schema changes, which is essential for development and deployment workflows.

**Tech Stack:**
* Go version: 1.24
* Database: PostgreSQL
* Migration Tool: `golang-migrate/migrate`

**Key Requirements from PRD:**
* Data persistence in PostgreSQL[cite: 84].
* Specific schemas for `teachers`, `notification_cycles`, `teacher_statuses` (renamed `teacher_report_statuses` for clarity)[cite: 87].
* Storage of teacher information (Telegram ID, name, active status)[cite: 21, 52].
* Storage of notification cycle information (date, type)[cite: 87].
* Storage of teacher status per table per cycle (pending, answered, awaiting reminder)[cite: 20, 72].

---

**Steps to Completion:**

1.  **Install `golang-migrate/migrate` CLI:**
    * If not already installed, install the `golang-migrate/migrate` CLI tool. You can find installation instructions on their official GitHub repository (usually involves downloading a pre-compiled binary for your OS or using `go install`).
    * Verify the installation by running `migrate -version` in your terminal.

2.  **Create Initial Migration Files:**
    * Navigate to your project's root directory in the terminal.
    * Use the `migrate` CLI to create a new set of migration files in the `migrations/` directory. The command typically looks like this:
        ```bash
        migrate create -ext sql -dir migrations -seq init_schema
        ```
    * This command will generate two files in the `migrations/` directory, for example:
        * `000001_init_schema.up.sql` (for applying the migration)
        * `000001_init_schema.down.sql` (for rolling back the migration)
        *(The numeric prefix `000001` will be sequential based on existing migrations.)*

3.  **Define Schema in the UP Migration (`..._init_schema.up.sql`):**
    * Open the generated `*.up.sql` file.
    * Add the SQL statements to create the tables, indexes, and an `updated_at` helper function and triggers.

    ```sql
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
    ```

4.  **Define Schema in the DOWN Migration (`..._init_schema.down.sql`):**
    * Open the generated `*.down.sql` file.
    * Add the SQL statements to drop the tables and the helper function, in reverse order of creation.

    ```sql
    -- migrations/000001_init_schema.down.sql

    BEGIN;

    DROP TRIGGER IF EXISTS set_timestamp_teacher_report_statuses ON teacher_report_statuses;
    DROP TABLE IF EXISTS teacher_report_statuses;

    DROP TRIGGER IF EXISTS set_timestamp_teachers ON teachers;
    DROP TABLE IF EXISTS teachers;

    DROP TABLE IF EXISTS notification_cycles;

    DROP FUNCTION IF EXISTS trigger_set_timestamp();

    COMMIT;
    ```

5.  **Document Migration Commands (in your project's README or developer notes):**
    * Note down how to apply and roll back migrations. You will need the `DATABASE_URL` from your configuration (e.g., from `.env` file if using one for local dev).
    * Example commands (replace `your_database_url_here` with the actual URL, e.g., `postgres://your_db_user:your_db_password@localhost:5432/teacher_bot_dev?sslmode=disable`):
        * **Apply all UP migrations:**
          ```bash
          migrate -database "your_database_url_here" -path migrations up
          ```
        * **Roll back the last applied migration:**
          ```bash
          migrate -database "your_database_url_here" -path migrations down 1
          ```
        * **Roll back all migrations:**
          ```bash
          migrate -database "your_database_url_here" -path migrations down --all # Use with caution
          ```
        * **Force a specific version (can be risky, use for fixing dirty states):**
          ```bash
          migrate -database "your_database_url_here" -path migrations force <version_number>
          ```

---

**Acceptance Criteria:**

* The `golang-migrate/migrate` CLI tool is installed and accessible.
* Two SQL migration files (e.g., `000001_init_schema.up.sql` and `000001_init_schema.down.sql`) are created in the `migrations/` directory.
* The `*.up.sql` migration script successfully creates:
    * The `trigger_set_timestamp()` PostgreSQL function.
    * The `teachers` table with specified columns, types, constraints (PK, UNIQUE `telegram_id`, NOT NULL, defaults), indexes, and the `updated_at` trigger.
    * The `notification_cycles` table with specified columns, types, constraints (PK, NOT NULL, defaults), and indexes.
    * The `teacher_report_statuses` table with specified columns, types, constraints (PK, FKs to `teachers` and `notification_cycles` with `ON DELETE CASCADE`, UNIQUE constraint on `teacher_id, cycle_id, report_key`, NOT NULL, defaults), indexes, and the `updated_at` trigger.
* The `*.down.sql` migration script successfully drops all tables, their triggers, and the `trigger_set_timestamp()` function in the correct reverse order.
* Running the UP migration command against the PostgreSQL database (running in Docker via `docker-compose up -d db`) completes without errors.
* After applying the UP migration, connecting to the database (e.g., using `psql` or a GUI tool) shows that all tables, columns, indexes, and the trigger function exist as defined.
* Running the DOWN migration command successfully rolls back the schema changes, and the database reflects this (tables and function are dropped).

---

**Critical Tests (Manual Verification):**

1.  **Verify Migration File Creation:**
    * Check that the `migrations/` directory contains the `000001_init_schema.up.sql` and `000001_init_schema.down.sql` files (or with the correct sequence number).
2.  **Review SQL in UP File:**
    * Carefully read `000001_init_schema.up.sql`. Confirm all `CREATE TABLE`, `CREATE INDEX`, `CREATE FUNCTION`, `CREATE TRIGGER` statements are present, syntactically correct, and match the specified schema (column names, types like `BIGSERIAL`, `BIGINT`, `VARCHAR`, `TIMESTAMPTZ`, `BOOLEAN`, `DATE`, constraints like `PRIMARY KEY`, `FOREIGN KEY REFERENCES ... ON DELETE CASCADE`, `UNIQUE`, `NOT NULL`, `DEFAULT`).
3.  **Review SQL in DOWN File:**
    * Carefully read `000001_init_schema.down.sql`. Confirm all `DROP TRIGGER`, `DROP TABLE`, `DROP FUNCTION` statements are present, in the correct reverse order of creation, and syntactically correct.
4.  **Execute UP Migration:**
    * Ensure your PostgreSQL Docker container is running: `docker-compose up -d db`.
    * Load your `DATABASE_URL` into an environment variable or use it directly in the command. For example, if using an `.env` file: `export $(grep -v '^#' .env | xargs)` (on Linux/macOS) then `migrate -database "$DATABASE_URL" -path migrations up`.
    * Verify the command completes successfully.
5.  **Inspect Database Schema Post-UP:**
    * Connect to your PostgreSQL instance (e.g., `docker exec -it teacher_bot_db psql -U your_db_user -d teacher_bot_dev`).
    * Use `\dt` to list tables. Verify `teachers`, `notification_cycles`, `teacher_report_statuses`, and `schema_migrations` (created by the tool) are present.
    * Inspect individual table structures (e.g., `\d teachers`, `\d notification_cycles`, `\d teacher_report_statuses`). Check columns, types, nullability, defaults, indexes, and foreign key constraints.
    * Verify the `trigger_set_timestamp` function exists: `\df trigger_set_timestamp`.
6.  **Test `updated_at` Trigger (Example on `teachers` table):**
    * In `psql`:
        ```sql
        INSERT INTO teachers (telegram_id, first_name) VALUES (123, 'Test');
        SELECT id, created_at, updated_at FROM teachers WHERE telegram_id = 123; -- Note created_at and updated_at
        -- Wait a second
        UPDATE teachers SET first_name = 'Test Updated' WHERE telegram_id = 123;
        SELECT id, created_at, updated_at FROM teachers WHERE telegram_id = 123; -- Verify updated_at has changed and is later than created_at
        ```
7.  **Execute DOWN Migration:**
    * Run `migrate -database "$DATABASE_URL" -path migrations down 1` (or `migrate ... down --all` if it's the only one).
    * Verify the command completes successfully.
8.  **Inspect Database Schema Post-DOWN:**
    * Connect to PostgreSQL again.
    * Use `\dt`. The tables `teachers`, `notification_cycles`, `teacher_report_statuses` should be gone (though `schema_migrations` might remain, possibly empty or marking no migrations applied).
    * Verify the `trigger_set_timestamp` function is also gone using `\df trigger_set_timestamp`.
9.  **Re-apply UP Migration:**
    * Run `migrate -database "$DATABASE_URL" -path migrations up` again to leave the database schema in place for subsequent development tasks.