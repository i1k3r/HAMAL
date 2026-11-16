#!/usr/bin/env bash
# Regression tests for the Docker image's data-directory permission handling
# (docker-entrypoint.sh). Runs real containers with the production security
# options and checks identity, ownership, health checks and error messages.
#
# Usage: scripts/test-docker-permissions.sh <image>
# Needs: docker, curl, sha256sum, and root or passwordless sudo (test folders
# are created with specific owners). Nothing outside a temporary folder and
# containers/volumes named hamal-permtest-* is touched.
set -uo pipefail

IMG=${1:?usage: $0 <image>}
SUDO=""
[ "$(id -u)" = 0 ] || SUDO="sudo"
SEC=(--read-only --tmpfs /tmp --security-opt no-new-privileges:true --health-interval=2s --health-start-period=1s --health-retries=1)
WORK=$(mktemp -d)
PFX=hamal-permtest
pass=0
failures=0

ok() { echo "  PASS: $*"; pass=$((pass + 1)); }
bad() { echo "  FAIL: $*"; failures=$((failures + 1)); }
check() { local what=$1; shift; if "$@"; then ok "$what"; else bad "$what"; fi; }

cleanup() {
    docker ps -aq --filter "name=^$PFX-" | xargs -r docker rm -f >/dev/null 2>&1
    docker volume ls -q --filter "name=^$PFX-" | xargs -r docker volume rm -f >/dev/null 2>&1
    $SUDO rm -rf "$WORK"
}
trap cleanup EXIT

