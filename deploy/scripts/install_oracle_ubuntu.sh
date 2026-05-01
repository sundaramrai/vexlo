#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root"
  exit 1
fi

if [[ $# -lt 3 ]]; then
  echo "Usage: $0 <base-domain> <host-url> <acme-email> [binary-url]"
  echo "Example: $0 vexlo.example.com https://vexlo.example.com you@example.com https://github.com/yourorg/vexlo/releases/latest/download/vexlo-server-linux-amd64"
  exit 1
fi

BASE_DOMAIN="$1"
HOST_URL="$2"
ACME_EMAIL="$3"
BINARY_URL="${4:-}"

apt-get update
apt-get install -y curl ca-certificates ufw cron

id -u vexlo >/dev/null 2>&1 || useradd --system --home /opt/vexlo --shell /usr/sbin/nologin vexlo

mkdir -p /opt/vexlo /etc/vexlo /var/lib/vexlo/acme-cache
chown -R vexlo:vexlo /opt/vexlo /etc/vexlo /var/lib/vexlo

if [[ -n "$BINARY_URL" ]]; then
  curl -fsSL "$BINARY_URL" -o /opt/vexlo/vexlo-server
  chmod +x /opt/vexlo/vexlo-server
else
  echo "No binary URL supplied. Place vexlo-server at /opt/vexlo/vexlo-server before starting the service."
fi

cat >/etc/vexlo/vexlo.env <<EOF
VEXLO_BASE_DOMAIN=${BASE_DOMAIN}
VEXLO_HOST_URL=${HOST_URL}
VEXLO_ACME_EMAIL=${ACME_EMAIL}
DUCKDNS_DOMAIN=
DUCKDNS_TOKEN=
EOF

cp "$(dirname "$0")/../systemd/vexlo.service" /etc/systemd/system/vexlo.service

ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 2222/tcp
ufw allow 9000/tcp
ufw --force enable

systemctl daemon-reload
systemctl enable vexlo.service
systemctl enable cron

echo "Install complete."
echo "Next steps:"
echo "1. Put the server binary at /opt/vexlo/vexlo-server if you did not pass a binary URL."
echo "2. Edit /etc/vexlo/vexlo.env and fill DuckDNS values if needed."
echo "3. Start the service with: systemctl start vexlo"
echo "4. Inspect logs with: journalctl -u vexlo -f"
