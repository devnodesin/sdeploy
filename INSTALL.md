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
  sh -c "go mod tidy && go build -buildvcs=false -o sdeploy ./cmd/sdeploy"

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

## Verify

```sh
# Check status
sudo systemctl status sdeploy

# Test webhook
curl -X POST "http://localhost:8080/hooks/sdeploy-test?secret=your_webhook_secret_here" \
  -d '{"ref":"refs/heads/main"}'
```
