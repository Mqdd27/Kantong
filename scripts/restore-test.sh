#!/bin/sh
# Proves an offsite backup can actually be restored: fetches one .age copy, decrypts it with the
# private key, and checks the database opens, passes integrity_check and holds recent data.
# Run it on a clean machine (your laptop), NOT the server: the server must never hold the key.
# Needs: age, sqlite3, and rclone if the backup is on a remote. Nothing is restored into the app.
#   KEY=~/kantong.key scripts/restore-test.sh gdrive:kantong/money-2026-11-08.db.age
#   KEY=~/kantong.key scripts/restore-test.sh /mnt/usb/kantong              # newest copy in a dir
set -eu
umask 077

SRC=${1:?usage: KEY=path/to/kantong.key $0 <backup.db.age | dir | rclone-remote:path>}
KEY=${KEY:?set KEY to the age private key file (kept off the server)}
MAX_DAYS=${MAX_DAYS:-3}

[ -d /var/lib/docker/volumes/kantong_money-data ] && { echo "this looks like the Kantong server; run the restore test elsewhere" >&2; exit 1; }
grep -q '^AGE-SECRET-KEY-' "$KEY" || { echo "$KEY is not an age private key" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

case $SRC in
  *.age) file=$SRC ;;
  *:*) file=$SRC/$(rclone lsf --include 'money-*.db.age' "$SRC" | sort | tail -n 1) ;; # dated names sort by date
  *) file=$(ls "$SRC"/money-*.db.age | sort | tail -n 1) ;;
esac
case $file in
  *:*) rclone copyto "$file" "$tmp/db.age" ;;
  *) cp "$file" "$tmp/db.age" ;;
esac

age -d -i "$KEY" -o "$tmp/db" "$tmp/db.age"
[ "$(sqlite3 "$tmp/db" 'PRAGMA integrity_check')" = ok ] || { echo "integrity_check failed on $file" >&2; exit 1; }
for t in transactions assets recurring; do
  [ "$(sqlite3 "$tmp/db" "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = '$t'")" = 1 ] ||
    { echo "table $t missing from $file" >&2; exit 1; }
done

taken=$(basename "$file" | sed -n 's/^money-\([0-9-]*\)\.db\.age$/\1/p')
if [ -n "$taken" ]; then
  days=$(( ($(date +%s) - $(date -d "$taken" +%s 2>/dev/null || date -j -f %Y-%m-%d "$taken" +%s)) / 86400 ))
  [ "$days" -le "$MAX_DAYS" ] || { echo "newest copy $file is $days days old; is the offsite cron running?" >&2; exit 1; }
fi

sqlite3 "$tmp/db" "SELECT 'restore ok: ' || (SELECT count(*) FROM transactions) || ' transactions, ' ||
  (SELECT count(*) FROM assets) || ' assets, newest ' || COALESCE((SELECT max(substr(occurred_at, 1, 10)) FROM transactions), '-')"
echo "from $file"
