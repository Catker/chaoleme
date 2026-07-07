#!/usr/bin/env bash
set -u

period="${1:-daily}"
bin="${CHAOLEME_BIN:-chaoleme}"
config="${CHAOLEME_CONFIG:-}"

case "$period" in
  daily|weekly|monthly) ;;
  *)
    echo "用法: $0 daily|weekly|monthly" >&2
    exit 2
    ;;
esac

config_args=()
if [ -n "$config" ]; then
  config_args=(--config "$config")
fi

"$bin" "${config_args[@]}" --verify-evidence "$period"
exit $?
