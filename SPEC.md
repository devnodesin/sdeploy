# 📦 SDeploy: Simple Webhook Deployment Daemon

This document outlines the features and requirements for SDeploy, a lightweight, Go-based daemon service designed to automate application deployment via incoming webhooks.

## 🚀 Overview and Goal

SDeploy provides a dedicated service that listens for external webhook notifications (e.g., GitHub, GitLab, CI/CD) and triggers a local deployment script.

### 🔑 Core Principle: Single Execution

Only one deployment process runs at a time for any given project. New webhook requests arriving during an active deployment are safely skipped until the current one finishes.

## 🏃 Installation and Usage

The compiled binary is named `sdeploy`.

### Execution Modes

| Mode         | Command           | Description                                                                 |
|--------------|-------------------|-----------------------------------------------------------------------------|
| Console      | `./sdeploy`       | Foreground, blocking. Output to stdout/stderr. Used for testing/setup.      |
| Daemon       | `./sdeploy -d`    | Background service. Output to console/logger. For use with system services. |

Custom config file path: `./sdeploy -c /home/user/my_project/custom_config.json -d`

### Running SDeploy as a Service (Production)

#### 1. Using systemd (Linux)

Create `/etc/systemd/system/sdeploy.service`:

```ini
[Unit]
Description=SDeploy Webhook Daemon
After=network.target

[Service]
Type=simple
User=sdeploy_user  # Dedicated, non-root user
WorkingDirectory=/usr/local/bin/
ExecStart=/usr/local/bin/sdeploy -d
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Commands:
```sh
sudo useradd -r sdeploy_user
sudo systemctl daemon-reload
sudo systemctl enable sdeploy
sudo systemctl start sdeploy
sudo systemctl status sdeploy
```

#### 2. Using nohup

```sh
nohup /path/to/sdeploy -d > sdeploy.log 2>&1 &
```

## ⚙️ Configuration (`config.json`)

SDeploy searches for its config file in order:
1. Path from `-c` flag
2. `/etc/sdeploy/config.json`
3. `./config.json`

### Sample Configuration

```json
{
  "listen_port": 8080,
  "log_filepath": "/var/log/sdeploy/daemon.log",
  "email_config": {
    "smtp_host": "smtp.sendgrid.net",
    "smtp_port": 587,
    "smtp_user": "apikey",
    "smtp_pass": "SG.xxxxxxxxxxxx",
    "email_sender": "sdeploy@yourdomain.com"
  },
  "projects": [
    {
      "name": "User-Facing Frontend",
      "webhook_path": "/hooks/frontend",
      "webhook_secret": "secret_token_for_frontend_repo",
      "git_repo": "git@github.com:myorg/frontend-app.git",
      "git_path": "/var/repo/frontend-repo",
      "execute_path": "/var/www/site",
      "git_branch": "main",
      "execute_command": "sh /var/www/site/deploy.sh",
      "git_update": true,
      "email_recipients": ["frontend-team@domain.com", "on-call@domain.com"]
    },
    {
      "name": "Backend REST API",
      "webhook_path": "/hooks/api-service",
      "webhook_secret": "api_prod_key_777",
      "git_repo": "https://gitlab.com/api/backend-service.git",
      "git_path": "/opt/repo/api-repo",
      "execute_path": "/opt/services/api",
      "git_branch": "staging",
      "execute_command": "/usr/bin/supervisorctl restart api-service",
      "git_update": false,
      "email_recipients": ["api-team@domain.com"]
    }
  ]
}
```

### Global Email Configuration

| Key          | Description                                 |
|--------------|---------------------------------------------|
| smtp_host    | SMTP server address                         |
| smtp_port    | SMTP server port (e.g., 587 for TLS)        |
| smtp_user    | Username for SMTP authentication            |
| smtp_pass    | Password or API key for SMTP                |
| email_sender | Sender email address                        |

### Project Configuration

| Key               | Description                                               |
|-------------------|----------------------------------------------------------|
| name              | Human-readable project identifier                         |
| webhook_path      | Unique URI path (e.g., /hooks/api)                       |
| webhook_secret    | Secret key for webhook authentication                     |
| git_repo          | Git repository URL (SSH/HTTPS)                            |
| git_path          | Local directory for git operations                        |
| execute_path      | Directory for deployment script execution                 |
| git_branch        | Branch required to trigger deployment                     |
| execute_command   | Shell command to execute                                  |
| git_update        | If true, run `git pull` before deployment                 |
| timeout_seconds   | (Optional) Hard timeout for command execution             |
| email_recipients  | (Optional) Array of notification email addresses          |

## 🛠️ Key Features

- **Webhook Listener:** Configurable port (default: 8080) for HTTP POST requests.
- **Flexible Routing:** Routes requests by URI path to the correct project.
- **Authentication (HMAC & Secret Fallback):** Prioritize HMAC signature (`X-Hub-Signature`). If missing, fallback to secret in URL query.
- **Branch Verification:** Ensures webhook payload branch matches configured branch.
- **Asynchronous Deployment:** Valid requests trigger deployment in background, respond `202 Accepted`.
- **Git Update Control:** If `git_update` is true, run `git pull` before deployment.
- **Robust Execution:** Runs shell command in specified directory.
- **Environment Variable Injection:** Injects project details (e.g., `SDEPLOY_PROJECT_NAME`, `SDEPLOY_TRIGGER_SOURCE`) into deployment environment.
- **Comprehensive Logging:** Logs start/end time, output, status, to stdout/stderr or log file.
- **Email Notification:** On completion, sends summary email if configured.

## 🛡️ Operational Principles

| Principle           | Detail                                                                 |
|---------------------|------------------------------------------------------------------------|
| Technology          | MUST be implemented in Go for performance/resource efficiency           |
| Security            | Each project uses its own webhook_secret for authentication             |
| Robustness          | Command errors are caught, logged, and do not crash the daemon          |
| Concurrency Control | Single-instance execution enforced per project using locks              |

## 📐 Execution Flow

1. **Request Entry:** Webhook POST received.
2. **Validation (Security):** Check HMAC signature. If valid, trigger is WEBHOOK. If absent, check secret query parameter for INTERNAL trigger.
3. **Validation (Logic):** Verify git branch (for WEBHOOK).
4. **Lock Check:** If deployment lock held, log "Skipped" and return 202. If not, acquire lock and proceed.
5. **Asynchronous Trigger:** Start deployment in background, return 202.
6. **Pre-Execution (Git Update):** If `git_update`, run `git pull`.
7. **Execution:** Run `execute_command` in `execute_path` (with timeout, env vars).
8. **Cleanup & Notify:** Log result, send email notification, release lock.

## 🌐 Integration with Reverse Proxies

Recommended to run SDeploy behind a reverse proxy for TLS/SSL and rate limiting.

### Nginx Example

```nginx
server {
    listen 443 ssl;
    server_name yourdomain.com;

    # ssl_certificate /etc/ssl/certs/yourdomain.crt;
    # ssl_certificate_key /etc/ssl/private/yourdomain.key;

    location /hooks/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_request_buffering off;
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
        @notPost {
            method not POST
        }
        respond @notPost 405
    }
}
```

## 🌐 Integration with Cron (Scheduled Deployments)

Scheduled deployments are triggered by external job schedulers (e.g., cron) sending an internal trigger request using the secret query parameter.

### Cron Job Example

```cron
0 3 * * * curl -X POST "http://localhost:8080/hooks/frontend?secret=secret_token_for_frontend_repo" -d "{}" -H "Content-Type: application/json" >/dev/null 2>&1
```

SDeploy recognizes the missing HMAC, validates the secret, classifies as INTERNAL, and proceeds with deployment.

