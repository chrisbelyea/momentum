#!/usr/bin/env bash
set -euo pipefail

LB_BIN=${LB_BIN:-liquibase}
DEFAULTS_FILE=${DEFAULTS_FILE:-liquibase/liquibase.properties}

if [[ ! -f "$DEFAULTS_FILE" ]]; then
  echo "Defaults file not found: $DEFAULTS_FILE" >&2
  exit 1
fi

cmd=${1:-update}

$LB_BIN --defaultsFile="$DEFAULTS_FILE" "$@"
