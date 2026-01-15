
# 📦 SDeploy: Simple Webhook Deployment Daemon

This document outlines the features and requirements for SDeploy, a lightweight, Go-based daemon service designed to automate application deployment via incoming webhooks.

## 🧪 Test-Driven Development (TDD) Policy

All Go source code in SDeploy MUST be developed using test-driven development (TDD):

- Every new feature, bugfix, or refactor must be accompanied by one or more unit tests before implementation.
- All major logic modules require corresponding `*_test.go` files.
- Tests must cover:
  - Webhook validation and routing
  - Deployment locking and execution
  - Config loading and error handling
  - Email notification logic
  - Logging output and error capture
  - Hot reload functionality
  - Pre-flight directory checks
- CI/CD or local workflows must run `go test ./cmd/sdeploy/...` and all tests must pass before merging or release.
- No code is considered complete or production-ready without passing tests.

**Enforcement:**
- PRs and code reviews must reject changes that lack appropriate tests.
- All contributors must follow TDD for every code change.

## 🔧 Centralized Default Values

All hardcoded fallback defaults must be centralized in [`cmd/sdeploy/config.go`](cmd/sdeploy/config.go). This ensures maintainability and consistency across the codebase.

### Current Centralized Defaults

> **Source:** See `Defaults` struct in [`cmd/sdeploy/config.go`](cmd/sdeploy/config.go)

| Field       | Default Value          | Description                          |
|-------------|------------------------|--------------------------------------|
| `Port`      | `8080`                 | HTTP listener port                   |
| `LogPath`   | `/var/log/sdeploy`     | Base directory for log files         |
| `GitBranch` | `"main"`               | Default git branch                   |

Config file search order is defined in `ConfigSearchPaths`:
1. `/etc/sdeploy.conf`
2. `./sdeploy.conf`

### Developer Workflow for Default Values

1. **Always define defaults in the `Defaults` struct**: Never use hardcoded string literals for default values directly in business logic.
2. **Reference via `Defaults.X`**: All code and tests should access defaults via `Defaults.Port`, `Defaults.GitBranch`, etc.
3. **Naming convention**: Use clear, concise field names (e.g., `Port`, `LogPath`, `GitBranch`).
4. **Update tests**: Tests should use `Defaults.X` instead of hardcoded values to ensure they stay in sync.

## 🚀 Overview and Goal

SDeploy provides a dedicated service that listens for external webhook notifications (e.g., GitHub, GitLab, CI/CD) and triggers a local deployment script.

### 🔑 Core Principle: Single Execution

Only one deployment process runs at a time for any given project. New webhook requests arriving during an active deployment are safely skipped until the current one finishes.

## 🏃 Installation and Usage

> **Full installation instructions:** See [`INSTALL.md`](INSTALL.md)

### Quick Start

```sh
# Build
go build -o sdeploy ./cmd/sdeploy

# Console mode (foreground)
./sdeploy -c /path/to/sdeploy.conf

# Daemon mode (background service)
./sdeploy -c /path/to/sdeploy.conf -d
```

### Execution Modes

| Mode         | Command           | Description                                                                 |
|--------------|-------------------|-----------------------------------------------------------------------------|
| Console      | `./sdeploy`       | Foreground, blocking. Output to stdout/stderr. Used for testing/setup.      |
| Daemon       | `./sdeploy -d`    | Background service. Output to log file. For use with system services.       |

### Running as a Service

> **systemd service files:** See [`samples/sdeploy.service`](samples/sdeploy.service)

## 📁 Project Folder Structure

