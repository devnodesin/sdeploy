#!/bin/bash

# SDeploy Install Script
# This script installs sdeploy as a systemd service

# Check for root
if [ "$EUID" -ne 0 ]; then
	echo "Please run as root"
	exit 1
fi

# Check for sdeploy binary in current directory
if [ ! -f "sdeploy" ]; then
	echo "Error: sdeploy binary not found in current directory."
	echo "Please build sdeploy first (see INSTALL.md)."
  echo ""
  echo "Run go build -o sdeploy ./cmd/sdeploy"
  echo "Or use Docker to build:"
  echo 'docker run --rm -v "$(pwd):/app" -w /app golang:latest sh -c "go build -buildvcs=false -o sdeploy ./cmd/sdeploy"'
	exit 1
fi

echo "Stopping sdeploy service if running..."
systemctl stop sdeploy

mkdir -p /etc/sdeploy-keys
chmod 700 /etc/sdeploy-keys

echo "Copying sdeploy binary to /usr/local/bin..."
cp sdeploy /usr/local/bin/

echo "Copying config files: /etc/sdeploy.conf"
cp samples/sdeploy.conf /etc/sdeploy.conf

echo "Copying systemd service files: /etc/systemd/system/sdeploy.service"
cp samples/sdeploy.service /etc/systemd/system/sdeploy.service

echo "Reloading systemd, enabling sdeploy service..."
systemctl daemon-reload
systemctl enable sdeploy

echo "Starting sdeploy service..."
systemctl start sdeploy

echo "Checking sdeploy service status..."
systemctl status sdeploy

echo "Install complete."
