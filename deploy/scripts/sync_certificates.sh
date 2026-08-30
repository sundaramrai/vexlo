#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root" >&2
  exit 1
fi

cert_dir=/etc/vexlo/certs
install -d -o root -g vexlo -m 0750 "$cert_dir"

copy_pair() {
  local source_name="$1"
  local target_name="$2"
  install -o root -g vexlo -m 0640 \
    "/etc/letsencrypt/live/${source_name}/fullchain.pem" \
    "${cert_dir}/${target_name}-fullchain.pem"
  install -o root -g vexlo -m 0640 \
    "/etc/letsencrypt/live/${source_name}/privkey.pem" \
    "${cert_dir}/${target_name}-privkey.pem"
}

copy_pair vexlo-dashboard dashboard
copy_pair vexlo-wildcard wildcard

if systemctl is-active --quiet vexlo.service; then
  systemctl restart vexlo.service
fi
