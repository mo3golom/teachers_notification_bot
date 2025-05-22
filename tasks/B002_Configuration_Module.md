## Backend Task: B002 - Implement Configuration Management

**Objective:**
To develop a robust configuration module that loads, validates, and provides access to application settings. These settings include the Telegram API token, database connection string, Admin Telegram ID, Manager Telegram ID, and logging level. Configuration should be primarily sourced from environment variables, with support for a `.env` file for convenient local development.

**Background:**
Secure and flexible management of application settings is critical for any application. Sensitive information such as API tokens or database credentials must not be hardcoded into the source code[cite: 94]. This module will establish a centralized and secure way to manage and access configuration parameters throughout the "Teacher Notification Bot" application.

**Tech Stack:**
* Go version: 1.24
* Libraries: `os`, `strconv`, `github.com/joho/godotenv`

**Key Requirements from PRD:**
* Configuration for Telegram token, Admin ID, Manager ID, and DB connection parameters must be externalized (via config files or environment variables)[cite: 94].

---

**Steps to Completion:**

1.  **Define Configuration Struct:**
    * Navigate to the `internal/infra/config/` directory.
    * Create a new file named `config.go`.
    * In `config.go`, define a Go struct, for example `AppConfig`, to hold all configuration parameters:
        ```go
        package config

        import (
            "os"
            "strconv"
            "time" // Example for future use, like timeouts

            "[github.com/joho/godotenv](https://github.com/joho/godotenv)"
        )

        // AppConfig holds all configuration for the application
        type AppConfig struct {
            TelegramToken     string
            DatabaseURL       string
            AdminTelegramID   int64
            ManagerTelegramID int64
            LogLevel          string
            Environment       string        // e.g., "development", "production", "testing"
            // Example for future: ServerPort string
            // Example for future: DefaultTimeout time.Duration
        }

        // Placeholder for Load function, to be implemented next
        func Load() (*AppConfig, error) {
            // Implementation will follow in the next step
            return nil, nil
        }
        ```

2.  **Implement Configuration Loading Logic:**
    * In the same `internal/infra/config/config.go` file, implement the `Load()` function.
    * This function should:
        * Attempt to load a `.env` file using `godotenv.Load()`. It's okay if the file doesn't exist, especially in production.
        * Read each configuration value from an environment variable using `os.Getenv()`.
        * Populate the `AppConfig` struct.
        * Convert string values from environment variables to their correct types (e.g., `AdminTelegramID` to `int64` using `strconv.ParseInt`).
        * Implement basic validation: ensure essential fields (like `TelegramToken`, `DatabaseURL`, `AdminTelegramID`, `ManagerTelegramID`) are not empty. Return an error if critical configuration is missing.
        * Provide sensible defaults for non-critical fields if appropriate (e.g., `LogLevel` defaults to "info", `Environment` to "development").

        ```go
        package config

        import (
            "fmt"
            "os"
            "strconv"
            "strings" // For LogLevel normalization

            "[github.com/joho/godotenv](https://github.com/joho/godotenv)"
        )

        // AppConfig holds all configuration for the application
        type AppConfig struct {
            TelegramToken     string
            DatabaseURL       string
            AdminTelegramID   int64
            ManagerTelegramID int64
            LogLevel          string
            Environment       string
        }

        // Load reads configuration from environment variables and .env file (if present).
        func Load() (*AppConfig, error) {
            // Attempt to load .env file. Errors are ignored if the file doesn't exist.
            _ = godotenv.Load() // godotenv.Load will not override existing env variables

            cfg := &AppConfig{}
            var err error

            cfg.TelegramToken = os.Getenv("TELEGRAM_TOKEN")
            if cfg.TelegramToken == "" {
                return nil, fmt.Errorf("TELEGRAM_TOKEN is not set")
            }

            cfg.DatabaseURL = os.Getenv("DATABASE_URL")
            if cfg.DatabaseURL == "" {
                return nil, fmt.Errorf("DATABASE_URL is not set")
            }

            adminIDStr := os.Getenv("ADMIN_TELEGRAM_ID")
            if adminIDStr == "" {
                return nil, fmt.Errorf("ADMIN_TELEGRAM_ID is not set")
            }
            cfg.AdminTelegramID, err = strconv.ParseInt(adminIDStr, 10, 64)
            if err != nil {
                return nil, fmt.Errorf("invalid ADMIN_TELEGRAM_ID: %w", err)
            }

            managerIDStr := os.Getenv("MANAGER_TELEGRAM_ID")
            if managerIDStr == "" {
                return nil, fmt.Errorf("MANAGER_TELEGRAM_ID is not set")
            }
            cfg.ManagerTelegramID, err = strconv.ParseInt(managerIDStr, 10, 64)
            if err != nil {
                return nil, fmt.Errorf("invalid MANAGER_TELEGRAM_ID: %w", err)
            }

            cfg.LogLevel = strings.ToLower(os.Getenv("LOG_LEVEL"))
            if cfg.LogLevel == "" {
                cfg.LogLevel = "info" // Default log level
            }

            cfg.Environment = strings.ToLower(os.Getenv("ENVIRONMENT"))
            if cfg.Environment == "" {
                cfg.Environment = "development" // Default environment
            }

            return cfg, nil
        }
        ```