```
sdeploy/
├── cmd/
│   └── sdeploy/
│       ├── main.go              # Entry point and CLI flags
│       ├── config.go            # Configuration loading and validation
│       ├── webhook.go           # HTTP webhook handler
│       ├── deploy.go            # Deployment execution logic
│       ├── preflight.go         # Pre-flight directory checks
│       ├── email.go             # Email notification logic
│       ├── logging.go           # Logging infrastructure
│       ├── hotreload.go         # Hot reload functionality
│       ├── signal.go            # Signal handling
│       ├── deploy_platform.go   # Platform-specific deployment (Unix)
│       ├── logging_platform.go  # Platform-specific logging (Unix)
│       └── *_test.go            # Test files for each module
├── samples/
│   ├── sdeploy.conf             # Minimal configuration example
│   ├── sdeploy-full.conf        # Full configuration reference
│   └── sdeploy.service          # systemd service file
├── SPEC.md                      # This specification document
├── INSTALL.md                   # Installation instructions
├── README.md                    # Quick start guide
└── go.mod                       # Go module definition
```

## ⚙️ Configuration

SDeploy uses YAML format for configuration.

### Configuration Files

| File | Description |
|------|-------------|
| [`samples/sdeploy.conf`](samples/sdeploy.conf) | Minimal quick-start example |
| [`samples/sdeploy-full.conf`](samples/sdeploy-full.conf) | Full reference with all fields and comments |

### Config File Search Order

1. Path from `-c` flag (explicit)
2. `/etc/sdeploy.conf`
3. `./sdeploy.conf`

### Global Configuration

| Key            | Type   | Default              | Description                                    |
|----------------|--------|----------------------|------------------------------------------------|
| `listen_port`  | int    | `8080`               | HTTP port for webhook listener                 |
| `log_path`     | string | `/var/log/sdeploy`   | Base directory for log files (daemon mode)     |
| `email_config` | object | —                    | SMTP configuration (see below)                 |
| `projects`     | array  | —                    | List of project configurations                 |

**Logging Details:**
- **Service logs**: Written to `{log_path}/main.log` (or stderr in console mode)
- **Build logs**: Written to `{log_path}/{project_name}-{yyyy-mm-dd}-{HHMM}-{success|fail}.log`
- All logs are timestamped and include severity level (INFO, WARN, ERROR)
- Build logs are created per deployment and include only that build's output
- In console mode, service logs go to stderr, but build logs still go to files

### Email Configuration (`email_config`)

| Key            | Type   | Required | Description                    |
|----------------|--------|----------|--------------------------------|
| `smtp_host`    | string | Yes      | SMTP server address            |
| `smtp_port`    | int    | Yes      | SMTP server port (587 for TLS) |
| `smtp_user`    | string | Yes      | SMTP authentication username   |
| `smtp_pass`    | string | Yes      | SMTP password or API key       |
| `email_sender` | string | Yes      | Sender email address           |

**Behavior:**
- If `email_config` is absent or any required field is missing, email notifications are **globally disabled**.
- Per-project: If `email_recipients` is empty, email notifications are disabled for that project only.

### Project Configuration

| Key               | Type     | Required | Default      | Description                                    |
|-------------------|----------|----------|--------------|------------------------------------------------|
| `name`            | string   | No       | —            | Human-readable project identifier              |
| `webhook_path`    | string   | Yes      | —            | Unique URI path (e.g., `/hooks/api`)           |
| `webhook_secret`  | string   | Yes      | —            | Secret key for webhook authentication          |
| `git_repo`        | string   | No       | —            | Git repository URL (SSH/HTTPS)                 |
| `local_path`      | string   | No       | —            | Local directory for git operations             |
| `execute_path`    | string   | No       | `local_path` | Working directory for command execution        |
| `git_branch`      | string   | No       | `"main"`     | Branch required to trigger deployment          |
| `execute_command` | string   | Yes      | —            | Shell command to execute                       |
| `git_update`      | bool     | No       | `false`      | Run `git pull` before deployment               |
| `git_ssh_key_path`| string   | No       | —            | Path to SSH private key for git operations     |
| `timeout_seconds` | int      | No       | `0`          | Command timeout (0 = no timeout)               |
| `email_recipients`| []string | No       | —            | Notification email addresses                   |

### Git Behavior

- If `git_repo` is **not set**: No git operations are performed. `local_path` is treated as a local directory.
- If `git_repo` is **set** and repo not cloned: Clone the repository.
- If `git_repo` is **set** and repo exists: Skip cloning.
- If `git_update` is `true`: Run `git pull` before deployment.

