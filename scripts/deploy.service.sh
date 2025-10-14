#!/usr/bin/env bash
set -e

APP_NAME="burlo"
SERVICE_NAME="${APP_NAME}.service"

if [ -z "$1" ]; then
  echo "Usage: $0 <path-to-source-root>"
  exit 1
fi

SRC_DIR=$(realpath "$1")
BIN_PATH="${SRC_DIR}/build/${APP_NAME}"
CONFIG_SRC="${SRC_DIR}/config"

INSTALL_BIN_DIR="/usr/local/bin"
CONFIG_DIR="/etc/${APP_NAME}"
DATA_DIR="/var/lib/${APP_NAME}"
LOG_DIR="/var/log/${APP_NAME}"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}"

# Validate binary
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
chown -R "${APP_NAME}:${APP_NAME}" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
chmod -R 755 "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"

# Install binary
cp -f "$BIN_PATH" "$INSTALL_BIN_DIR/"
chown "${APP_NAME}:${APP_NAME}" "${INSTALL_BIN_DIR}/${APP_NAME}"
chmod 755 "${INSTALL_BIN_DIR}/${APP_NAME}"

# Write systemd service
tee "$SERVICE_FILE" > /dev/null <<EOF
[Unit]
Description=Burlo Service
After=network.target

[Service]
Type=simple
User=${APP_NAME}
Group=${APP_NAME}
ExecStart=${INSTALL_BIN_DIR}/${APP_NAME} --config ${CONFIG_DIR}
WorkingDirectory=${DATA_DIR}
Restart=on-failure
RestartSec=5
Environment=CONFIG_DIR=${CONFIG_DIR}
Environment=DATA_DIR=${DATA_DIR}
Environment=LOG_DIR=${LOG_DIR}
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Reload + enable + start
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
systemctl restart "$SERVICE_NAME"

echo "✅ Installation complete! Logs: journalctl -u $SERVICE_NAME -f"

