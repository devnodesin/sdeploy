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

### Copy binary:

```sh
# Stop service if already running
sudo systemctl stop sdeploy

sudo cp sdeploy /usr/local/bin/

# Create directory for deployments
sudo mkdir -pv /opt/sdeploy
```

### Create config:

```sh
# Quick start (minimal config)
sudo cp samples/sdeploy.conf /etc/sdeploy.conf
sudo cp samples/sdeploy.service /etc/systemd/system/sdeploy.service
```

### SSH Key Setup (for private repositories):

If you need to deploy from private git repositories, set up SSH keys:

```sh
# Create directory for SSH keys
sudo mkdir -p /etc/sdeploy/keys
sudo chmod 700 /etc/sdeploy/keys

# Generate a deploy key (ED25519 recommended)
sudo ssh-keygen -t ed25519 -C "sdeploy-deploy-key" -f /etc/sdeploy/keys/deploy-key -N ""

# Set proper permissions
sudo chmod 600 /etc/sdeploy/keys/deploy-key
sudo chmod 644 /etc/sdeploy/keys/deploy-key.pub

# Display public key to add to your repository
sudo cat /etc/sdeploy/keys/deploy-key.pub
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
    git_ssh_key_path: /etc/sdeploy/keys/deploy-key
    # ... other config
```

### systemctl Service:

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
