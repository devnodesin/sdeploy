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
  echo "Run go build -o sdeploy ./cmd/sdeploy"
  echo "Or use Docker to build:"
  echo "docker run --rm -v "$(pwd):/app" -w /app golang:latest sh -c "go build -buildvcs=false -o sdeploy ./cmd/sdeploy""
	exit 1
fi

echo "Stopping sdeploy service if running..."
sudo systemctl stop sdeploy

echo "Copying sdeploy binary to /usr/local/bin..."
sudo cp sdeploy /usr/local/bin/

echo "Copying config files: /etc/sdeploy.conf"
sudo cp samples/sdeploy.conf /etc/sdeploy.conf

echo "Copying systemd service files: /etc/systemd/system/sdeploy.service"
sudo cp samples/sdeploy.service /etc/systemd/system/sdeploy.service

echo "Reloading systemd, enabling sdeploy service..."
sudo systemctl daemon-reload
sudo systemctl enable sdeploy

echo "Starting sdeploy service..."
sudo systemctl start sdeploy

echo "Checking sdeploy service status..."
sudo systemctl status sdeploy

echo "Install complete."
