# SDeploy

A lightweight, Go-based daemon that automates deployments via webhooks.

## Features

- **Webhook Listener** — HTTP endpoint for GitHub, GitLab, or CI/CD triggers
- **HMAC & Secret Auth** — Secure requests via signature or query parameter
- **Branch Filtering** — Only deploy matching branches
- **Single Execution** — One deployment at a time per project (lock-based)
- **Pre-flight Checks** — Automatic directory setup with correct ownership and permissions
- **Git Integration** — Optional `git pull` before running deploy commands
- **Email Notifications** — Send deployment summaries on completion
- **Daemon Mode** — Run as a background service with logging
- **Hot Reload** — Configuration changes are automatically applied without restart

## Quick Start

```sh
# Build
go build -o sdeploy ./cmd/sdeploy

# Run (console mode)
./sdeploy -c config.json

# Run (daemon mode)
./sdeploy -c config.json -d
```

See [INSTALL.md](INSTALL.md) for detailed build, test, and deployment instructions.

## Usage

```
sdeploy [options]

Options:
  -c <path>  Path to config file
  -d         Run as daemon (background service)
  -h         Show help
```

Config file search order:
1. Path from `-c` flag
2. `/etc/sdeploy/config.json`
3. `./config.json`

## Configuration

See [samples/config.json](samples/config.json) for a complete example.

| Key             | Description                              |
|-----------------|------------------------------------------|
| `listen_port`   | HTTP port (default: 8080)                |
| `log_filepath`  | Log file path (daemon mode)              |
| `email_config`  | SMTP settings for notifications          |
| `projects`      | Array of project configurations          |

### Project Config

| Key               | Description                                       |
|-------------------|---------------------------------------------------|
| `name`            | Project identifier                                |
| `webhook_path`    | Unique URI path (e.g., `/hooks/api`)              |
| `webhook_secret`  | Secret for authentication                         |
| `git_branch`      | Branch required to trigger deployment             |
| `execute_command` | Shell command to run                              |
| `local_path`      | Local directory for git operations                |
| `execute_path`    | Working directory for command (defaults to local_path) |
| `run_as_user`     | User to run commands as (default: www-data)       |
| `run_as_group`    | Group to run commands as (default: www-data)      |
| `git_update`      | Run `git pull` before deployment                  |
| `email_recipients`| Notification email addresses                      |

## Pre-flight Directory Checks

SDeploy automatically handles directory setup before each deployment:

- **Auto-Creation**: Missing `local_path` and `execute_path` directories are created automatically
- **Ownership Management**: When running as root, directories are set to `run_as_user:run_as_group`
- **Path Defaults**: If `execute_path` is not set, it defaults to `local_path`
- **Logging**: All directory operations are logged for transparency

This eliminates manual setup steps and ensures deployments work correctly from the first run.

## Triggering Deployments

**Via webhook (HMAC signature):**
```sh
curl -X POST http://localhost:8080/hooks/myproject \
  -H "X-Hub-Signature: sha1=..." \
  -d '{"ref":"refs/heads/main"}'
```

**Via secret query parameter (internal/cron):**
```sh
curl -X POST "http://localhost:8080/hooks/myproject?secret=your_secret" \
  -d '{"ref":"refs/heads/main"}'
```

## Hot Reload

SDeploy automatically detects changes to the configuration file and applies them without requiring a restart.

### What Gets Hot-Reloaded

- ✅ Project configurations (add/remove/modify)
- ✅ Email/SMTP settings
- ✅ Log file path
- ⚠️ Listen port (requires restart)

### How It Works

1. SDeploy watches the config file for changes
2. When a change is detected, the new config is validated
3. If valid, the new config is applied immediately
4. If invalid, the current config is preserved and an error is logged

### During Active Deployments

If a deployment is in progress when the config file changes:
- The reload is deferred until all active deployments complete
- This ensures deployments use consistent configuration throughout

### Example Log Output

```
[INFO] Hot reload enabled for config file: /etc/sdeploy/config.json
[INFO] Reloading configuration...
[INFO] Configuration reloaded successfully
```

### Troubleshooting Hot Reload

| Issue | Solution |
|-------|----------|
| Config not reloading | Check file permissions and ensure SDeploy has read access |
| Invalid config rejected | Check logs for validation errors, fix config and save again |
| Port change not taking effect | Restart SDeploy - listen_port cannot be hot-reloaded |

## Documentation

- [INSTALL.md](INSTALL.md) — Build, test, and deployment instructions
- [SPEC.md](SPEC.md) — Full specification and architecture details

## License

See [LICENSE](LICENSE).
