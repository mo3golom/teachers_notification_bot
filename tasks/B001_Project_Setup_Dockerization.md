## Backend Task: B001 - Basic Project Setup & Dockerization

**Objective:**
To initialize the Go module for the "Teacher Notification Bot", create the initial directory structure as per the architectural plan, and set up Docker for both the Go application and a PostgreSQL database. This task lays the foundational environment for all subsequent development work.

**Background:**
The project aims to create a Telegram bot using Go to automate reminders for teachers and notify management. [cite: 3] A robust project setup and containerization strategy are crucial for consistent development, testing, and deployment environments. [cite: 91, 95]

**Tech Stack:**
* Go version: 1.24 [cite: 85]
* Database: PostgreSQL [cite: 87]
* Containerization: Docker, Docker Compose [cite: 91]

**Key Requirements from PRD:**
* The application will be written in Golang. [cite: 85]
* PostgreSQL will be used for data persistence. [cite: 87]
* The deployment will utilize Docker containers for the application and database. [cite: 91]

---

**Steps to Completion:**

1.  **Initialize Go Module:**
    * Create a root directory for the project, named `teacher_notification_bot`.
    * Navigate into this directory.
    * Initialize the Go module by running the command: `go mod init teacher_notification_bot` (or your preferred module path, e.g., `github.com/<your-username>/teacher_notification_bot`).

2.  **Create Basic Project Directory Structure:**
    * Inside the `teacher_notification_bot` root directory, create the following subdirectories:
        * `cmd/`
            * `cmd/bot/` (This will house the `main.go` for the bot application)
        * `internal/`
            * `internal/app/` (For application services and use case logic)
            * `internal/domain/` (For domain entities, value objects, repository interfaces, and domain services)
            * `internal/infra/` (For infrastructure implementations)
                * `internal/infra/config/`
                * `internal/infra/database/` (For PostgreSQL connection and repository implementations)
                * `internal/infra/logger/`
                * `internal/infra/scheduler/`
                * `internal/infra/telegram/` (For Telegram bot client and handlers)
        * `migrations/` (To store SQL database migration files)

3.  **Create Placeholder `main.go`:**
    * Inside `cmd/bot/`, create a file named `main.go`.
    * Add the following basic Go code to it:
        ```go
        package main

        import "fmt"

        func main() {
            fmt.Println("Teacher Notification Bot starting...")
        }
        ```

4.  **Add Initial Core Dependencies:**
    * Open your terminal in the project root directory.
    * Run `go get` for the following libraries:
        * Telegram Bot library: `gopkg.in/telebot.v3` [cite: 86]
        * PostgreSQL driver: `github.com/lib/pq`
        * Cron job library: `github.com/robfig/cron/v3` [cite: 88]
        * Logging library: `github.com/sirupsen/logrus` [cite: 90]
        * Configuration helper (for .env files, useful for local dev): `github.com/joho/godotenv`

5.  **Create `Dockerfile` for the Go Application:**
    * In the project root directory, create a file named `Dockerfile`.
    * Add the following content, ensuring you are using Go 1.24:
        ```dockerfile
        # Stage 1: Build the Go application
        FROM golang:1.24-alpine AS builder

        WORKDIR /app

        # Copy go.mod and go.sum files
        COPY go.mod go.sum ./
        # Download dependencies
        RUN go mod download

        # Copy the source code
        COPY . .

        # Build the application
        # Adjust the output path if your main.go is elsewhere or named differently
        RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/teacher_bot_server ./cmd/bot/main.go

        # Stage 2: Create a minimal production image
        FROM alpine:latest

        WORKDIR /root/

        # Copy the pre-built binary from the builder stage
        COPY --from=builder /app/teacher_bot_server .

        # Command to run the executable
        CMD ["./teacher_bot_server"]
        ```

