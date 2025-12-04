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
./sdeploy -c sdeploy.conf

# Run (daemon mode)
./sdeploy -c sdeploy.conf -d
```

See [INSTALL.md](INSTALL.md) for detailed build, test, and deployment instructions.

## Usage

```
sdeploy [options]

Options:
  -c <path>  Path to config file (YAML format)
  -d         Run as daemon (background service)
  -h         Show help
```

Config file search order:
1. Path from `-c` flag
2. `/etc/sdeploy.conf`
3. `./sdeploy.conf`

## Configuration

SDeploy uses YAML format for configuration:

- **[samples/sdeploy.conf](samples/sdeploy.conf)** — Minimal quick-start example
- **[samples/sdeploy-full.conf](samples/sdeploy-full.conf)** — Full reference with all fields documented

| Key             | Description                              |
|-----------------|------------------------------------------|
| `listen_port`   | HTTP port (default: 8080)                |
| `email_config`  | SMTP settings for notifications          |
| `projects`      | Array of project configurations          |

**Note:** Logs are always written to `/var/log/sdeploy.log`. The `log_filepath` configuration option is deprecated and ignored.

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
| `git_update`      | Run `git pull` before deployment                  |
| `git_ssh_key_path`| Path to SSH private key for git operations        |
| `email_recipients`| Notification email addresses                      |

## Documentation

- [INSTALL.md](INSTALL.md) — Build, test, and deployment instructions
- [SPEC.md](SPEC.md) — Full specification and architecture details

## License

See [LICENSE](LICENSE).
