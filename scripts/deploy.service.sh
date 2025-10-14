#!/usr/bin/env bash
set -e

APP_NAME="burlo"
SERVICE_NAME="${APP_NAME}.service"

if [ -z "$1" ]; then
  echo "Usage: $0 <path-to-project-root>"
  exit 1
fi

ROOT_DIR=$(realpath "$1")
CONFIG_DIR="${ROOT_DIR}/var/config"
DATA_DIR="${ROOT_DIR}/var/cache"
LOG_DIR="${ROOT_DIR}/var/logs"
BIN_PATH="${ROOT_DIR}/${APP_NAME}"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}"

# --- Validate binary ---
if [ ! -f "$BIN_PATH" ]; then
  echo "❌ Binary not found at $BIN_PATH. Build first."
  exit 1
fi

# --- Check config directory ---
if [ ! -d "$CONFIG_DIR" ] || [ -z "$(ls -A "$CONFIG_DIR")" ]; then
  echo "⚠️ Config directory '$CONFIG_DIR' is missing or empty."
  echo "Please move your config files there before starting the service."
fi

# --- Create user if missing ---
if ! id -u "$APP_NAME" >/dev/null 2>&1; then
  echo "Creating system user $APP_NAME"
  useradd --system --no-create-home --shell /usr/sbin/nologin "$APP_NAME"
fi

# --- Create data and log directories ---
mkdir -p "$DATA_DIR" "$LOG_DIR"
chmod -R 755 "$DATA_DIR" "$LOG_DIR"

# --- Deploy binary ---
chmod 755 "$BIN_PATH"

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
ExecStart=${BIN_PATH}
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
