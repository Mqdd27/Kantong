#!/bin/sh
# Copies the newest daily backup off the Docker volume: integrity-checks it, encrypts it with age,
# and ships it to DEST (a dir on another disk, or an rclone remote). Run on the host as root
# (the volume dir is root-only), once a day any time after the app's 03:00 WIB backup.
# Needs: sqlite3, age, and rclone if DEST is a remote.
set -eu
umask 077

SRC=${SRC:-/var/lib/docker/volumes/kantong_money-data/_data/backups}
DEST=${DEST:?set DEST to a dir on another disk (/mnt/usb/kantong) or an rclone remote (gdrive:kantong)}
RECIPIENT=${AGE_RECIPIENT:?set AGE_RECIPIENT to your age public key (age1...)}

latest=$(ls "$SRC"/money-*.db 2>/dev/null | sort | tail -n 1) # dated names sort by date
[ -n "$latest" ] || { echo "no money-*.db in $SRC" >&2; exit 1; }
[ -n "$(find "$latest" -mtime -2)" ] || { echo "newest backup $latest is over 2 days old; is the app running?" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp "$latest" "$tmp/db"
# restore test: the copy we ship must open and pass SQLite's own check
[ "$(sqlite3 "$tmp/db" 'PRAGMA integrity_check')" = ok ] || { echo "integrity_check failed on $latest" >&2; exit 1; }

name=$(basename "$latest").age
age -r "$RECIPIENT" -o "$tmp/$name" "$tmp/db"
case $DEST in
  *:*) rclone copyto "$tmp/$name" "$DEST/$name" ;;
  *) mkdir -p "$DEST" && cp "$tmp/$name" "$DEST/$name" ;;
esac
echo "offsite backup: $name -> $DEST"
