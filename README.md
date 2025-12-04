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

## Pre-flight Directory Checks

SDeploy automatically handles directory setup before each deployment:

- **Auto-Creation**: Missing `local_path` and `execute_path` directories are created automatically with 0755 permissions
- **Path Defaults**: If `execute_path` is not set, it defaults to `local_path`
- **Logging**: All directory operations are logged for transparency

This eliminates manual setup steps and ensures deployments work correctly from the first run.

## Using Private Repositories

SDeploy supports deploying from private git repositories using SSH key authentication.

### Setup

1. **Generate a deploy key** (or use an existing SSH key):
   ```sh
   ssh-keygen -t ed25519 -C "sdeploy-deploy-key" -f /etc/sdeploy/keys/deploy-key -N ""
   ```

2. **Set strict permissions** on the private key:
   ```sh
   chmod 600 /etc/sdeploy/keys/deploy-key
   ```

3. **Add the public key to your repository**:
   - GitHub: Settings → Deploy keys → Add deploy key
   - GitLab: Settings → Repository → Deploy Keys → Add key
   - Copy the contents of `/etc/sdeploy/keys/deploy-key.pub`

4. **Configure SDeploy** to use the key:
   ```yaml
   projects:
     - name: Private Backend
       webhook_path: /hooks/backend
       webhook_secret: your_secret_here
       git_repo: git@github.com:myorg/private-repo.git
       git_ssh_key_path: /etc/sdeploy/keys/deploy-key
       git_branch: main
       git_update: true
       local_path: /var/repo/backend
       execute_command: npm install && npm run build
   ```

### Public Repositories

For public repositories, you can omit `git_ssh_key_path`:

```yaml
projects:
  - name: Public Frontend
    webhook_path: /hooks/frontend
    webhook_secret: your_secret_here
    git_repo: https://github.com/myorg/public-repo.git
    git_branch: main
    git_update: true
    local_path: /var/repo/frontend
    execute_command: npm install && npm run build
```

### Troubleshooting SSH Keys

| Issue | Solution |
|-------|----------|
| `SSH key validation failed: file does not exist` | Verify the path in `git_ssh_key_path` is correct and the file exists |
| `SSH key validation failed: not readable` | Check file permissions: `chmod 600 /path/to/key` |
| `Git clone failed` with authentication error | Verify the public key is added to your repository's deploy keys |
| `Host key verification failed` | The SSH option `StrictHostKeyChecking=accept-new` is used automatically to accept new host keys |

### Security Best Practices

- Use read-only deploy keys when possible
- Store SSH keys in a secure location (e.g., `/etc/sdeploy/keys/`)
- Set file permissions to `600` (owner read/write only)
- Never commit SSH private keys to version control
- Rotate deploy keys regularly

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
[INFO] Hot reload enabled for config file: /etc/sdeploy.conf
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
