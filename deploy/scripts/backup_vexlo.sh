#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${1:-/var/backups/vexlo}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
TARGET_DIR="${BACKUP_DIR}/${TIMESTAMP}"

if [[ "$TARGET_DIR" == *"'"* || "$TARGET_DIR" == *$'\n'* ]]; then
  echo 'Backup path may not contain quotes or newlines.' >&2
  exit 1
fi
command -v sqlite3 >/dev/null || { echo 'sqlite3 is required for an online SQLite backup.' >&2; exit 1; }
install -d -m 0750 "$TARGET_DIR"

# SQLite's online backup API takes a consistent snapshot even while WAL writes
# continue. Copying the database and WAL files separately does not.
sqlite3 /var/lib/vexlo/vexlo.db ".backup '${TARGET_DIR}/vexlo.db'"
chmod 0600 "${TARGET_DIR}/vexlo.db"
integrity="$(sqlite3 "${TARGET_DIR}/vexlo.db" 'PRAGMA integrity_check;')"
if [[ "$integrity" != ok ]]; then
  echo "Backup integrity check failed: ${integrity}" >&2
  exit 1
fi

echo "Backup written to ${TARGET_DIR}"