6.  **Create `docker-compose.yml` for Services:**
    * In the project root directory, create a file named `docker-compose.yml`.
    * Add the following content:
        ```yaml
        version: '3.8'

        services:
          app:
            build:
              context: .
              dockerfile: Dockerfile
            container_name: teacher_bot_app
            restart: unless-stopped
            # Environment variables can be passed here or through an .env file
            # env_file:
            #   - .env
            depends_on:
              - db
            # For development, you might want to mount volumes to reflect code changes:
            # volumes:
            #   - .:/app
            # If your app needs to expose a port (e.g., for health checks later):
            # ports:
            #   - "8080:8080"

          db:
            image: postgres:15-alpine # Using a specific version is good practice
            container_name: teacher_bot_db
            restart: unless-stopped
            environment:
              POSTGRES_USER: your_db_user       # Replace with your desired user
              POSTGRES_PASSWORD: your_db_password # Replace with your desired password
              POSTGRES_DB: teacher_bot_dev    # Replace with your desired DB name
            ports:
              - "5432:5432" # Maps host port 5432 to container port 5432
            volumes:
              - postgres_data:/var/lib/postgresql/data/

        volumes:
          postgres_data: # Persists database data across container restarts
        ```
    * **Note:** Remember to replace `your_db_user`, `your_db_password`, and `teacher_bot_dev` with your preferred credentials. You can create a `.env` file for these later if you uncomment `env_file`.

---

**Acceptance Criteria:**

* The Go module (`go.mod`, `go.sum`) is successfully initialized in the project root.
* The specified directory structure (`cmd/bot`, `internal/*`, `migrations/`) is correctly created.
* The `cmd/bot/main.go` file exists and, when run locally (`go run ./cmd/bot/main.go`), prints "Teacher Notification Bot starting..." to the console.
* The core dependencies (`gopkg.in/telebot.v3`, `github.com/lib/pq`, `github.com/robfig/cron/v3`, `github.com/sirupsen/logrus`, `github.com/joho/godotenv`) are listed in the `go.mod` file.
* The `Dockerfile` is present and can successfully build a Go application image using `docker build -t teacher-bot-app-test .`.
* The `docker-compose.yml` file is present.
* Running `docker-compose up -d --build` successfully starts both the `app` and `db` containers without errors.
* The `app` container's logs (viewable with `docker-compose logs app` or `docker logs teacher_bot_app`) show the "Teacher Notification Bot starting..." message.
* The `db` container (`teacher_bot_db`) is running, and PostgreSQL is confirmed to be operational (e.g., one can connect to it using a DB tool or `docker exec`).

---

**Critical Tests (Manual Verification):**

1.  **Verify Go Module:**
    * Check for `go.mod` and `go.sum` files in the project root.
    * Open `go.mod` and confirm the module path is `teacher_notification_bot` (or as chosen) and Go version (e.g., `go 1.24`).
2.  **Verify Directory Structure:**
    * Manually inspect the project directory to ensure all specified folders (`cmd/bot`, `internal/*/*`, `migrations`) exist.
3.  **Test `main.go` Locally:**
    * Navigate to the project root in your terminal.
    * Run `go run ./cmd/bot/main.go`.
    * Confirm "Teacher Notification Bot starting..." is printed to the console.
4.  **Check Dependencies:**
    * Open `go.mod` and verify that `gopkg.in/telebot.v3`, `github.com/lib/pq`, `github.com/robfig/cron/v3`, `github.com/sirupsen/logrus`, and `github.com/joho/godotenv` are listed under the `require` section.
5.  **Test Docker Image Build:**
    * In the project root, run `docker build -t teacher-bot-app-test .`.
    * The build should complete without errors.
6.  **Test Docker Compose Setup:**
    * In the project root, run `docker-compose up -d --build`.
    * After it completes, run `docker-compose ps`. Both `teacher_bot_app` and `teacher_bot_db` should show as `Up` or `running`.
7.  **Check Application Container Logs:**
    * Run `docker-compose logs app` (or `docker logs teacher_bot_app`).
    * Verify that the "Teacher Notification Bot starting..." message is present in the logs.
8.  **Verify PostgreSQL Connectivity (Basic Check):**
    * Run `docker exec -it teacher_bot_db psql -U your_db_user -d teacher_bot_dev` (using the credentials you set in `docker-compose.yml`).
    * If successful, you'll enter the `psql` prompt. Type `\conninfo` to see connection details or `\q` to exit. This confirms the PostgreSQL server is running and accessible.