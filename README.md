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

**With Docker:**

```sh
# Build
docker build -t sdeploy:latest .

# Run (single config file)
docker run -d -p 8080:8080 \
  -v /path/to/sdeploy.conf:/etc/sdeploy.conf:ro \
  -v /var/log/sdeploy:/var/log/sdeploy \
  sdeploy:latest

# Run (multiple config files merged together, later override earlier)
docker run -d -p 8080:8080 \
  -v /path/to/sdeploy.conf:/etc/sdeploy.conf:ro \
  -v /path/to/secrets.conf:/etc/secrets.conf:ro \
  -v /var/log/sdeploy:/var/log/sdeploy \
  sdeploy:latest -c /etc/sdeploy.conf,/etc/secrets.conf
```

See [INSTALL.md](INSTALL.md) for detailed build, test, and deployment instructions.

## Usage

```
sdeploy [options]

Options:
  -c <list>  Comma-separated list of config files (YAML format).
             Later files override earlier ones. When empty, falls back to
             /etc/sdeploy.conf then ./sdeploy.conf.
  -d         Run as daemon (background service)
  -h         Show help
```

Config file search order:
1. Path(s) from `-c` flag (comma-separated list, merged in order)
2. `/etc/sdeploy.conf`
3. `./sdeploy.conf`

## Configuration

SDeploy uses YAML format for configuration:

- **[samples/sdeploy.conf](samples/sdeploy.conf)** — Minimal quick-start example
- **[samples/sdeploy-full.conf](samples/sdeploy-full.conf)** — Full reference with all fields documented

### Multiple Config Files (Merging)

You can split configuration across multiple YAML files and have SDeploy merge
them at startup using the `-c` flag with a comma-separated list:

```sh
sdeploy -c /etc/sdeploy.conf,/etc/secrets.conf
```

Files are merged **in order, left to right** — a value in a later file overrides
the same key in an earlier file. The merge is deep:

- Top-level maps and nested maps are merged key-by-key.
- `projects` lists are matched by `webhook_path`:
  - A project in a later file with the **same** `webhook_path` **patches** the existing project (only the fields you specify are overridden; everything else is inherited).
  - A project with a **new** `webhook_path` is **appended** as a new project.
  - This means you can keep projects in completely separate files and they will all be combined.
- Scalars and other lists (e.g. `email_recipients`, `env_variables`) are replaced wholesale.

A common pattern is to keep your main `sdeploy.conf` in version control while
putting secrets (SMTP password, webhook secrets) in a separate `secrets.conf`
that is mounted into the container or kept out of source control:

```yaml
# secrets.conf — mounted at runtime, not committed
email_config:
  smtp_pass: super-secret-password
projects:
  - webhook_secret: project-specific-secret
```

Missing or unreadable files in the list are skipped silently, so optional
overlays can be omitted without breaking startup. If **no** valid file is found
across the whole list, SDeploy exits with an error.

| Key             | Description                                     |
|-----------------|-------------------------------------------------|
| `listen_port`   | HTTP port (default: 8080)                       |
| `log_path`      | Base directory for log files (default: `/var/log/sdeploy`) |
| `email_config`  | SMTP settings for notifications                 |
| `projects`      | Array of project configurations                 |

**Logging:**
- Service logs: `{log_path}/main.log` (daemon mode) or stderr (console mode)
- Build logs: `{log_path}/{project}-{date}-{time}-{success|fail}.log` (always to file)

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
