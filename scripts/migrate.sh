#!/usr/bin/env bash
set -euo pipefail

direction="${1:-up}"
database_url="${CLUSTERSTOR_DATABASE_URL:-postgres://clusterstor:clusterstor@localhost:5432/clusterstor?sslmode=disable}"

case "$direction" in
  up)
    for file in db/migrations/*.up.sql; do
      echo "applying $file"
      psql "$database_url" -v ON_ERROR_STOP=1 -f "$file"
    done
    ;;
  down)
    for file in $(find db/migrations -name '*.down.sql' | sort -r); do
      echo "reverting $file"
      psql "$database_url" -v ON_ERROR_STOP=1 -f "$file"
    done
    ;;
  *)
    echo "usage: $0 [up|down]" >&2
    exit 2
    ;;
esac
