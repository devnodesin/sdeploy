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

# Copy sdeploy binary with confirmation
if [ -f "/usr/local/bin/sdeploy" ]; then
	read -p "/usr/local/bin/sdeploy already exists. Overwrite? (y/N): " -n 1 -r
	echo
	if [[ ! $REPLY =~ ^[Yy]$ ]]; then
		echo "Skipping binary installation."
	else
		echo "Copying sdeploy binary to /usr/local/bin..."
		cp sdeploy /usr/local/bin/
	fi
else
	echo "Copying sdeploy binary to /usr/local/bin..."
	cp sdeploy /usr/local/bin/
fi

# Copy config file with confirmation
if [ -f "/etc/sdeploy.conf" ]; then
	read -p "/etc/sdeploy.conf already exists. Overwrite? (y/N): " -n 1 -r
	echo
	if [[ ! $REPLY =~ ^[Yy]$ ]]; then
		echo "Skipping config file installation."
	else
		echo "Copying config files: /etc/sdeploy.conf"
		cp samples/sdeploy.conf /etc/sdeploy.conf
	fi
else
	echo "Copying config files: /etc/sdeploy.conf"
	cp samples/sdeploy.conf /etc/sdeploy.conf
fi

# Copy systemd service file with confirmation
if [ -f "/etc/systemd/system/sdeploy.service" ]; then
	read -p "/etc/systemd/system/sdeploy.service already exists. Overwrite? (y/N): " -n 1 -r
	echo
	if [[ ! $REPLY =~ ^[Yy]$ ]]; then
		echo "Skipping systemd service installation."
	else
		echo "Copying systemd service files: /etc/systemd/system/sdeploy.service"
		cp samples/sdeploy.service /etc/systemd/system/sdeploy.service
	fi
else
	echo "Copying systemd service files: /etc/systemd/system/sdeploy.service"
	cp samples/sdeploy.service /etc/systemd/system/sdeploy.service
fi

echo "Reloading systemd, enabling sdeploy service..."
systemctl daemon-reload
systemctl enable sdeploy

echo "Starting sdeploy service..."
systemctl start sdeploy

echo "Checking sdeploy service status..."
systemctl status sdeploy

echo "Install complete."
