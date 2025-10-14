#!/usr/bin/env bash
set -e

APP_NAME="burlo"
SERVICE_NAME="${APP_NAME}.service"

if [ -z "$1" ]; then
  echo "Usage: $0 <path-to-project-root>"
  exit 1
fi

ROOT_DIR=$(realpath "$1")
CONFIG_SRC="${ROOT_DIR}/config"

BIN_PATH="${ROOT_DIR}/${APP_NAME}"
CONFIG_DIR="${ROOT_DIR}/var/config"
DATA_DIR="${ROOT_DIR}/var/cache"
LOG_DIR="${ROOT_DIR}/var/logs"

SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}"

# --- Validate binary ---
if [ ! -f "$BIN_PATH" ]; then
  echo "❌ Binary not found at $BIN_PATH. Build first."
  exit 1
fi

# Create user if missing
if ! id -u "$APP_NAME" >/dev/null 2>&1; then
  echo "Creating system user $APP_NAME"
  useradd --system --no-create-home --shell /usr/sbin/nologin "$APP_NAME"
fi

# Create directories
mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
rsync -a --ignore-existing "${CONFIG_SRC}/" "$CONFIG_DIR/"

# --- Create systemd service ---
tee "$SERVICE_FILE" > /dev/null <<EOF
[Unit]
Description=Burlo Service
After=network.target

[Service]
Type=simple
User=${APP_NAME}
Group=${APP_NAME}
WorkingDirectory=${ROOT_DIR}
ExecStart=${ROOT_DIR}/${APP_NAME}
Restart=on-failure
RestartSec=5
Environment=PROJECT_ROOT=${ROOT_DIR}
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# --- Reload systemd and restart service ---
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
systemctl restart "$SERVICE_NAME"

echo "✅ Installation complete!"
echo "Logs: journalctl -u $SERVICE_NAME -f"
