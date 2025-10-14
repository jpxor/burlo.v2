#!/usr/bin/env bash
set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <target-port>"
  exit 1
fi

TARGET_PORT="$1"

# --- Delete any existing redirect from port 80 ---
while iptables -t nat -C PREROUTING -p tcp --dport 80 -j REDIRECT >/dev/null 2>&1; do
  echo "Deleting existing port 80 redirect..."
  iptables -t nat -D PREROUTING -p tcp --dport 80 -j REDIRECT
done

# --- Add new redirect ---
echo "Adding redirect: port 80 -> $TARGET_PORT"
iptables -t nat -A PREROUTING -p tcp --dport 80 -j REDIRECT --to-port "$TARGET_PORT"

# --- Persist rules (Debian/Ubuntu) ---
if command -v netfilter-persistent >/dev/null 2>&1; then
  netfilter-persistent save
  echo "iptables rules saved"
else
  echo "⚠️ netfilter-persistent not found; rules will not survive reboot"
fi

echo "✅ Port redirection updated successfully!"
