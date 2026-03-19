#!/bin/sh
# nightly SQLite backup with 7-day retention
# usage: backup.sh [database_path] [backup_directory]

set -eu

DB_PATH="${1:-/data/otterly.db}"
BACKUP_DIR="${2:-$HOME/otterly/backups}"
DATE="$(date +%Y-%m-%d)"
BACKUP_FILE="$BACKUP_DIR/otterly-$DATE.db"

if [ ! -f "$DB_PATH" ]; then
  echo "error: database not found at $DB_PATH"
  exit 1
fi

mkdir -p "$BACKUP_DIR"

echo "backing up $DB_PATH to $BACKUP_FILE"
sqlite3 "$DB_PATH" ".backup '$BACKUP_FILE'"
echo "backup complete: $BACKUP_FILE"

echo "pruning backups older than 7 days"
find "$BACKUP_DIR" -name "otterly-*.db" -type f -mtime +7 -print -delete
echo "done"
