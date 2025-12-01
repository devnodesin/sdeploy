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

1. Copy binary:

```sh
sudo cp sdeploy /usr/local/bin/

# Create directory for deployments
sudo mkdir -pv /opt/sdeploy

```

2. Create config:

```sh
# Quick start (minimal config)
sudo cp samples/sdeploy.conf /etc/sdeploy.conf

# Or use the full reference config
sudo cp samples/sdeploy-full.conf /etc/sdeploy.conf

sudo cp samples/sdeploy.service /etc/systemd/system/sdeploy.service
```

3. systemctl Service:

```sh
# Register and Enable Service
sudo systemctl daemon-reload
sudo systemctl enable sdeploy
```
```sh
# Start/stop service
sudo systemctl start sdeploy
sudo systemctl stop sdeploy
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
