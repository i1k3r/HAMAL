# HAMAL Architecture

HAMAL is a single-container, self-hosted temporary local file courier service. The architecture uses a Go server, server-rendered templates with vanilla JavaScript enhancements, SQLite metadata, and opaque filesystem storage under one persistent `/data` mount.

## Scope

The current implementation contains application startup/shutdown, environment configuration, structured logs, request IDs, embedded templates and static assets (including self-hosted fonts), SQLite initialization and migrations, storage validation, health/readiness endpoints, and Docker deployment, plus the product features built on that foundation:

- Temporary rooms with a creator link and a derived participant link, QR code generation, and automatic expiry.
- Optional 4–8 digit PIN protection with lockout and per-room participant sessions.
- Streaming file upload and download with per-room, global and free-disk-space limits.
- A room-scoped Text / Clipboard channel.
- A background cleanup worker for expired or closed rooms, stale staging files and orphaned files.
- Optional Global Share links for single files (disabled by default).

## Design decisions

- One non-root OCI container; no external database, Redis, host networking, privileged mode, or Docker socket.
- `/data` is the only required persistent mount.
- SQLite uses WAL mode, foreign keys, and a busy timeout.
- Go templates, CSS, and vanilla JavaScript are used instead of React, TypeScript, Vite, SSE, or WebSockets. Clients poll the JSON API.
- All page assets are embedded in the binary and served locally; pages make no third-party requests.
- Creator credentials are cryptographically random bearer tokens; participant tokens are derived from them one-way. Only HMAC hashes are stored. QR codes contain participant URLs only.
- Creator-only actions (closing a room, unlocking a PIN lockout, managing share links) are rejected for participant tokens.
- Raw credentials, room tokens, PINs, cookies, query strings, and authorization headers must never be logged.
- Uploads stream to `/data/staging` and become visible only after being atomically moved to opaque paths beneath `/data/files`. Quota reservations are checked against committed usage on every growth step, so limits hold across concurrent uploads.
- Files and text are stored unencrypted and the server speaks plain HTTP; TLS is the responsibility of an optional reverse proxy.
- Global Share is an optional feature of the same self-hosted instance and the same HTTP listener; it is not a hosted LAN-Drop service and does not run under a separate policy profile.

## Runtime flow

1. Validate configuration.
2. Create and verify `/data`, `/data/files`, `/data/staging`, and `/data/secrets`.
3. Load an environment-provided server secret or persist a generated secret beneath `/data/secrets`.
4. Open SQLite, enable WAL/foreign keys/busy timeout, and run migrations.
5. Run one cleanup pass, then start the background cleanup worker.
6. Serve the landing page, room pages, the JSON API under `/api/v1/`, optional share links under `/s/`, `/healthz`, and `/readyz`.
