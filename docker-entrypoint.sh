#!/bin/sh
set -eu

# Unraid commonly stores appdata as UID:GID 99:100 (nobody:users).
# The bind-mounted /data directory can be created by Docker as root:root,
# so initialize ownership before dropping privileges.
if [ "$(id -u)" -eq 0 ]; then
    mkdir -p /data

    if ! gosu 99:100 sh -c 'test -w /data'; then
        echo "HAMAL: initializing ownership of /data for Unraid (99:100)." >&2
        chown -R 99:100 /data
    fi

    if ! gosu 99:100 sh -c 'test -w /data'; then
        echo "HAMAL: /data is still not writable by 99:100. Check the host appdata mount and filesystem permissions." >&2
        exit 1
    fi

    exec gosu 99:100 /app/hamal "$@"
fi

# Preserve compatibility when an operator explicitly runs the container
# under a non-root UID/GID.
exec /app/hamal "$@"