3.  **Create Example Configuration File (`.env.example`):**
    * In the project root directory, create a file named `.env.example`.
    * Add the following content, serving as a template for actual `.env` files:
        ```env
        # Telegram Bot API Token
        TELEGRAM_TOKEN="your_actual_telegram_bot_token_here"

        # PostgreSQL Connection URL
        # Format: postgres://user:password@host:port/dbname?sslmode=disable
        DATABASE_URL="postgres://your_db_user:your_db_password@localhost:5432/teacher_bot_dev?sslmode=disable"

        # Telegram User ID of the Bot Administrator
        ADMIN_TELEGRAM_ID="123456789"

        # Telegram User ID of the Manager/Supervisor to receive final reports
        MANAGER_TELEGRAM_ID="987654321"

        # Log Level (e.g., debug, info, warn, error)
        LOG_LEVEL="info"

        # Environment (e.g., development, staging, production)
        ENVIRONMENT="development"
        ```

4.  **Update `.gitignore`:**
    * Open the `.gitignore` file in your project root.
    * Add the line `.env` to ensure that local environment files containing sensitive credentials are not committed to version control.
        ```gitignore
        # Previous entries from B001 might be here
        
        # Environment variables
        .env
        ```

5.  **Integrate Configuration Loading into `main.go`:**
    * Modify `cmd/bot/main.go` to load the configuration at startup.
    * If configuration loading fails, the application should log the error and exit.
    * For now, print a confirmation message and a non-sensitive config value.
        ```go
        package main

        import (
            "fmt"
            "log" // Using standard log for now, will replace with structured logger later
            "os"  // For os.Exit

            "teacher_notification_bot/internal/infra/config" // Adjust import path if needed
        )

        func main() {
            fmt.Println("Teacher Notification Bot starting...")

            cfg, err := config.Load()
            if err != nil {
                log.Fatalf("FATAL: Could not load application configuration: %v", err)
                os.Exit(1) // Ensure application exits
            }

            log.Printf("INFO: Configuration loaded successfully. LogLevel: %s, Environment: %s", cfg.LogLevel, cfg.Environment)
            // Placeholder for further application startup logic using cfg
        }
        ```

---

**Acceptance Criteria:**

* An `AppConfig` struct is defined in `internal/infra/config/config.go` containing fields for `TelegramToken`, `DatabaseURL`, `AdminTelegramID`, `ManagerTelegramID`, `LogLevel`, and `Environment`.
* A `Load()` function in `internal/infra/config/config.go` correctly loads configuration from environment variables.
* The `Load()` function attempts to load variables from a `.env` file if it exists (values in `.env` should not override already set environment variables due to `godotenv.Load()` behavior).
* The `Load()` function returns an error if `TELEGRAM_TOKEN`, `DATABASE_URL`, `ADMIN_TELEGRAM_ID`, or `MANAGER_TELEGRAM_ID` are not set or if IDs are not valid integers.
* Default values are applied for `LogLevel` ("info") and `Environment` ("development") if they are not set.
* An `.env.example` file exists in the project root, clearly documenting all required environment variables and their formats.
* The `.gitignore` file includes an entry for `.env`.
* The `cmd/bot/main.go` application calls `config.Load()` upon startup.
* If configuration loading fails in `main.go`, a fatal error is logged, and the application exits.
* If configuration loading is successful, a confirmation message including some loaded values (e.g., LogLevel) is printed to the console.

---

**Critical Tests (Manual Verification & Future Unit Tests):**

1.  **Unit Tests for `config.Load()` (to be written in a subsequent task focusing on testing):**
    * Verify successful loading when all environment variables are correctly set.
    * Verify error return when `TELEGRAM_TOKEN` is missing.
    * Verify error return when `DATABASE_URL` is missing.
    * Verify error return when `ADMIN_TELEGRAM_ID` is missing or not a valid integer.
    * Verify error return when `MANAGER_TELEGRAM_ID` is missing or not a valid integer.
    * Verify default `LogLevel` and `Environment` are set if their respective ENV_VARS are missing.
    * Verify that values from a `.env` file are loaded if corresponding environment variables are not set.
    * Verify that existing environment variables take precedence over values in a `.env` file.
2.  **Manual Test with `.env` file:**
    * Create a valid `.env` file in the project root using `.env.example` as a template.
    * Run `go run ./cmd/bot/main.go`.
    * Confirm the "Configuration loaded successfully" message is printed, along with the correct LogLevel and Environment values from the `.env` file.
3.  **Manual Test with Environment Variables Only:**
    * Ensure no `.env` file is present (or rename it temporarily).
    * Set all required environment variables directly in your terminal (e.g., `export TELEGRAM_TOKEN="test_token_from_env"` etc.).
    * Run `go run ./cmd/bot/main.go`.
    * Confirm the application starts and logs indicate it's using the values set via environment variables.
4.  **Manual Test for Missing Critical Configuration:**
    * Unset a critical environment variable (e.g., `unset TELEGRAM_TOKEN` if using bash, or remove it from `.env` and ensure no env var is set).
    * Run `go run ./cmd/bot/main.go`.
    * Confirm the application logs a fatal error message and exits.
5.  **Verification of `.gitignore`:**
    * Open `.gitignore` and confirm that `.env` is listed.
6.  **Verification of `.env.example`:**
    * Open `.env.example` and confirm it exists and correctly lists all necessary configuration keys and example formats.