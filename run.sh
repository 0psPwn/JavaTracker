#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${1:-}"
ADDR="${ADDR:-:8090}"

cd "$(dirname "$0")"

exec go run ./cmd/javatracker -addr "${ADDR}" ${ROOT_DIR:+-root "${ROOT_DIR}"}
