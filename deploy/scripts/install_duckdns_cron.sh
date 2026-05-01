#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root"
  exit 1
fi

install -d -m 0755 /opt/vexlo
install -m 0755 "$(dirname "$0")/duckdns-update.sh" /opt/vexlo/duckdns-update.sh

cron_line='*/5 * * * * . /etc/vexlo/vexlo.env && /opt/vexlo/duckdns-update.sh >> /var/log/vexlo-duckdns.log 2>&1'

tmp="$(mktemp)"
crontab -l 2>/dev/null | grep -v 'duckdns-update.sh' >"$tmp" || true
echo "$cron_line" >>"$tmp"
crontab "$tmp"
rm -f "$tmp"

echo "DuckDNS cron installed."
