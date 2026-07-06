#!/bin/sh
set -eu

CONFIG_PATH="${CONFIG_PATH:-/app/config/config.yaml}"
CONFIG_EXAMPLE_PATH="${CONFIG_EXAMPLE_PATH:-/app/config.yaml.example}"

if [ ! -f "$CONFIG_PATH" ]; then
  mkdir -p "$(dirname "$CONFIG_PATH")"
  cp "$CONFIG_EXAMPLE_PATH" "$CONFIG_PATH"
fi

exec /app/vo-hive "$@"
