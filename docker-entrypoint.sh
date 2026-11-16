#!/bin/sh
# HAMAL container entrypoint.
#
# The HAMAL server itself never runs as root. When the container is started as
# root (the default for this image, and for the Unraid template), this script
# only decides which non-root UID:GID to run as, makes at most one ownership
# change, and then drops privileges with setpriv before exec'ing the server.
#
# Rules:
#   * Existing data decides the identity. If HAMAL data already exists under
#     the data directory, run as the owner of that data (10001:10001 for
#     installs from older images and named volumes, 99:100 for Unraid installs
#     that used --user 99:100, and so on). Nothing is chowned in that case.
#   * A brand-new, root-owned data directory (what Docker creates for a missing
#     bind-mount source such as /mnt/user/appdata/hamal) is handed to 99:100,
#     Unraid's nobody:users. Only that one directory inode is chowned, never
#     its contents, and only when it is owned by root and not writable.
#   * PUID / PGID override the detection for operators who need a specific
#     identity. They are optional and never required.
#   * When the container is started with --user, this script does nothing and
#     runs the server as that user, exactly like older images did.
#
# "healthcheck" as the first argument uses the same identity but never changes
# anything, so Docker health checks also run without root.
set -eu

APP=/app/hamal
DATA_DIR="${LAN_DROP_DATA_DIR:-/data}"
DB_PATH="${LAN_DROP_DB_PATH:-$DATA_DIR/lan-drop.db}"
DEFAULT_UID=99
DEFAULT_GID=100

if [ "$(id -u)" -ne 0 ]; then
    exec "$APP" "$@"
fi

MODE=run
if [ "${1:-}" = "healthcheck" ]; then
    MODE=healthcheck
fi

log() {
    if [ "$MODE" = run ]; then
        echo "HAMAL: $*" >&2
    fi
}

fail() {
    echo "HAMAL: $*" >&2
    exit 1
}

is_uint() {
    case "$1" in
        '' | *[!0-9]*) return 1 ;;
    esac
    return 0
}

describe() {
    stat -c '%n (owner %u:%g, mode %a)' "$1" 2>/dev/null || echo "$1"
}

as_app() {
    setpriv --reuid="$RUN_UID" --regid="$RUN_GID" --clear-groups "$@"
}

# 1. Decide the runtime identity.
RUN_UID=""
RUN_GID=""
SOURCE=""
if [ -n "${PUID:-}" ] || [ -n "${PGID:-}" ]; then
    RUN_UID="${PUID:-$DEFAULT_UID}"
    RUN_GID="${PGID:-$DEFAULT_GID}"
    is_uint "$RUN_UID" || fail "PUID must be a number, got '$RUN_UID'."
    is_uint "$RUN_GID" || fail "PGID must be a number, got '$RUN_GID'."
    SOURCE="PUID/PGID"
else
    # Most specific first: the secret file is mode 0600, so its owner is the
    # only identity that can read it.
    for candidate in \
        "$DATA_DIR/secrets/server-secret" \
        "$DB_PATH" \
        "$DATA_DIR/secrets" \
        "$DATA_DIR/files" \
        "$DATA_DIR/staging" \
        "$DATA_DIR"; do
        [ -e "$candidate" ] || continue
        owner=$(stat -c '%u:%g' "$candidate")
        if [ "${owner%%:*}" != 0 ]; then
            RUN_UID="${owner%%:*}"
            RUN_GID="${owner##*:}"
            SOURCE="owner of $candidate"
            break
        fi
    done
    if [ -z "$RUN_UID" ]; then
        RUN_UID=$DEFAULT_UID
        RUN_GID=$DEFAULT_GID
        SOURCE="default for a new data directory"
    fi
fi
[ "$RUN_UID" -ne 0 ] || fail "refusing to run the HAMAL server as root (UID 0). Set PUID/PGID to a non-root user."

if [ "$MODE" = healthcheck ]; then
    exec setpriv --reuid="$RUN_UID" --regid="$RUN_GID" --clear-groups "$APP" "$@"
fi

[ -d "$DATA_DIR" ] || fail "data directory $DATA_DIR does not exist. Map a host folder or volume to $DATA_DIR."
if awk -v d="$DATA_DIR" '$2 == d { split($4, o, ","); ro = (o[1] == "ro") } END { exit !ro }' /proc/self/mounts; then
    fail "$DATA_DIR is mounted read-only. HAMAL needs a writable $DATA_DIR for its database, uploads and secrets: map it read-write (Unraid: Access Mode \"Read/Write\")."
fi

# 2. A fresh root-owned mount point: hand that single directory to the
#    runtime user. Contents are never touched.
if ! as_app test -w "$DATA_DIR" && [ "$(stat -c '%u' "$DATA_DIR")" = 0 ]; then
    log "giving the new data directory $DATA_DIR to $RUN_UID:$RUN_GID (this directory only, not its contents)."
    chown "$RUN_UID:$RUN_GID" "$DATA_DIR" 2>/dev/null \
        || fail "could not change the owner of $DATA_DIR. The host folder may be read-only, on a network share that blocks chown, or the container may lack CAP_CHOWN. Make the host folder writable for $RUN_UID:$RUN_GID or set PUID/PGID to its owner."
fi

# 3. Preflight: report every path the server will need but cannot use,
#    instead of a single "permission denied" from the first one it hits.
problems=""
for dir in "$DATA_DIR" "$DATA_DIR/files" "$DATA_DIR/staging" "$DATA_DIR/secrets"; do
    if [ -e "$dir" ] && ! as_app test -w "$dir" -a -x "$dir"; then
        problems="$problems
  - not writable: $(describe "$dir")"
    fi
done
for file in "$DATA_DIR/secrets/server-secret" "$DB_PATH" "$DB_PATH-wal" "$DB_PATH-shm"; do
    if [ -e "$file" ] && ! as_app test -r "$file" -a -w "$file"; then
        problems="$problems
  - not readable/writable: $(describe "$file")"
    fi
done
if [ -n "$problems" ]; then
    fail "HAMAL runs as UID:GID $RUN_UID:$RUN_GID ($SOURCE), but it cannot use:$problems
HAMAL does not change the ownership of existing files automatically.
Fix the ownership on the host (for Unraid: chown -R $RUN_UID:$RUN_GID on the appdata folder), or set PUID/PGID to the owner of the existing data."
fi

log "starting as UID:GID $RUN_UID:$RUN_GID ($SOURCE)."
exec setpriv --reuid="$RUN_UID" --regid="$RUN_GID" --clear-groups "$APP" "$@"
