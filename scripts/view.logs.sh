#!/usr/bin/env bash
set -e

APP_NAME="burlo"

# Optional: allow passing a service name or log lines count
SERVICE="${1:-$APP_NAME.service}"
LINES="${2:-100}"

echo "📖 Showing last $LINES lines for $SERVICE..."
journalctl -u "$SERVICE" -n "$LINES"

echo
echo "📡 To follow logs in real-time, run:"
echo "  sudo journalctl -u $SERVICE -f"