### Git SSH Key Authentication

SDeploy supports per-project SSH key authentication for private git repositories through the `git_ssh_key_path` configuration option.

**Configuration:**

- Set `git_ssh_key_path` to the absolute path of an SSH private key file.
- If `git_ssh_key_path` is set, all git operations (clone, pull) for that project will use the specified key.
- If `git_ssh_key_path` is not set, git operations use the default system SSH agent (suitable for public repos or user-managed keys).

**Requirements:**

- The SSH key file must exist and be readable by the SDeploy process.
- SSH key file should have strict permissions (`chmod 600`) for security.
- The SSH key must be authorized to access the git repository (e.g., added as a deploy key on GitHub).

**Error Handling:**

- If `git_ssh_key_path` is set but the file doesn't exist, deployment fails with a clear error message.
- If the file exists but is not readable, deployment fails with a permission error.
- Git operation failures (authentication, network) are logged with detailed error messages.

**Security:**

- Key file contents are never logged or exposed.
- Users should set strict file permissions on SSH keys (`chmod 600`).
- Deploy keys should be scoped to read-only access when possible.

## 🛠️ Key Features

| Feature                     | Description                                                              |
|-----------------------------|--------------------------------------------------------------------------|
| Webhook Listener            | Configurable port (default: 8080) for HTTP POST requests                 |
| Flexible Routing            | Routes requests by URI path to the correct project                       |
| HMAC Authentication         | Validates `X-Hub-Signature` header or fallback to `?secret=` query param |
| Branch Verification         | Ensures webhook payload branch matches configured branch                 |
| Asynchronous Deployment     | Valid requests trigger deployment in background, respond `202 Accepted`  |
| Pre-flight Directory Checks | Automatically creates directories with 0755 permissions                  |
| Git Operations              | Clone and pull support with configurable branch                          |
| Environment Variables       | Injects `SDEPLOY_PROJECT_NAME`, `SDEPLOY_TRIGGER_SOURCE`, etc.           |
| Comprehensive Logging       | Logs to stdout/stderr (console) or file (daemon mode)                    |
| Email Notifications         | Sends deployment summary emails when configured                          |
| Hot Reload                  | Configuration changes auto-detected and applied without restart          |

## 🔍 Pre-flight Directory Checks

SDeploy performs automated pre-flight checks before each deployment.

| Aspect              | Behavior                                                    |
|---------------------|-------------------------------------------------------------|
| Directory Existence | Checks if `local_path` and `execute_path` directories exist |
| Auto-Creation       | Missing directories are created with 0755 permissions       |
| Path Defaults       | `execute_path` defaults to `local_path` if not set          |
| Logging             | All directory creation actions are logged                   |

### Error Handling

| Error Type        | Handling                               |
|-------------------|----------------------------------------|
| Path is a file    | Deployment fails with error message    |
| Permission denied | Deployment fails with error message    |

## 🔄 Hot Reload

SDeploy supports hot reloading of the configuration file without daemon restart.

### What Can Be Hot-Reloaded

- **Projects:** Add, remove, or modify project configurations
- **Email Configuration:** Update SMTP settings
- **Log File Path:** Change log file location

### What Requires Restart

- **Listen Port:** Changing `listen_port` requires daemon restart
- **Active Deployments:** Continue with previous configuration

### Hot Reload Behavior

| Aspect          | Behavior                                                      |
|-----------------|---------------------------------------------------------------|
| Detection       | File system watcher monitors the config file                  |
| Validation      | New configuration validated before applying                   |
| Thread Safety   | Configuration reload is thread-safe using mutex               |
| Build Deferral  | If deployment in progress, reload deferred until completion   |

## 🛡️ Operational Principles

| Principle           | Detail                                                       |
|---------------------|--------------------------------------------------------------|
| Technology          | Implemented in Go for performance and resource efficiency    |
| Security            | Each project uses its own `webhook_secret` for authentication|
| Robustness          | Command errors are caught, logged, and do not crash daemon   |
| Concurrency Control | Single-instance execution enforced per project using locks   |

