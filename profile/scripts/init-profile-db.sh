#!/bin/sh
set -eu

DB_HOST="${PROFILE_DB_HOST:-profile-postgres}"
DB_USER="${PROFILE_DB_USER:-postgres}"
TARGET_DB="${PROFILE_DB_NAME:-snooker_game}"
LEGACY_DB="${PROFILE_LEGACY_DB_NAME:-snooker_profile}"
PUBLIC_BASE_URL="${STORAGE_PUBLIC_BASE_URL:-}"

export PGPASSWORD="${PGPASSWORD:-postgres}"

until pg_isready -h "$DB_HOST" -U "$DB_USER" -d postgres; do
  sleep 1
done

database_exists() {
  psql -h "$DB_HOST" -U "$DB_USER" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$1'" | grep -q 1
}

table_exists() {
  psql -h "$DB_HOST" -U "$DB_USER" -d "$1" -tAc "SELECT to_regclass('public.$2') IS NOT NULL" | grep -q t
}

table_count() {
  if table_exists "$1" "$2"; then
    psql -h "$DB_HOST" -U "$DB_USER" -d "$1" -tAc "SELECT COUNT(*) FROM $2"
  else
    echo 0
  fi
}

if ! database_exists "$TARGET_DB"; then
  if [ "$TARGET_DB" != "$LEGACY_DB" ] && database_exists "$LEGACY_DB"; then
    createdb -h "$DB_HOST" -U "$DB_USER" -T "$LEGACY_DB" "$TARGET_DB"
    exit 0
  fi

  createdb -h "$DB_HOST" -U "$DB_USER" "$TARGET_DB"
fi

if [ "$TARGET_DB" = "$LEGACY_DB" ] || ! database_exists "$LEGACY_DB"; then
  exit 0
fi

LEGACY_PROFILE_COUNT="$(table_count "$LEGACY_DB" profiles)"
TARGET_PROFILE_COUNT="$(table_count "$TARGET_DB" profiles)"

if [ "$LEGACY_PROFILE_COUNT" -gt 0 ] && [ "$TARGET_PROFILE_COUNT" -eq 0 ]; then
  psql -h "$DB_HOST" -U "$DB_USER" -d "$TARGET_DB" -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";'

  if ! table_exists "$TARGET_DB" profiles; then
    pg_dump -h "$DB_HOST" -U "$DB_USER" --schema-only --no-owner --no-privileges \
      -t profiles -t photo_upload_sessions "$LEGACY_DB" |
      psql -v ON_ERROR_STOP=1 -h "$DB_HOST" -U "$DB_USER" -d "$TARGET_DB"
  fi

  pg_dump -h "$DB_HOST" -U "$DB_USER" --data-only --inserts --on-conflict-do-nothing \
    -t profiles -t photo_upload_sessions "$LEGACY_DB" |
    psql -v ON_ERROR_STOP=1 -h "$DB_HOST" -U "$DB_USER" -d "$TARGET_DB"
fi

if [ -n "$PUBLIC_BASE_URL" ] && table_exists "$TARGET_DB" profiles; then
  PUBLIC_BASE_URL="${PUBLIC_BASE_URL%/}"
  ESCAPED_PUBLIC_BASE_URL="$(printf "%s" "$PUBLIC_BASE_URL" | sed "s/'/''/g")"
  psql -h "$DB_HOST" -U "$DB_USER" -d "$TARGET_DB" \
    -c "UPDATE profiles SET photo_url = '$ESCAPED_PUBLIC_BASE_URL' || '/' || photo_object_key WHERE photo_object_key <> '';"
fi
