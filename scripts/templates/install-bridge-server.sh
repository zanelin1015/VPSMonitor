#!/usr/bin/env sh
set -eu

SERVICE_NAME="${SERVICE_NAME:-bridge-server}"
INSTALL_DIR="${INSTALL_DIR:-/opt/bridge-server}"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
CONFIG_SRC="$SCRIPT_DIR/config/server.json"
CONFIG_DST="$INSTALL_DIR/config/server.json"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

if [ "$(id -u)" -ne 0 ]; then
  if command -v sudo >/dev/null 2>&1; then
    exec sudo SERVICE_NAME="$SERVICE_NAME" INSTALL_DIR="$INSTALL_DIR" "$0" "$@"
  fi
  echo "Please run as root, or install sudo first." >&2
  exit 1
fi

if [ ! -x "$SCRIPT_DIR/bridge-server" ]; then
  echo "bridge-server binary not found in $SCRIPT_DIR" >&2
  exit 1
fi

random_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
  else
    dd if=/dev/urandom bs=24 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n'
  fi
}

ADMIN_PASSWORD=""
REGISTRATION_TOKEN=""

mkdir -p "$INSTALL_DIR/config" "$INSTALL_DIR/data"
install -m 0755 "$SCRIPT_DIR/bridge-server" "$INSTALL_DIR/bridge-server"
install -m 0755 "$SCRIPT_DIR/run.sh" "$INSTALL_DIR/run.sh"
[ -f "$SCRIPT_DIR/README.md" ] && install -m 0644 "$SCRIPT_DIR/README.md" "$INSTALL_DIR/README.md"

if [ ! -f "$CONFIG_DST" ]; then
  install -m 0600 "$CONFIG_SRC" "$CONFIG_DST"
  ADMIN_PASSWORD="$(random_token)"
  REGISTRATION_TOKEN="$(random_token)"
  sed -i "s/replace-with-admin-password/${ADMIN_PASSWORD}/g" "$CONFIG_DST"
  sed -i "s/replace-with-registration-token/${REGISTRATION_TOKEN}/g" "$CONFIG_DST"
else
  echo "Existing config kept: $CONFIG_DST"
fi

cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Bridge Core Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/bridge-server -config $CONFIG_DST
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null
systemctl restart "$SERVICE_NAME"

echo "Bridge Core Server installed."
echo "Service: $SERVICE_NAME"
echo "Install dir: $INSTALL_DIR"
echo "Config: $CONFIG_DST"
echo "Status: systemctl status $SERVICE_NAME"
echo "Logs: journalctl -u $SERVICE_NAME -f"
if [ -n "$ADMIN_PASSWORD" ]; then
  echo ""
  echo "Initial admin username: admin"
  echo "Initial admin password: $ADMIN_PASSWORD"
  echo "Registration token: $REGISTRATION_TOKEN"
  echo "Please save these values now. You can later edit $CONFIG_DST and run:"
  echo "  systemctl restart $SERVICE_NAME"
fi
