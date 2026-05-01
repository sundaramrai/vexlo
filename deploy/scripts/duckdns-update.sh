#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${DUCKDNS_DOMAIN:-}" || -z "${DUCKDNS_TOKEN:-}" ]]; then
  echo "DUCKDNS_DOMAIN and DUCKDNS_TOKEN are required"
  exit 1
fi

response="$(curl -fsSL "https://www.duckdns.org/update?domains=${DUCKDNS_DOMAIN}&token=${DUCKDNS_TOKEN}&ip=")"
if [[ "$response" != "OK" ]]; then
  echo "DuckDNS update failed: ${response}"
  exit 1
fi

echo "DuckDNS updated for ${DUCKDNS_DOMAIN}"
