#!/usr/bin/env bash
#
# Pre-deploy snapshot of the production ShellyAdmin SQLite database.
#
# Why a host-side script: the database lives on the Docker host bind mount
# (<data-dir>/shellyctl.db). A container-exec tool cannot create the snapshot
# file inside the read-only-rootfs container, so the copy runs on the host
# over SSH instead.
#
# The database runs in WAL mode, so a plain `cp shellyctl.db` is NOT a snapshot:
# everything since the last checkpoint lives in shellyctl.db-wal and is left
# behind (measured 2026-10-06: the -wal file was five hours newer than the .db).
# `sqlite3 .backup` uses SQLite's online-backup API, which reads through the WAL
# and yields one consistent file while the container keeps writing. The source
# is opened -readonly; the host user needs read access to the data dir and write
# access for the new file, no sudo.
#
# This is belt-and-suspenders for releases that carry a DB migration. For a
# pure frontend/CI release (no schema change) it is non-critical: rollback is
# just redeploying the previous image against the unchanged, compatible DB.
#
# Usage:
#   scripts/snapshot-prod-db.sh [user@host] [tag]
#
#   user@host  SSH target (default: $SHELLYADMIN_SSH_HOST or your-docker-host)
#   tag        label embedded in the filename (default: manual);
#              pass the target version, e.g. v0.3.6
#
# Override the data directory with SHELLYADMIN_DATA_DIR (default
# /srv/shellyadmin). Result filename: shellyctl.db.pre-<tag>-<epoch>.
set -euo pipefail

HOST="${1:-${SHELLYADMIN_SSH_HOST:-your-docker-host}}"
TAG="${2:-manual}"
DATA_DIR="${SHELLYADMIN_DATA_DIR:-/srv/shellyadmin}"

# shellcheck disable=SC2029  # we intentionally expand $DATA_DIR/$TAG locally
# and let $(date) run on the remote host.
ssh "$HOST" "
  set -euo pipefail
  src='${DATA_DIR}/shellyctl.db'
  dst='${DATA_DIR}/shellyctl.db.pre-${TAG}-'\$(date +%s)
  if [ ! -f \"\$src\" ]; then
    echo \"error: \$src not found on ${HOST}\" >&2
    exit 1
  fi
  command -v sqlite3 >/dev/null || { echo 'error: sqlite3 not installed on ${HOST}' >&2; exit 1; }
  sqlite3 -readonly \"\$src\" \".backup '\$dst'\"
  # The copy inherits WAL mode; switch it to a self-contained single file so no
  # -wal/-shm pair appears next to it. The app re-enables WAL on open
  # (internal/db/db.go), so restoring this file needs no extra step.
  sqlite3 \"\$dst\" 'PRAGMA journal_mode=DELETE;' >/dev/null
  check=\$(sqlite3 -readonly \"\$dst\" 'PRAGMA integrity_check;')
  if [ \"\$check\" != ok ]; then
    echo \"error: integrity_check on \$dst returned: \$check\" >&2
    exit 1
  fi
  echo \"snapshot created: \$dst (\$(du -h \"\$dst\" | cut -f1), integrity_check ok)\"
  echo 'recent snapshots:'
  ls -1t '${DATA_DIR}'/shellyctl.db.pre-* | head -5
"
