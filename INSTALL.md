# SDeploy Installation

## Requirements

- Go 1.24+ (or Docker)

## Build & Test

### Standard Build

```sh
go mod tidy
go build -o sdeploy ./cmd/sdeploy
go test ./cmd/sdeploy/... -v
```

### Build with Docker

```sh
docker run --rm -v "$(pwd):/app" -w /app golang:latest \
  sh -c "go mod tidy"

docker run --rm -v "$(pwd):/app" -w /app golang:latest \
  sh -c "go build -buildvcs=false -o sdeploy ./cmd/sdeploy"

## Run Test
docker run --rm -v "$(pwd):/app" -w /app golang:latest \
  sh -c "go test ./cmd/sdeploy/... -v"
```

## Run

```sh
## Console mode:
./sdeploy -c sdeploy.conf

## Daemon mode:

./sdeploy -c sdeploy.conf -d
```

## Install as systemd Service

### Copy binary

```sh
# Stop service if already running
sudo systemctl stop sdeploy

sudo cp sdeploy /usr/local/bin/

# Create directory for deployments
sudo mkdir -pv /opt/sdeploy
```

### Create config

```sh
# Quick start (minimal config)
sudo cp samples/sdeploy.conf /etc/sdeploy.conf
sudo cp samples/sdeploy.service /etc/systemd/system/sdeploy.service
```

### SSH Key Setup (for private repositories)

If you need to deploy from private git repositories, set up SSH keys:

```sh
# Create directory for SSH keys
sudo mkdir -p /etc/sdeploy-keys
sudo chmod 700 /etc/sdeploy-keys

# Generate a deploy key (ED25519 recommended)
sudo ssh-keygen -t ed25519 -C "sdeploy-deploy-key" -f /etc/sdeploy-keys/deploy-key -N ""

# Set proper permissions
sudo chmod 600 /etc/sdeploy-keys/deploy-key
sudo chmod 644 /etc/sdeploy-keys/deploy-key.pub

# Display public key to add to your repository
sudo cat /etc/sdeploy-keys/deploy-key.pub
```

Then add the public key to your repository:

- **GitHub**: Settings → Deploy keys → Add deploy key
- **GitLab**: Settings → Repository → Deploy Keys → Add key
- **Bitbucket**: Repository settings → Access keys → Add key

Update your config to use the key:

```yaml
projects:
  - name: Private Project
    git_repo: git@github.com:myorg/private-repo.git
    git_ssh_key_path: /etc/sdeploy-keys/deploy-key
    # ... other config
```

### systemctl Service

```sh
# Register and Enable Service
sudo systemctl daemon-reload
sudo systemctl enable sdeploy
```

```sh
## Start the service
sudo systemctl start sdeploy
```

```sh
# Check status
sudo systemctl status sdeploy
```

## Verify

```sh
# Test webhook
curl -X POST "http://localhost:8080/hooks/sdeploy-test?secret=your_webhook_secret_here" \
  -d '{"ref":"refs/heads/main"}'
```

## Triggering Deployments

**Via webhook (HMAC signature):**

```bash
## To create HMAC signatruce
echo -n '{"ref":"refs/heads/main"}' | openssl dgst -sha256 -hmac "your_webhook_secret_here"
# Output: (stdin)= 4ff441efda243ce9ea45c937bb4021c2d46cfd8f...

curl -X POST http://localhost:8080/hooks/myproject \
  -H "X-Hub-Signature-256: sha256=4ff441efda243ce9ea45c937bb4021c2d46cfd8f" \
  -d '{"ref":"refs/heads/main"}'
```

**Via secret query parameter (internal/cron):**

```sh
curl -X POST "http://localhost:8080/hooks/myproject?secret=your_secret" \
  -d '{"ref":"refs/heads/main"}'
```

**Refrence :** https://docs.github.com/en/webhooks/webhook-events-and-payloads#push

## Pre-flight Directory Checks

SDeploy automatically handles directory setup before each deployment:

- **Auto-Creation**: Missing `local_path` and `execute_path` directories are created automatically with 0755 permissions
- **Path Defaults**: If `execute_path` is not set, it defaults to `local_path`
- **Logging**: All directory operations are logged for transparency

This eliminates manual setup steps and ensures deployments work correctly from the first run.

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

```text
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

### Security Best Practices

- Use read-only deploy keys when possible
- Store SSH keys in a secure location (e.g., `/etc/sdeploy-keys/`)
- Set file permissions to `600` (owner read/write only)
- Never commit SSH private keys to version control
- Rotate deploy keys regularly
