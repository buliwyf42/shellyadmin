#!/usr/bin/env bash
#
# Pre-deploy snapshot of the production ShellyAdmin SQLite database.
#
# Why a host-side script: the database lives on the Docker host bind mount
# (/docker/shellyadmin/shellyctl.db). The Dockhand MCP `exec_container` tool
# cannot create the snapshot file inside the read-only-rootfs container, so
# the copy runs on the host over SSH instead.
#
# This is belt-and-suspenders for releases that carry a DB migration. For a
# pure frontend/CI release (no schema change) it is non-critical: rollback is
# just redeploying the previous image against the unchanged, compatible DB.
#
# Usage:
#   scripts/snapshot-prod-db.sh [user@host] [tag]
#
#   user@host  SSH target (default: docker.home.lan)
#   tag        label embedded in the filename (default: manual);
#              pass the target version, e.g. v0.3.6
#
# Override the data directory with SHELLYADMIN_DATA_DIR (default
# /docker/shellyadmin). Result filename: shellyctl.db.pre-<tag>-<epoch>.
set -euo pipefail

HOST="${1:-docker.home.lan}"
TAG="${2:-manual}"
DATA_DIR="${SHELLYADMIN_DATA_DIR:-/docker/shellyadmin}"

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
  cp \"\$src\" \"\$dst\"
  echo \"snapshot created: \$dst (\$(du -h \"\$dst\" | cut -f1))\"
  echo 'recent snapshots:'
  ls -1t '${DATA_DIR}'/shellyctl.db.pre-* | head -5
"
