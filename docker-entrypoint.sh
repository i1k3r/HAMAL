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
#     identity. They are optional and never required. Each one replaces only
#     its own half: PUID alone keeps the detected (or default) GID, PGID alone
#     keeps the detected (or default) UID. Values must be whole numbers from
#     1 to 4294967294; 0 (root) and anything else are rejected.
#   * LAN_DROP_DATA_DIR and LAN_DROP_DB_PATH are honoured the same way the
#     server reads them.
#   * When the container is started with --user, this script does nothing and
#     runs the server as that user, exactly like older images did.
#
# "healthcheck" as the first argument uses the same identity but never changes
# anything, so Docker health checks also run without root.
set -eu

APP=/app/hamal
DEFAULT_UID=99
DEFAULT_GID=100
# Largest valid ID. 4294967295 is (uid_t)-1, which setresuid treats as
# "leave unchanged", so it would keep the process running as root.
MAX_ID=4294967294

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

# Same rule as the server's envString: surrounding whitespace is ignored and
# an empty value means "use the default".
trim() {
    printf '%s' "$1" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//'
}

DATA_DIR=$(trim "${LAN_DROP_DATA_DIR:-}")
[ -n "$DATA_DIR" ] || DATA_DIR=/data
DB_PATH=$(trim "${LAN_DROP_DB_PATH:-}")
[ -n "$DB_PATH" ] || DB_PATH="$DATA_DIR/lan-drop.db"

# parse_id NAME VALUE: prints VALUE as a plain decimal non-root ID, or fails.
parse_id() {
    value=$(trim "$2")
    case "$value" in
        '' | *[!0-9]*) fail "$1 must be a whole number between 1 and $MAX_ID, got '$2'." ;;
    esac
    value=$(printf '%s' "$value" | sed 's/^0*//')
    [ -n "$value" ] || fail "$1=0 would run HAMAL as root. Use a non-root ID between 1 and $MAX_ID."
    if [ "${#value}" -gt 10 ] || [ "$value" -gt "$MAX_ID" ]; then
        fail "$1 must be a whole number between 1 and $MAX_ID, got '$2'."
    fi
    printf '%s' "$value"
}

describe() {
    stat -c '%n (owner %u:%g, mode %a)' "$1" 2>/dev/null || echo "$1"
}

as_app() {
    setpriv --reuid="$RUN_UID" --regid="$RUN_GID" --clear-groups "$@"
}

# 1. Decide the runtime identity from existing data. Most specific first: the
#    secret file is mode 0600, so its owner is the only identity that can read
#    it. If the data directory does not exist yet, its nearest existing parent
#    decides (e.g. LAN_DROP_DATA_DIR=/data/hamal on a volume owned by 10001).
#    Root-owned paths say nothing about the intended user and are skipped.
RUN_UID=""
RUN_GID=""
SOURCE=""
candidates="$DATA_DIR/secrets/server-secret
$DB_PATH
$DATA_DIR/secrets
$DATA_DIR/files
$DATA_DIR/staging
$DATA_DIR"
parent=$(dirname "$DATA_DIR")
while [ "$parent" != / ] && [ "$parent" != . ]; do
    candidates="$candidates
$parent"
    parent=$(dirname "$parent")
done
while IFS= read -r candidate; do
    [ -e "$candidate" ] || continue
    owner=$(stat -c '%u:%g' "$candidate")
    if [ "${owner%%:*}" != 0 ]; then
        RUN_UID="${owner%%:*}"
        RUN_GID="${owner##*:}"
        SOURCE="owner of $candidate"
        break
    fi
done <<CANDIDATES
$candidates
CANDIDATES
if [ -z "$RUN_UID" ]; then
    RUN_UID=$DEFAULT_UID
    RUN_GID=$DEFAULT_GID
    SOURCE="default for a new data directory"
fi
# A non-root owner with group root (e.g. 1000:0): keep the user, not group 0.
[ "$RUN_GID" != 0 ] || RUN_GID=$DEFAULT_GID