## 📐 Execution Flow

1. **Daemon Startup:** Log all global settings and project configurations.
2. **Request Entry:** Webhook POST received.
3. **Validation (Security):** Check HMAC signature (`X-Hub-Signature`). If missing, check `?secret=` query parameter.
4. **Validation (Logic):** Verify git branch matches configured branch.
5. **Lock Check:** If deployment lock held, log "Skipped" and return `202`. Otherwise, acquire lock.
6. **Asynchronous Trigger:** Start deployment in background, return `202 Accepted`.
7. **Log Project Config:** Print project configuration for this build.
8. **Pre-flight Checks:** Verify/create `local_path` and `execute_path` directories.
9. **Git Operations:**
   - If `git_repo` not set: Skip git operations.
   - If repo not cloned: Clone repository.
   - If `git_update` is true: Run `git pull`.
10. **Build Decision Logic:** Determine if build should proceed (see Build Trigger Logic below).
11. **Execution:** Run `execute_command` in `execute_path` (with timeout, env vars).
12. **Cleanup:** Log result, send email notification (if configured), release lock.

## 🎯 Build Trigger Logic

SDeploy uses intelligent build triggering based on the webhook source and git changes:

### No Changes Detection

When `git_update` is enabled and `git pull` detects no new commits, SDeploy determines whether to skip the build based on the trigger source:

| Trigger Source | Behavior When No Changes | Rationale |
|----------------|-------------------------|-----------|
| `WEBHOOK (Github)` | **Skip build** | GitHub push webhooks indicate explicit code pushes; no changes means nothing to deploy |
| `WEBHOOK (unknown)` | **Skip build** | Unknown webhook sources are treated conservatively |
| `WEBHOOK (<other>)` | **Always build** | Non-GitHub webhooks (Jenkins, GitLab, CI/CD) may have external reasons to rebuild |
| `INTERNAL` | **Always build** | Internal triggers (cron, manual) should always execute regardless of git state |

### Logging

When a build is skipped due to no changes:
```
[INFO] No changes detected (commit: d3af528a)
[INFO] Build ignored: no changes in the configured branch (trigger: WEBHOOK (Github))
```

When a build proceeds despite no changes:
```
[INFO] No changes detected, but proceeding with build (trigger: INTERNAL)
```

### Use Cases

**Example 1: GitHub Push Webhook**
- User pushes to GitHub → webhook fires → if no new commits pulled, skip build
- Prevents redundant builds from multiple webhook deliveries

**Example 2: Scheduled Cron Job**
- Cron triggers deployment → even if no new commits, run build
- Useful for periodic rebuilds, cache warming, or environment refreshes

**Example 3: CI/CD Pipeline**
- Jenkins/GitLab CI triggers webhook → always run build
- CI/CD systems may rebuild for reasons beyond git changes (dependency updates, cache refresh)

## 🌐 Integration with Reverse Proxies

Recommended to run SDeploy behind a reverse proxy for TLS/SSL and rate limiting.

### Nginx Example

```nginx
server {
    listen 443 ssl;
    server_name yourdomain.com;

    location /hooks/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        if ($request_method !~ ^(POST)$) {
            return 405;
        }
    }
}
```

### Caddy Example

```caddyfile
yourdomain.com {
    route /hooks/* {
        reverse_proxy 127.0.0.1:8080 {
            flush_interval -1
        }
    }
}
```

## 🕐 Integration with Cron (Scheduled Deployments)

Trigger deployments on a schedule using the secret query parameter:

```sh
# Cron job example: deploy at 3 AM daily
0 3 * * * curl -X POST "http://localhost:8080/hooks/frontend?secret=your_secret" -d '{"ref":"refs/heads/main"}'
```

SDeploy recognizes the missing HMAC signature, validates the secret query parameter, classifies as INTERNAL trigger, and proceeds with deployment.

