#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${1:-/var/backups/vexlo}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
TARGET_DIR="${BACKUP_DIR}/${TIMESTAMP}"

install -d -m 0750 "$TARGET_DIR"

install -m 0600 /var/lib/vexlo/vexlo.db "${TARGET_DIR}/vexlo.db"

for suffix in -wal -shm; do
  if [[ -f "/var/lib/vexlo/vexlo.db${suffix}" ]]; then
    install -m 0600 "/var/lib/vexlo/vexlo.db${suffix}" "${TARGET_DIR}/vexlo.db${suffix}"
  fi
done

echo "Backup written to ${TARGET_DIR}"
