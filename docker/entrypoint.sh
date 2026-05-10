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

mkdir -p /data /tmp
chown -R shelly:shelly /data /tmp

load_secret SHELLYADMIN_PASS_HASH
load_secret SHELLYADMIN_SECRET

exec su-exec shelly /usr/local/bin/shellyctl