# Optional overrides. Each one replaces only its own half; the other half
# keeps the detected (or default) value.
if [ -n "$(trim "${PUID:-}")" ]; then
    RUN_UID=$(parse_id PUID "$PUID")
    SOURCE="UID from PUID; detected: $SOURCE"
fi
if [ -n "$(trim "${PGID:-}")" ]; then
    RUN_GID=$(parse_id PGID "$PGID")
    SOURCE="GID from PGID; $SOURCE"
fi
if [ "$RUN_UID" -eq 0 ] || [ "$RUN_UID" -gt "$MAX_ID" ] || [ "$RUN_GID" -eq 0 ] || [ "$RUN_GID" -gt "$MAX_ID" ]; then
    fail "refusing to run the HAMAL server as $RUN_UID:$RUN_GID. Set PUID/PGID to a non-root user and group."
fi

if [ "$MODE" = healthcheck ]; then
    # No changes and no preflight here: "/app/hamal healthcheck" itself
    # creates and removes a test file in every storage directory, reads the
    # secret and opens the database, all as the same identity as the server.
    exec setpriv --reuid="$RUN_UID" --regid="$RUN_GID" --clear-groups "$APP" "$@"
fi

# 2. Read-only storage is reported as such, not as a permission problem.
existing="$DATA_DIR"
while [ ! -e "$existing" ]; do
    existing=$(dirname "$existing")
done
absolute=$(readlink -f "$existing")
if awk -v d="$absolute" '
    $2 == "/" || $2 == d || index(d, $2 "/") == 1 {
        if (length($2) >= best) { best = length($2); split($4, o, ","); ro = (o[1] == "ro") }
    }
    END { exit !ro }' /proc/self/mounts; then
    fail "$DATA_DIR is on a read-only mount ($absolute). HAMAL needs a writable data directory for its database, uploads and secrets: map it read-write (Unraid: Access Mode \"Read/Write\")."
fi

# 3. Create a missing data directory (the server would create it too), owned
#    by the runtime user. These directories are new and empty.
if [ ! -e "$DATA_DIR" ]; then
    missing=""
    path="$DATA_DIR"
    while [ ! -e "$path" ]; do
        missing="$path
$missing"
        path=$(dirname "$path")
    done
    while IFS= read -r path; do
        [ -n "$path" ] || continue
        if ! { mkdir "$path" 2>/dev/null && chown "$RUN_UID:$RUN_GID" "$path" 2>/dev/null; }; then
            fail "could not create $path for the data directory. Map a writable host folder or volume to $DATA_DIR."
        fi
        log "created $path for $RUN_UID:$RUN_GID."
    done <<MISSING
$missing
MISSING
fi
[ -d "$DATA_DIR" ] || fail "$DATA_DIR is not a directory."

# 4. A fresh root-owned mount point: hand that single directory to the
#    runtime user. Contents are never touched.
if ! as_app test -w "$DATA_DIR" && [ "$(stat -c '%u' "$DATA_DIR")" = 0 ]; then
    log "giving the new data directory $DATA_DIR to $RUN_UID:$RUN_GID (this directory only, not its contents)."
    chown "$RUN_UID:$RUN_GID" "$DATA_DIR" 2>/dev/null \
        || fail "could not change the owner of $DATA_DIR. The host folder may be on a network share that blocks chown, or the container may lack CAP_CHOWN. Make the host folder writable for $RUN_UID:$RUN_GID or set PUID/PGID to its owner."
fi

# 5. Preflight: report every path the server will need but cannot use,
#    instead of a single "permission denied" from the first one it hits.
#    These are the paths storage.Initialize and database.Open use.
problems=""
db_dir=$(dirname "$DB_PATH")
for dir in "$DATA_DIR" "$DATA_DIR/files" "$DATA_DIR/staging" "$DATA_DIR/secrets" "$db_dir"; do
    if [ -e "$dir" ] && ! as_app test -w "$dir" -a -x "$dir"; then
        problems="$problems
  - not writable: $(describe "$dir")"
    fi
done
if [ ! -d "$db_dir" ]; then
    problems="$problems
  - missing directory for the database (LAN_DROP_DB_PATH=$DB_PATH): $db_dir"
fi
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
