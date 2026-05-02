#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root"
  exit 1
fi

if [[ $# -lt 3 ]]; then
  echo "Usage: $0 <base-domain> <host-url> <acme-email> [binary-url]"
  echo "Example: $0 vexlo.example.com https://vexlo.example.com you@example.com https://github.com/yourorg/vexlo/releases/latest/download/vexlo-server-linux-amd64.tar.gz"
  exit 1
fi

BASE_DOMAIN="$1"
HOST_URL="$2"
ACME_EMAIL="$3"
BINARY_URL="${4:-}"

apt-get update
apt-get install -y curl ca-certificates ufw cron tar

id -u vexlo >/dev/null 2>&1 || useradd --system --home /opt/vexlo --shell /usr/sbin/nologin vexlo

mkdir -p /opt/vexlo /etc/vexlo /var/lib/vexlo/acme-cache
chown -R vexlo:vexlo /opt/vexlo /etc/vexlo /var/lib/vexlo
chmod 0750 /etc/vexlo

if [[ -n "$BINARY_URL" ]]; then
  tmp="$(mktemp)"
  curl -fsSL "$BINARY_URL" -o "$tmp"
  if [[ "$BINARY_URL" == *.tar.gz ]]; then
    extracted_name="$(tar -tzf "$tmp" | head -n1)"
    tar -xzf "$tmp" -C /opt/vexlo
    mv "/opt/vexlo/${extracted_name}" /opt/vexlo/vexlo-server
    rm -f "$tmp"
  else
    mv "$tmp" /opt/vexlo/vexlo-server
  fi
  chmod +x /opt/vexlo/vexlo-server
else
  echo "No binary URL supplied. Place vexlo-server at /opt/vexlo/vexlo-server before starting the service."
fi

cat >/etc/vexlo/vexlo.env <<EOF
VEXLO_BASE_DOMAIN=${BASE_DOMAIN}
VEXLO_HOST_URL=${HOST_URL}
VEXLO_ACME_EMAIL=${ACME_EMAIL}
VEXLO_REGISTRATION_TOKEN=replace-with-long-random-token
VEXLO_ADMIN_USER=admin
VEXLO_ADMIN_PASS=replace-with-strong-password
VEXLO_CAPTURE_BODY_LIMIT=262144
VEXLO_MAX_REQUEST_BODY_BYTES=2097152
VEXLO_MAX_API_BODY_BYTES=524288
VEXLO_RETENTION_PERIOD=168h
VEXLO_READ_TIMEOUT=15s
VEXLO_WRITE_TIMEOUT=60s
VEXLO_IDLE_TIMEOUT=60s
EOF

cp "$(dirname "$0")/../systemd/vexlo.service" /etc/systemd/system/vexlo.service
install -m 0755 "$(dirname "$0")/backup_vexlo.sh" /opt/vexlo/backup_vexlo.sh

ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 9000/tcp
ufw --force enable

systemctl daemon-reload
systemctl enable vexlo.service
systemctl enable cron

echo "Install complete."
echo "Next steps:"
echo "1. Put the server binary at /opt/vexlo/vexlo-server if you did not pass a binary URL."
echo "2. Edit /etc/vexlo/vexlo.env and replace the placeholder registration/admin secrets."
echo "3. Start the service with: systemctl start vexlo"
echo "4. Inspect logs with: journalctl -u vexlo -f"