ip() { docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$1"; }
running() { [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" = true ]; }
health() {
    local s=""
    for _ in $(seq 1 30); do
        s=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$1" 2>/dev/null)
        case "$s" in healthy | unhealthy) echo "$s"; return ;; esac
        running "$1" || { echo exited; return; }
        sleep 1
    done
    echo "${s:-none}"
}
healthy() { [ "$(health "$1")" = healthy ]; }
becomes_unhealthy() {
    for _ in $(seq 1 20); do
        [ "$(docker inspect -f '{{.State.Health.Status}}' "$1")" = unhealthy ] && return 0
        sleep 1
    done
    return 1
}
proc_id() { docker exec "$1" sh -c 'awk "/^Uid/{u=\$2} /^Gid/{g=\$2} END{print u\":\"g}" /proc/1/status' 2>/dev/null; }
has_id() { [ "$(proc_id "$1")" = "$2" ]; }
exited_with() { # name text: container exited non-zero and its log contains text
    for _ in $(seq 1 15); do running "$1" || break; sleep 1; done
    ! running "$1" && [ "$(docker inspect -f '{{.State.ExitCode}}' "$1")" != 0 ] && docker logs "$1" 2>&1 | grep -qF -- "$2"
}
start() { # name hostdir|volume [docker args...]
    local n=$PFX-$1 d=$2; shift 2
    docker rm -f "$n" >/dev/null 2>&1
    docker run -d --name "$n" "${SEC[@]}" "$@" -v "$d:/data${RO:+:ro}" "$IMG" >/dev/null
}
snap() { $SUDO find "$1" -printf '%u:%g %m %P\n' | grep -v -e '-wal$' -e '-shm$' -e 'lan-drop-check' | sort; }
fresh() { mkdir -p "$WORK/$1"; echo "$WORK/$1"; } # root-owned when created via sudo
seeded() { # name owner -> copy of seed data chowned to owner
    local d=$WORK/$1
    $SUDO mkdir -p "$d" && $SUDO cp -a "$WORK/seed/." "$d/" && $SUDO chown -R "$2" "$d"
    echo "$d"
}
download_ok() {
    local h
    h=$(curl -s -b "$WORK/cookies" "http://$(ip "$1"):7700/api/v1/rooms/$PTOK/files/$FID" | sha256sum | cut -d' ' -f1)
    [ "$h" = "$WANT" ]
}
new_room_ok() { curl -sf -H 'Content-Type: application/json' -H "Origin: http://$(ip "$1"):7700" -d '{}' "http://$(ip "$1"):7700/api/v1/rooms" >/dev/null; }
no_root_files() { [ -z "$($SUDO find "$1" -uid 0 -mindepth 1 -print -quit)" ]; }

mkdir() { $SUDO env mkdir "$@"; }

echo "== seed: real room + upload created by the image itself, owned 10001:10001"
mkdir -p "$WORK/seed" && $SUDO chown 10001:10001 "$WORK/seed"
start seed "$WORK/seed" --user 10001:10001
healthy $PFX-seed || { docker logs $PFX-seed; exit 1; }
SIP=$(ip $PFX-seed)
head -c 200000 /dev/urandom >"$WORK/payload.bin"
WANT=$(sha256sum "$WORK/payload.bin" | cut -d' ' -f1)
PTOK=$(curl -s -c "$WORK/cookies" -b "$WORK/cookies" -H 'Content-Type: application/json' -H "Origin: http://$SIP:7700" -d '{"ttl_seconds":86400}' "http://$SIP:7700/api/v1/rooms" | sed -n 's/.*"participant_token":"\([^"]*\)".*/\1/p')
FID=$(curl -s -c "$WORK/cookies" -b "$WORK/cookies" -H "Origin: http://$SIP:7700" -F "file=@$WORK/payload.bin" "http://$SIP:7700/api/v1/rooms/$PTOK/files" | sed -n 's/.*"file_id":"\([^"]*\)".*/\1/p')
if [ -z "$PTOK" ] || [ -z "$FID" ]; then echo "seeding failed"; exit 1; fi
docker rm -f $PFX-seed >/dev/null

echo "== 1. fresh empty host folder (root:root 0755, what Docker creates on Unraid)"
d=$(fresh s1)
start s1 "$d"
check "healthy" healthy $PFX-s1
check "runs as 99:100" has_id $PFX-s1 99:100
check "/data handed to 99:100" [ "$(stat -c %u:%g "$d")" = 99:100 ]
check "new room works" new_room_ok $PFX-s1
check "no root-owned files" no_root_files "$d"

echo "== 2. existing data owned by 99:100"
d=$(seeded s2 99:100); before=$(snap "$d")
start s2 "$d"
check "healthy" healthy $PFX-s2
check "runs as 99:100" has_id $PFX-s2 99:100
check "old upload downloads byte-identical" download_ok $PFX-s2
check "ownership unchanged" [ "$before" = "$(snap "$d")" ]

echo "== 3. existing data owned by 10001:10001"
d=$(seeded s3 10001:10001); before=$(snap "$d")
start s3 "$d"
check "healthy" healthy $PFX-s3
check "runs as 10001:10001" has_id $PFX-s3 10001:10001
check "old upload downloads byte-identical" download_ok $PFX-s3
check "ownership unchanged" [ "$before" = "$(snap "$d")" ]

echo "== 4a. mixed: /data 99:100 0777, contents 10001:10001"
d=$(seeded s4a 10001:10001); $SUDO chown 99:100 "$d"; $SUDO chmod 777 "$d"; before=$(snap "$d")
start s4a "$d"
check "healthy" healthy $PFX-s4a
check "runs as 10001:10001 (secret owner)" has_id $PFX-s4a 10001:10001
check "old upload downloads" download_ok $PFX-s4a
check "ownership unchanged" [ "$before" = "$(snap "$d")" ]

echo "== 4b. mixed and broken: files/ owned by 99:100 0750, rest 10001"
d=$(seeded s4b 10001:10001); $SUDO chown -R 99:100 "$d/files"; before=$(snap "$d")
start s4b "$d"
check "refuses to start and names the path" exited_with $PFX-s4b "not writable: /data/files (owner 99:100, mode 750)"
check "nothing changed on disk" [ "$before" = "$(snap "$d")" ]

echo "== 4c. data owned by a user with group root (1000:0)"
d=$(seeded s4c 1000:0); before=$(snap "$d")
start s4c "$d"
check "healthy" healthy $PFX-s4c
check "runs as 1000:100 (never group 0)" has_id $PFX-s4c 1000:100
check "ownership unchanged" [ "$before" = "$(snap "$d")" ]

echo "== 5. read-only /data"
d=$(seeded s5 99:100)
RO=1 start s5 "$d"
check "existing data: read-only message" exited_with $PFX-s5 "read-only mount"
d=$(fresh s5b)
RO=1 start s5b "$d"
check "empty folder: read-only message" exited_with $PFX-s5b "read-only mount"
RO=1 start s5c "$d" -e LAN_DROP_DATA_DIR=/data/
check "trailing slash in LAN_DROP_DATA_DIR: read-only message" exited_with $PFX-s5c "read-only mount"
RO=1 start s5d "$d" -e LAN_DROP_DATA_DIR=/data/sub
check "missing subdirectory on read-only mount: read-only message" exited_with $PFX-s5d "read-only mount"

echo "== 6/7. --read-only and no-new-privileges are in effect"
check "NoNewPrivs=1 and no effective capabilities in the server" \
    sh -c "docker exec $PFX-s3 grep -qx 'NoNewPrivs:[[:space:]]*1' /proc/1/status && docker exec $PFX-s3 grep -qx 'CapEff:[[:space:]]*0000000000000000' /proc/1/status"
check "root filesystem is read-only" sh -c "! docker exec $PFX-s3 touch /app/x 2>/dev/null"

echo "== 9. restart does not change ownership again"
d=$WORK/s1; before=$(snap "$d")
docker restart $PFX-s1 >/dev/null
check "healthy after restart" healthy $PFX-s1
check "ownership unchanged" [ "$before" = "$(snap "$d")" ]
check "directory handed over once in total" [ "$(docker logs $PFX-s1 2>&1 | grep -c 'giving the new data directory')" = 1 ]

echo "== 10. other deployments"
start s10a $PFX-vol
check "fresh named volume: healthy" healthy $PFX-s10a
check "fresh named volume: runs as 10001:10001 like older images" has_id $PFX-s10a 10001:10001
d=$(seeded s10b 99:100)
start s10b "$d" --user 99:100
check "old Unraid template (--user 99:100): healthy" healthy $PFX-s10b
check "old Unraid template: old upload downloads" download_ok $PFX-s10b

echo "== 11. custom LAN_DROP_DATA_DIR / LAN_DROP_DB_PATH"
d=$(fresh s11a)
start s11a "$d" -e LAN_DROP_DATA_DIR=/data/hamal
check "missing subdirectory on a fresh folder: healthy" healthy $PFX-s11a
check "subdirectory created for 99:100" [ "$(stat -c %u:%g "$d/hamal")" = 99:100 ]
check "database created inside it" $SUDO test -f "$d/hamal/lan-drop.db"
start s11b $PFX-vol2 -e LAN_DROP_DATA_DIR=/data/hamal
check "missing subdirectory on a 10001 volume: runs as 10001:10001" bash -c "$(declare -f health running healthy proc_id has_id); PFX=$PFX; healthy $PFX-s11b && has_id $PFX-s11b 10001:10001"
d=$(seeded s11c 99:100)
start s11c "$d" -e "LAN_DROP_DATA_DIR=  /data  "
check "whitespace around LAN_DROP_DATA_DIR is ignored like the server does" healthy $PFX-s11c
check "  ...and the old upload is still there" download_ok $PFX-s11c
d=$(fresh s11d)
start s11d "$d" -e LAN_DROP_DB_PATH=/data/db/hamal.db
check "LAN_DROP_DB_PATH in a missing directory: clear message" exited_with $PFX-s11d "missing directory for the database (LAN_DROP_DB_PATH=/data/db/hamal.db)"
d=$(fresh s11e); mkdir -p "$d/db"; $SUDO chown 99:100 "$d" "$d/db"
start s11e "$d" -e LAN_DROP_DB_PATH=/data/db/hamal.db
check "LAN_DROP_DB_PATH in an existing directory: healthy" healthy $PFX-s11e
check "  ...database at the custom path" $SUDO test -f "$d/db/hamal.db"
d=$(fresh s11f); mkdir -p "$d/db"; $SUDO chown 99:100 "$d"
start s11f "$d" -e LAN_DROP_DB_PATH=/data/db/hamal.db
check "LAN_DROP_DB_PATH directory not writable: names it" exited_with $PFX-s11f "not writable: /data/db (owner 0:0"

echo "== HC. health check"
check "health check command goes through the entrypoint" \
    [ "$(docker inspect -f '{{json .Config.Healthcheck.Test}}' "$IMG")" = '["CMD","/usr/local/bin/docker-entrypoint.sh","healthcheck"]' ]
check "health check drops to the server's identity" \
    sh -c "docker exec $PFX-s3 sh -x /usr/local/bin/docker-entrypoint.sh healthcheck 2>&1 | grep -q 'exec setpriv --reuid=10001 --regid=10001 --clear-groups /app/hamal healthcheck'"
d=$WORK/s2
$SUDO chmod 000 "$d/files"
check "files/ made unusable after start -> unhealthy" becomes_unhealthy $PFX-s2
$SUDO chmod 750 "$d/files"
d=$WORK/s3
$SUDO chown 0:0 "$d/secrets/server-secret"
check "secret made unreadable after start -> unhealthy" becomes_unhealthy $PFX-s3
check "no root-owned files left by health checks" sh -c "for x in $WORK/s1 $WORK/s2 $WORK/s4a $WORK/s10b; do [ -z \"\$($SUDO find \$x -uid 0 -print -quit)\" ] || exit 1; done"

echo "== P. PUID / PGID"
puid_case() { # label expect(uid:gid|fail:text) env...
    local label=$1 expect=$2; shift 2
    local n=p$((++pcount)) d
    d=$(fresh "$n")
    local args=()
    for e in "$@"; do args+=(-e "$e"); done
    start "$n" "$d" "${args[@]}"
    case "$expect" in
        fail:*) check "$label -> refused" exited_with "$PFX-$n" "${expect#fail:}" ;;
        *) check "$label -> $expect" bash -c "$(declare -f health running healthy proc_id has_id); PFX=$PFX; healthy $PFX-$n && has_id $PFX-$n $expect" ;;
    esac
    docker rm -f "$PFX-$n" >/dev/null 2>&1
}
pcount=0
puid_case "PUID=1000 only (GID stays default)" 1000:100 PUID=1000
puid_case "PGID=1000 only (UID stays default)" 99:1000 PGID=1000
puid_case "PUID=1000 PGID=1000" 1000:1000 PUID=1000 PGID=1000
puid_case "empty PUID/PGID are ignored" 99:100 PUID= PGID=
puid_case "' 1000 ' (whitespace trimmed)" 1000:100 "PUID= 1000 "
puid_case "leading zeros 0099/0100" 99:100 PUID=0099 PGID=0100
puid_case "PUID=1 (lowest)" 1:100 PUID=1
puid_case "PUID=4294967294 (highest)" 4294967294:100 PUID=4294967294
puid_case "PUID=0" "fail:PUID=0 would run HAMAL as root" PUID=0
puid_case "PUID=000" "fail:PUID=0 would run HAMAL as root" PUID=000
puid_case "PGID=0" "fail:PGID=0 would run HAMAL as root" PGID=0
puid_case "PUID=4294967295 ((uid_t)-1 keeps root)" "fail:PUID must be a whole number" PUID=4294967295
puid_case "PUID=99999999999" "fail:PUID must be a whole number" PUID=99999999999
puid_case "PUID=-1" "fail:PUID must be a whole number" PUID=-1
puid_case "PUID=abc" "fail:PUID must be a whole number" PUID=abc
puid_case "PUID=1.5" "fail:PUID must be a whole number" PUID=1.5
puid_case "PGID=1e3" "fail:PGID must be a whole number" PGID=1e3
d=$(seeded pq 10001:10001)
start pq "$d" -e PGID=10001
check "PGID only on 10001 data keeps detected UID 10001" bash -c "$(declare -f health running healthy proc_id has_id); PFX=$PFX; healthy $PFX-pq && has_id $PFX-pq 10001:10001"

echo
echo "TOTAL: $pass passed, $failures failed"
[ "$failures" = 0 ]
