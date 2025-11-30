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
./sdeploy -c config.json

## Daemon mode:

./sdeploy -c config.json -d
```

## Install as systemd Service

1. Copy binary:

```sh
sudo cp sdeploy /usr/local/bin/
```

2. Create config:

```sh
sudo mkdir -p /etc/sdeploy
sudo cp samples/config.json /etc/sdeploy/config.json
# Edit config as needed

sudo cp samples/sdeploy.service /etc/systemd/system/sdeploy.service
```

3. Enable and start:

```sh
sudo systemctl daemon-reload
sudo systemctl enable sdeploy
sudo systemctl start sdeploy
```

## Automatic Directory Setup

SDeploy automatically handles directory setup during deployment:

- **Pre-flight Checks**: Before each deployment, SDeploy verifies that `local_path` and `execute_path` directories exist.
- **Auto-Creation**: Missing directories are automatically created with correct permissions (0755).
- **Ownership Management**: When running as root (e.g., via systemd), SDeploy sets directory ownership to the configured `run_as_user` and `run_as_group` (default: `www-data:www-data`).
- **Path Defaults**: If `execute_path` is not specified, it defaults to `local_path`.

This eliminates the need for manual directory setup before deployment.

### Example Scenario

If your config specifies:
```json
{
  "local_path": "/var/repo/myproject",
  "execute_path": "/var/www/myproject"
}
```

SDeploy will:
1. Create `/var/repo/myproject` if it doesn't exist
2. Create `/var/www/myproject` if it doesn't exist
3. Set ownership to `www-data:www-data` (if running as root)
4. Log all actions for transparency

## Verify

```sh
# Check status
sudo systemctl status sdeploy

# Test webhook
curl -X POST "http://localhost:8080/hooks/sdeploy-test?secret=your_webhook_secret_here" \
  -d '{"ref":"refs/heads/main"}'
```
