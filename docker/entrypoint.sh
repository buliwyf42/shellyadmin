#!/bin/sh
set -eu

load_secret() {
  var_name="$1"
  file_var="${var_name}_FILE"
  eval "file_path=\${$file_var:-}"
  if [ -n "${file_path:-}" ] && [ -r "$file_path" ]; then
    value="$(cat "$file_path")"
    # Match the app's TrimSpace behavior while still resolving secrets before dropping privileges.
    value="$(printf '%s' "$value" | sed 's/[[:space:]]*$//')"
    export "$var_name=$value"
  fi
  unset "$file_var"
}

# S14 — best-effort chown of /data so a fresh bind-mount (which arrives
# root-owned) becomes writeable by the shelly user. Only attempted when
# we are running as root; under "USER shelly" or when CAP_CHOWN was
# dropped this becomes a no-op and the operator must pre-chown the host
# path. Always continues even if chown fails — the app will report a
# clearer error if it can't write to /data.
if [ "$(id -u)" = "0" ]; then
  mkdir -p /data /tmp 2>/dev/null || true
  chown -R shelly:shelly /data /tmp 2>/dev/null || true
fi

load_secret SHELLYADMIN_PASS_HASH
load_secret SHELLYADMIN_SECRET
load_secret SHELLYADMIN_MCP_TOKEN
load_secret SHELLYADMIN_ENCRYPTION_KEY

# When invoked as root (compose without `user:` override) drop privileges
# via su-exec. When the container is started under a non-root UID (USER
# directive or `user:` override) just exec the binary directly.
if [ "$(id -u)" = "0" ]; then
  exec su-exec shelly /usr/local/bin/shellyctl "$@"
fi
exec /usr/local/bin/shellyctl "$@"
